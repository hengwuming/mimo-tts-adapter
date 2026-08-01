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

func TestValidateRejectsInsecureEmotionEndpoint(t *testing.T) {
	cfg := validConfig()
	cfg.EmotionEnabled = true
	cfg.EmotionEndpoint = "http://emotion.example.com/v1/chat/completions"
	cfg.EmotionAPIKey = "emotion-key"
	cfg.EmotionModel = "emotion-model"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected insecure emotion endpoint error")
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
