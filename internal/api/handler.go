package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"mimo-tts-adapter/internal/auth"
	"mimo-tts-adapter/internal/limits"
	"mimo-tts-adapter/internal/upstream"
)

var voices = map[string]struct{}{
	"mimo_default": {}, "冰糖": {}, "茉莉": {}, "苏打": {}, "白桦": {},
	"Mia": {}, "Chloe": {}, "Milo": {}, "Dean": {},
}

type Config struct {
	PublicBaseURL   string
	DefaultVoice    string
	DefaultSpeed    int
	MaxRequestBytes int64
	MaxTextBytes    int
}

type Handler struct {
	config      Config
	gate        *limits.Gate
	synthesizer func(context.Context, string, string, int) ([]byte, error)
	logger      *slog.Logger
	requestSeq  atomic.Uint64
}

type ttsRequest struct {
	Text  string `json:"text"`
	Voice string `json:"voice,omitempty"`
	Speed *int   `json:"speed,omitempty"`
}

type errorEnvelope struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

func NewHandler(config Config, validator auth.Validator, gate *limits.Gate, synthesizer func(context.Context, string, string, int) ([]byte, error), logger *slog.Logger) http.Handler {
	h := &Handler{config: config, gate: gate, synthesizer: synthesizer, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /rule", h.rule)
	mux.Handle("GET /tts", validator.Middleware(http.HandlerFunc(h.ttsGET)))
	mux.Handle("POST /tts", validator.Middleware(http.HandlerFunc(h.ttsPOST)))
	return h.accessLog(mux)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSONStatus(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) rule(w http.ResponseWriter, _ *http.Request) {
	body := map[string]any{
		"name":           "MiMo V2.5 TTS - " + h.config.DefaultVoice,
		"url":            h.config.PublicBaseURL + `/tts,{"method":"POST","headers":{"Authorization":"Bearer REPLACE_WITH_ADAPTER_TOKEN","Content-Type":"application/json"},"body":{"text":"{{speakText}}","voice":"` + h.config.DefaultVoice + `","speed":{{speakSpeed}}}}`,
		"contentType":    "audio/mpeg",
		"concurrentRate": "0",
		"pauseDuration":  0,
	}
	writeJSONStatus(w, http.StatusOK, body)
}

func (h *Handler) ttsGET(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	for _, name := range []string{"text", "voice", "speed"} {
		if len(query[name]) > 1 {
			h.writeError(w, r, http.StatusBadRequest, "invalid_request", "duplicate query parameter")
			return
		}
	}
	var speed *int
	if raw := query.Get("speed"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			h.writeError(w, r, http.StatusBadRequest, "invalid_request", "speed must be an integer")
			return
		}
		speed = &parsed
	}
	h.synthesize(w, r, ttsRequest{Text: query.Get("text"), Voice: query.Get("voice"), Speed: speed})
}

func (h *Handler) ttsPOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.config.MaxRequestBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		h.writeError(w, r, status, "invalid_request", "invalid JSON request")
		return
	}
	if !utf8.Valid(body) {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "request must be valid UTF-8")
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request ttsRequest
	if err := decoder.Decode(&request); err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		h.writeError(w, r, status, "invalid_request", "invalid JSON request")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", "request must contain one JSON object")
		return
	}
	h.synthesize(w, r, request)
}

func (h *Handler) synthesize(w http.ResponseWriter, r *http.Request, request ttsRequest) {
	text, voice, speed, err := h.normalize(request)
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	release, err := h.gate.AcquireConcurrency(r.Context())
	if err != nil {
		h.writeError(w, r, http.StatusRequestTimeout, "cancelled", "request cancelled")
		return
	}
	defer release()

	audio, err := h.synthesizer(r.Context(), text, voice, speed)
	if err != nil {
		h.writeUpstreamError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(audio)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
}

func (h *Handler) normalize(request ttsRequest) (string, string, int, error) {
	if request.Text == "" {
		return "", "", 0, errors.New("text is required")
	}
	if !utf8.ValidString(request.Text) {
		return "", "", 0, errors.New("text must be valid UTF-8")
	}
	if len([]byte(request.Text)) > h.config.MaxTextBytes {
		return "", "", 0, errors.New("text is too long")
	}
	voice := request.Voice
	if voice == "" {
		voice = h.config.DefaultVoice
	}
	if _, ok := voices[voice]; !ok {
		return "", "", 0, errors.New("unsupported voice")
	}
	speed := h.config.DefaultSpeed
	if request.Speed != nil {
		speed = *request.Speed
	}
	if speed < 5 || speed > 50 {
		return "", "", 0, errors.New("speed must be between 5 and 50")
	}
	return request.Text, voice, speed, nil
}

func (h *Handler) writeUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		h.writeError(w, r, http.StatusGatewayTimeout, "timeout", "provider request timed out")
		return
	}
	if errors.Is(err, context.Canceled) {
		h.writeError(w, r, http.StatusRequestTimeout, "cancelled", "request cancelled")
		return
	}
	var providerErr *upstream.Error
	if !errors.As(err, &providerErr) {
		h.writeError(w, r, http.StatusBadGateway, "provider_unavailable", "TTS provider unavailable")
		return
	}
	switch providerErr.StatusCode {
	case http.StatusTooManyRequests:
		h.writeError(w, r, http.StatusTooManyRequests, "rate_limited", "TTS provider rate limited the request")
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden, 421:
		h.writeError(w, r, http.StatusBadGateway, "provider_rejected", "TTS provider rejected the request")
	default:
		if providerErr.Category == "provider_protocol_error" {
			h.writeError(w, r, http.StatusBadGateway, "provider_protocol_error", "invalid TTS provider response")
		} else {
			h.writeError(w, r, http.StatusBadGateway, "provider_unavailable", "TTS provider unavailable")
		}
	}
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	var envelope errorEnvelope
	envelope.Error.Code = code
	envelope.Error.Message = message
	envelope.Error.RequestID = RequestID(r.Context())
	writeJSONStatus(w, status, envelope)
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type contextKey string

const requestIDKey contextKey = "request-id"

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func (h *Handler) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		id := fmt.Sprintf("%x-%x", time.Now().UnixMilli(), h.requestSeq.Add(1))
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", id)
		recorder := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		next.ServeHTTP(recorder, r)
		h.logger.Info("http_request",
			"request_id", id,
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"bytes", recorder.bytes,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *responseRecorder) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseRecorder) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	written, err := w.ResponseWriter.Write(body)
	w.bytes += written
	return written, err
}

func (w *responseRecorder) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
