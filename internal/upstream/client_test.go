package upstream

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSpeedInstruction(t *testing.T) {
	if SpeedInstruction(25) != "请用正常语速自然朗读。" {
		t.Fatal("normal speed instruction changed")
	}
	if SpeedInstruction(50) != "请用很快但仍清晰的语速朗读。" {
		t.Fatal("fast speed instruction changed")
	}
}

func TestClientDecodesAudioAndBuildsPayload(t *testing.T) {
	wantAudio := []byte("ID3-test")
	encoded := base64.StdEncoding.EncodeToString(wantAudio)
	client := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer provider-key" {
			t.Errorf("missing provider authorization")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "assistant") || !strings.Contains(string(body), "正文") {
			t.Errorf("unexpected payload: %s", body)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"audio":{"data":"` + encoded + `"}}}]}`)), Header: make(http.Header)}, nil
	})}, Config{Endpoint: "https://provider.invalid", APIKey: "provider-key", Model: "mimo-v2.5-tts", MaxResponseBytes: 4096, MaxAudioBytes: 1024, MaxRetries: 0}, nil)
	got, err := client.Synthesize(context.Background(), "正文", "冰糖", 25)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(wantAudio) {
		t.Fatalf("audio = %q, want %q", got, wantAudio)
	}
}

func TestClientRetries429(t *testing.T) {
	attempts := 0
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("busy"))}, nil
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"audio":{"data":"SUQz"}}}]}`))}, nil
	})}, Config{Endpoint: "https://provider.invalid", APIKey: "key", Model: "mimo-v2.5-tts", MaxResponseBytes: 4096, MaxAudioBytes: 1024, MaxRetries: 1, MaxRetryDelay: time.Second}, nil)
	client.sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := client.Synthesize(context.Background(), "正文", "冰糖", 25); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestClientRejectsOversizedResponse(t *testing.T) {
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("12345"))}, nil
	})}, Config{Endpoint: "https://provider.invalid", APIKey: "key", Model: "mimo-v2.5-tts", MaxResponseBytes: 4, MaxAudioBytes: 1024}, nil)
	if _, err := client.Synthesize(context.Background(), "正文", "冰糖", 25); err == nil {
		t.Fatal("expected oversized response error")
	}
}
