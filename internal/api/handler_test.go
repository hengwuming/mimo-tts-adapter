package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mimo-tts-adapter/internal/auth"
	"mimo-tts-adapter/internal/limits"
)

type fakeSynthesizer struct {
	audio []byte
	err   error
	calls int
	text  string
	voice string
	speed int
}

func (f *fakeSynthesizer) Synthesize(_ context.Context, text, voice string, speed int) ([]byte, error) {
	f.calls++
	f.text, f.voice, f.speed = text, voice, speed
	return f.audio, f.err
}

func newTestHandler(synthesizer *fakeSynthesizer, logOutput io.Writer) http.Handler {
	return NewHandler(Config{
		PublicBaseURL:   "https://tts.example.com",
		DefaultVoice:    "冰糖",
		DefaultSpeed:    25,
		MaxRequestBytes: 1024,
		MaxTextBytes:    128,
	}, auth.New("adapter-secret"), limits.New(2, 1000, 2), synthesizer.Synthesize, slog.New(slog.NewJSONHandler(logOutput, nil)))
}

func TestHealthDoesNotSynthesize(t *testing.T) {
	fake := &fakeSynthesizer{}
	recorder := httptest.NewRecorder()
	newTestHandler(fake, io.Discard).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || fake.calls != 0 {
		t.Fatalf("status = %d, calls = %d", recorder.Code, fake.calls)
	}
}

func TestTTSRequiresAuthentication(t *testing.T) {
	fake := &fakeSynthesizer{}
	recorder := httptest.NewRecorder()
	newTestHandler(fake, io.Discard).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/tts?text=test", nil))
	if recorder.Code != http.StatusUnauthorized || fake.calls != 0 {
		t.Fatalf("status = %d, calls = %d", recorder.Code, fake.calls)
	}
}

func TestPostTTSReturnsMP3(t *testing.T) {
	fake := &fakeSynthesizer{audio: []byte("ID3-audio")}
	request := httptest.NewRequest(http.MethodPost, "/tts", strings.NewReader(`{"text":"中文\n\"引号\"","voice":"茉莉","speed":50}`))
	request.Header.Set("Authorization", "Bearer adapter-secret")
	recorder := httptest.NewRecorder()
	newTestHandler(fake, io.Discard).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "audio/mpeg" || recorder.Body.String() != "ID3-audio" {
		t.Fatalf("content-type = %q, body = %q", recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
	if fake.text != "中文\n\"引号\"" || fake.voice != "茉莉" || fake.speed != 50 {
		t.Fatalf("normalized request = %#v", fake)
	}
}

func TestPostRejectsUnknownAndTrailingJSON(t *testing.T) {
	for _, body := range []string{
		`{"text":"test","unknown":true}`,
		`{"text":"test"}{"text":"second"}`,
	} {
		fake := &fakeSynthesizer{}
		request := httptest.NewRequest(http.MethodPost, "/tts", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer adapter-secret")
		recorder := httptest.NewRecorder()
		newTestHandler(fake, io.Discard).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || fake.calls != 0 {
			t.Fatalf("body = %q, status = %d, calls = %d", body, recorder.Code, fake.calls)
		}
	}
}

func TestRuleDoesNotLeakSecrets(t *testing.T) {
	fake := &fakeSynthesizer{}
	recorder := httptest.NewRecorder()
	newTestHandler(fake, io.Discard).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/rule", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || strings.Contains(body, "adapter-secret") || !strings.Contains(body, "REPLACE_WITH_ADAPTER_TOKEN") {
		t.Fatalf("unsafe rule: %s", body)
	}
}

func TestLogsDoNotContainTextOrAuthorization(t *testing.T) {
	var logs bytes.Buffer
	fake := &fakeSynthesizer{err: errors.New("provider failed")}
	request := httptest.NewRequest(http.MethodGet, "/tts?text=private-paragraph", nil)
	request.Header.Set("Authorization", "Bearer adapter-secret")
	recorder := httptest.NewRecorder()
	newTestHandler(fake, &logs).ServeHTTP(recorder, request)
	if strings.Contains(logs.String(), "private-paragraph") || strings.Contains(logs.String(), "adapter-secret") {
		t.Fatalf("sensitive log: %s", logs.String())
	}
}
