package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultEndpoint = "https://api.xiaomimimo.com/v1/chat/completions"

type Config struct {
	ListenAddr               string
	AdapterAuthToken         string
	MIMOAPIKey               string
	MIMOEndpoint             string
	MIMOModel                string
	PublicBaseURL            string
	DefaultVoice             string
	DefaultSpeed             int
	MaxRequestBytes          int64
	MaxTextBytes             int
	MaxUpstreamBytes         int64
	MaxAudioBytes            int
	UpstreamTimeout          time.Duration
	MaxConcurrency           int
	RatePerSecond            float64
	RateBurst                int
	MaxRetries               int
	MaxRetryDelay            time.Duration
	ShutdownTimeout          time.Duration
	AllowInsecureMIMO        bool
	EmotionEnabled           bool
	EmotionEndpoint          string
	EmotionAPIKey            string
	EmotionModel             string
	EmotionTimeout           time.Duration
	EmotionMaxResponseBytes  int64
	EmotionMaxRetries        int
	EmotionResponseFormat    bool
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:              env("LISTEN_ADDR", ":8080"),
		AdapterAuthToken:        os.Getenv("ADAPTER_AUTH_TOKEN"),
		MIMOAPIKey:              os.Getenv("MIMO_API_KEY"),
		MIMOEndpoint:            env("MIMO_ENDPOINT", defaultEndpoint),
		MIMOModel:               env("MIMO_MODEL", "mimo-v2.5-tts"),
		PublicBaseURL:           strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
		DefaultVoice:            env("DEFAULT_VOICE", "冰糖"),
		EmotionEndpoint:         os.Getenv("EMOTION_ENDPOINT"),
		EmotionAPIKey:           os.Getenv("EMOTION_API_KEY"),
		EmotionModel:            os.Getenv("EMOTION_MODEL"),
		EmotionResponseFormat:   true,
	}

	var err error
	if cfg.DefaultSpeed, err = envInt("DEFAULT_SPEED", 25, 5, 50); err != nil {
		return Config{}, err
	}
	if cfg.MaxRequestBytes, err = envInt64("MAX_REQUEST_BYTES", 64<<10, 1024, 1<<20); err != nil {
		return Config{}, err
	}
	if cfg.MaxTextBytes, err = envInt("MAX_TEXT_BYTES", 16<<10, 1, 256<<10); err != nil {
		return Config{}, err
	}
	if cfg.MaxUpstreamBytes, err = envInt64("MAX_UPSTREAM_BYTES", 32<<20, 1024, 256<<20); err != nil {
		return Config{}, err
	}
	if cfg.MaxAudioBytes, err = envInt("MAX_AUDIO_BYTES", 20<<20, 1024, 128<<20); err != nil {
		return Config{}, err
	}
	if cfg.UpstreamTimeout, err = envDuration("UPSTREAM_TIMEOUT", 90*time.Second, time.Second, 5*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.MaxConcurrency, err = envInt("MAX_CONCURRENCY", 2, 1, 32); err != nil {
		return Config{}, err
	}
	if cfg.RatePerSecond, err = envFloat("RATE_PER_SECOND", 1, 0.01, 100); err != nil {
		return Config{}, err
	}
	if cfg.RateBurst, err = envInt("RATE_BURST", 2, 1, 100); err != nil {
		return Config{}, err
	}
	if cfg.MaxRetries, err = envInt("MAX_RETRIES", 2, 0, 5); err != nil {
		return Config{}, err
	}
	if cfg.MaxRetryDelay, err = envDuration("MAX_RETRY_DELAY", 10*time.Second, 100*time.Millisecond, time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = envDuration("SHUTDOWN_TIMEOUT", 15*time.Second, time.Second, time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.AllowInsecureMIMO, err = envBool("MIMO_ALLOW_HTTP", false); err != nil {
		return Config{}, err
	}
	if cfg.EmotionEnabled, err = envBool("EMOTION_ENABLED", false); err != nil {
		return Config{}, err
	}
	if cfg.EmotionTimeout, err = envDuration("EMOTION_TIMEOUT", 5*time.Second, time.Second, time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.EmotionMaxResponseBytes, err = envInt64("EMOTION_MAX_RESPONSE_BYTES", 8<<10, 1024, 1<<20); err != nil {
		return Config{}, err
	}
	if cfg.EmotionMaxRetries, err = envInt("EMOTION_MAX_RETRIES", 3, 0, 5); err != nil {
		return Config{}, err
	}
	if cfg.EmotionResponseFormat, err = envBool("EMOTION_RESPONSE_FORMAT", true); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.AdapterAuthToken == "" {
		return errors.New("ADAPTER_AUTH_TOKEN is required")
	}
	if c.MIMOAPIKey == "" {
		return errors.New("MIMO_API_KEY is required")
	}
	if c.PublicBaseURL == "" {
		return errors.New("PUBLIC_BASE_URL is required")
	}
	publicURL, err := url.Parse(c.PublicBaseURL)
	if err != nil || publicURL.Scheme == "" || publicURL.Host == "" {
		return errors.New("PUBLIC_BASE_URL must be an absolute URL")
	}
	endpoint, err := url.Parse(c.MIMOEndpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return errors.New("MIMO_ENDPOINT must be an absolute URL")
	}
	if endpoint.Scheme != "https" && !(c.AllowInsecureMIMO && endpoint.Scheme == "http") {
		return errors.New("MIMO_ENDPOINT must use HTTPS")
	}
	if c.MIMOModel != "mimo-v2.5-tts" {
		return errors.New("MIMO_MODEL must be mimo-v2.5-tts")
	}
	if !c.EmotionEnabled {
		return nil
	}
	if c.EmotionAPIKey == "" || c.EmotionModel == "" {
		return errors.New("EMOTION_API_KEY and EMOTION_MODEL are required when EMOTION_ENABLED is true")
	}
	emotionEndpoint, err := url.Parse(c.EmotionEndpoint)
	if err != nil || emotionEndpoint.Scheme == "" || emotionEndpoint.Host == "" || emotionEndpoint.Scheme != "https" {
		return errors.New("EMOTION_ENDPOINT must be an absolute HTTPS URL")
	}
	return nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback, min, max int) (int, error) {
	value, err := envInt64(name, int64(fallback), int64(min), int64(max))
	return int(value), err
}

func envInt64(name string, fallback, min, max int64) (int64, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("%s must be between %d and %d", name, min, max)
	}
	return value, nil
}

func envFloat(name string, fallback, min, max float64) (float64, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("%s must be between %g and %g", name, min, max)
	}
	return value, nil
}

func envDuration(name string, fallback, min, max time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("%s must be between %s and %s", name, min, max)
	}
	return value, nil
}

func envBool(name string, fallback bool) (bool, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return value, nil
}
