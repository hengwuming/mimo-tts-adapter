package config

import "testing"

func TestValidateAllowsDisabledEmotionWithoutCredentials(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRequiresEnabledEmotionConfiguration(t *testing.T) {
	cfg := validConfig()
	cfg.EmotionEnabled = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing emotion configuration error")
	}
	cfg.EmotionEndpoint = "https://emotion.example.com/v1/chat/completions"
	cfg.EmotionAPIKey = "emotion-key"
	cfg.EmotionModel = "emotion-model"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAllowsHTTPEmotionEndpoint(t *testing.T) {
	cfg := validConfig()
	cfg.EmotionEnabled = true
	cfg.EmotionEndpoint = "http://emotion-service:8000/v1/chat/completions"
	cfg.EmotionAPIKey = "emotion-key"
	cfg.EmotionModel = "emotion-model"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsInvalidEmotionEndpoint(t *testing.T) {
	cfg := validConfig()
	cfg.EmotionEnabled = true
	cfg.EmotionEndpoint = "emotion-service:8000/v1/chat/completions"
	cfg.EmotionAPIKey = "emotion-key"
	cfg.EmotionModel = "emotion-model"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid emotion endpoint error")
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"LISTEN_ADDR", "MIMO_ENDPOINT", "MIMO_MODEL", "DEFAULT_VOICE", "DEFAULT_SPEED",
		"MAX_REQUEST_BYTES", "MAX_TEXT_BYTES", "MAX_UPSTREAM_BYTES", "MAX_AUDIO_BYTES",
		"UPSTREAM_TIMEOUT", "MAX_CONCURRENCY", "RATE_PER_SECOND", "RATE_BURST",
		"MAX_RETRIES", "MAX_RETRY_DELAY", "SHUTDOWN_TIMEOUT", "MIMO_ALLOW_HTTP",
		"EMOTION_TIMEOUT", "EMOTION_MAX_RESPONSE_BYTES", "EMOTION_MAX_RETRIES",
		"EMOTION_RESPONSE_FORMAT", "EMOTION_RESPONSE_LOG_FILE",
	} {
		t.Setenv(name, "")
	}
	for name, value := range map[string]string{
		"ADAPTER_AUTH_TOKEN": "adapter-token",
		"MIMO_API_KEY":       "mimo-key",
		"PUBLIC_BASE_URL":    "https://tts.example.com",
		"EMOTION_ENABLED":    "true",
		"EMOTION_ENDPOINT":   "http://emotion.example.com/v1/chat/completions",
		"EMOTION_API_KEY":    "emotion-key",
		"EMOTION_MODEL":      "emotion-model",
	} {
		t.Setenv(name, value)
	}
}

func TestLoadDefaultsEmotionSettings(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmotionTimeout.String() != "7s" {
		t.Fatalf("EmotionTimeout = %s", cfg.EmotionTimeout)
	}
	if cfg.EmotionMaxRetries != 0 {
		t.Fatalf("EmotionMaxRetries = %d", cfg.EmotionMaxRetries)
	}
	if cfg.EmotionResponseLogFile != "" {
		t.Fatalf("EmotionResponseLogFile = %q", cfg.EmotionResponseLogFile)
	}
}

func TestLoadEmotionResponseLogFile(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("EMOTION_RESPONSE_LOG_FILE", "/logs/emotion.jsonl")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EmotionResponseLogFile != "/logs/emotion.jsonl" {
		t.Fatalf("EmotionResponseLogFile = %q", cfg.EmotionResponseLogFile)
	}
}

func validConfig() Config {
	return Config{
		AdapterAuthToken: "adapter-token",
		MIMOAPIKey:       "mimo-key",
		MIMOEndpoint:     defaultEndpoint,
		MIMOModel:        "mimo-v2.5-tts",
		PublicBaseURL:    "https://tts.example.com",
	}
}
