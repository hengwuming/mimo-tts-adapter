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
		emotionHTTPClient := &http.Client{
			Transport: transport,
			Timeout:   cfg.EmotionTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		emotionClient := emotion.New(emotionHTTPClient, emotion.Config{
			Endpoint:         cfg.EmotionEndpoint,
			APIKey:           cfg.EmotionAPIKey,
			Model:            cfg.EmotionModel,
			MaxResponseBytes: cfg.EmotionMaxResponseBytes,
			MaxRetries:       cfg.EmotionMaxRetries,
			ResponseFormat:   cfg.EmotionResponseFormat,
		})
		annotate = emotionClient.Annotate
	}
	annotatedSynthesizer := func(ctx context.Context, text, voice string, speed int) ([]byte, error) {
		if annotate != nil {
			if annotated, err := annotate(ctx, text); err == nil {
				text = annotated
			}
		}
		return provider.Synthesize(ctx, text, voice, speed)
	}
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
