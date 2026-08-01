package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestNewSynthesizerLogsStagesWithoutText(t *testing.T) {
	const original = "private-original-text"
	const instruction = "private-style-instruction"
	var providerText string
	var providerInstruction string
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	synthesize := newSynthesizer(
		func(_ context.Context, text, _ string, _ int, styleInstruction string) ([]byte, error) {
			providerText = text
			providerInstruction = styleInstruction
			return []byte("audio"), nil
		},
		func(context.Context, string) (string, error) { return instruction, nil },
		logger,
	)
	if _, err := synthesize(context.Background(), original, "冰糖", 25); err != nil {
		t.Fatal(err)
	}
	if providerText != original || providerInstruction != instruction {
		t.Fatalf("provider text = %q, instruction = %q", providerText, providerInstruction)
	}
	logs := output.String()
	if !strings.Contains(logs, `"msg":"emotion_completed"`) || !strings.Contains(logs, `"msg":"mimo_completed"`) {
		t.Fatalf("missing stage logs: %s", logs)
	}
	if strings.Contains(logs, original) || strings.Contains(logs, instruction) {
		t.Fatalf("sensitive stage logs: %s", logs)
	}
}

func TestNewSynthesizerLogsEmotionFallbackWithoutErrorText(t *testing.T) {
	const original = "private-original-text"
	const privateError = "provider exposed private response"
	var providerText string
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	synthesize := newSynthesizer(
		func(_ context.Context, text, _ string, _ int, styleInstruction string) ([]byte, error) {
			providerText = text
			if styleInstruction != "" {
				t.Fatalf("fallback instruction = %q", styleInstruction)
			}
			return []byte("audio"), nil
		},
		func(context.Context, string) (string, error) { return "", errors.New(privateError) },
		logger,
	)
	if _, err := synthesize(context.Background(), original, "冰糖", 25); err != nil {
		t.Fatal(err)
	}
	if providerText != original {
		t.Fatalf("provider text = %q", providerText)
	}
	logs := output.String()
	if !strings.Contains(logs, `"status":"fallback"`) || !strings.Contains(logs, `"error_category":"unavailable"`) {
		t.Fatalf("missing fallback log: %s", logs)
	}
	if strings.Contains(logs, original) || strings.Contains(logs, privateError) {
		t.Fatalf("sensitive fallback log: %s", logs)
	}
}
