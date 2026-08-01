package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mimo-tts-adapter/internal/api"
	"mimo-tts-adapter/internal/auth"
	"mimo-tts-adapter/internal/config"
	"mimo-tts-adapter/internal/emotion"
	"mimo-tts-adapter/internal/limits"
	"mimo-tts-adapter/internal/upstream"
)

func errorCategory(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var emotionErr *emotion.Error
	if errors.As(err, &emotionErr) {
		return emotionErr.Category
	}
	var providerErr *upstream.Error
	if errors.As(err, &providerErr) {
		return providerErr.Category
	}
	return "unavailable"
}

func newSynthesizer(
	provider func(context.Context, string, string, int, string) ([]byte, error),
	annotate func(context.Context, string) (string, error),
	logger *slog.Logger,
) func(context.Context, string, string, int) ([]byte, error) {
	return func(ctx context.Context, text, voice string, speed int) ([]byte, error) {
		requestID := api.RequestID(ctx)
		var styleInstruction string
		if annotate != nil {
			started := time.Now()
			instruction, err := annotate(ctx, text)
			if err == nil {
				styleInstruction = instruction
				logger.Info("emotion_completed", "request_id", requestID, "status", "success", "duration_ms", time.Since(started).Milliseconds())
			} else {
				logger.Warn("emotion_completed", "request_id", requestID, "status", "fallback", "duration_ms", time.Since(started).Milliseconds(), "error_category", errorCategory(err))
			}
		}
		started := time.Now()
		audio, err := provider(ctx, text, voice, speed, styleInstruction)
		status := "success"
		if err != nil {
			status = "error"
		}
		logger.Info("mimo_completed", "request_id", requestID, "status", status, "duration_ms", time.Since(started).Milliseconds(), "error_category", errorCategory(err))
		return audio, err
	}
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err.Error())
		os.Exit(1)
	}

	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   cfg.MaxConcurrency,
		MaxConnsPerHost:       cfg.MaxConcurrency,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: cfg.UpstreamTimeout,
		ExpectContinueTimeout: time.Second,
	}
	defer transport.CloseIdleConnections()

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.UpstreamTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	gate := limits.New(cfg.MaxConcurrency, cfg.RatePerSecond, cfg.RateBurst)
	provider := upstream.New(httpClient, upstream.Config{
		Endpoint:         cfg.MIMOEndpoint,
		APIKey:           cfg.MIMOAPIKey,
		Model:            cfg.MIMOModel,
		MaxResponseBytes: cfg.MaxUpstreamBytes,
		MaxAudioBytes:    cfg.MaxAudioBytes,
		MaxRetries:       cfg.MaxRetries,
		MaxRetryDelay:    cfg.MaxRetryDelay,
	}, gate.WaitRate)
	var annotate func(context.Context, string) (string, error)
	if cfg.EmotionEnabled {
		var responseLogger *emotion.ResponseLogger
		if cfg.EmotionResponseLogFile != "" {
			responseLogger, err = emotion.OpenResponseLogger(cfg.EmotionResponseLogFile)
			if err != nil {
				logger.Error("emotion_response_log_failed", "error", err.Error())
				os.Exit(1)
			}
			defer responseLogger.Close()
		}
		emotionHTTPClient := &http.Client{
			Transport: transport,
			Timeout:   cfg.EmotionTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		var logResponse emotion.ResponseLogFunc
		if responseLogger != nil {
			logResponse = func(ctx context.Context, entry emotion.ResponseLogEntry) {
				entry.RequestID = api.RequestID(ctx)
				if err := responseLogger.Write(entry); err != nil {
					logger.Error("emotion_response_log_failed", "request_id", entry.RequestID, "error", err.Error())
				}
			}
		}
		emotionClient := emotion.New(emotionHTTPClient, emotion.Config{
			Endpoint:         cfg.EmotionEndpoint,
			APIKey:           cfg.EmotionAPIKey,
			Model:            cfg.EmotionModel,
			MaxResponseBytes: cfg.EmotionMaxResponseBytes,
			MaxRetries:       cfg.EmotionMaxRetries,
			ResponseFormat:   cfg.EmotionResponseFormat,
			LogResponse:      logResponse,
		})
		annotate = emotionClient.Annotate
	}
	annotatedSynthesizer := newSynthesizer(provider.Synthesize, annotate, logger)
	handler := api.NewHandler(api.Config{
		PublicBaseURL:   cfg.PublicBaseURL,
		DefaultVoice:    cfg.DefaultVoice,
		DefaultSpeed:    cfg.DefaultSpeed,
		MaxRequestBytes: cfg.MaxRequestBytes,
		MaxTextBytes:    cfg.MaxTextBytes,
	}, auth.New(cfg.AdapterAuthToken), gate, annotatedSynthesizer, logger)

	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()
	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler,
		BaseContext: func(net.Listener) context.Context {
			return appCtx
		},
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      cfg.UpstreamTimeout + 10*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server_started", "listen_addr", cfg.ListenAddr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case <-signalCtx.Done():
		logger.Info("server_shutdown_started")
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server_failed", "error", err.Error())
			os.Exit(1)
		}
		return
	}

	cancelApp()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful_shutdown_failed", "error", err.Error())
		_ = server.Close()
		os.Exit(1)
	}
	logger.Info("server_stopped")
}
