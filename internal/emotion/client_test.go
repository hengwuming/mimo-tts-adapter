package emotion

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAnnotateRendersVerifiedSegments(t *testing.T) {
	client := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer emotion-key" {
			t.Fatal("missing emotion authorization")
		}
		requestBody := string(body)
		if !strings.Contains(requestBody, `"role":"system"`) || !strings.Contains(requestBody, `"role":"user"`) ||
			!strings.Contains(requestBody, `"response_format":{"type":"json_object"}`) || !strings.Contains(requestBody, "原文") {
			t.Fatalf("unexpected request: %s", body)
		}
		return jsonResponse(`{"choices":[{"message":{"content":"{\"segments\":[{\"text\":\"他推开门，\",\"style\":\"突然、愤怒\"},{\"text\":\"大喊！\",\"style\":\"提高音量\"}]}"}}]}`), nil
	})}, Config{Endpoint: "https://emotion.invalid", APIKey: "emotion-key", Model: "model", MaxResponseBytes: 4096, MaxRetries: 0, ResponseFormat: true})
	got, err := client.Annotate(context.Background(), "他推开门，大喊！")
	if err != nil {
		t.Fatal(err)
	}
	if got != "(突然、愤怒)他推开门，(提高音量)大喊！" {
		t.Fatalf("annotated = %q", got)
	}
}

func TestAnnotateRejectsChangedText(t *testing.T) {
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"choices":[{"message":{"content":"{\"segments\":[{\"text\":\"改写\"}]}"}}]}`), nil
	})}, Config{Endpoint: "https://emotion.invalid", APIKey: "key", Model: "model", MaxResponseBytes: 4096})
	if _, err := client.Annotate(context.Background(), "原文"); err == nil {
		t.Fatal("expected changed text error")
	}
}

func TestAnnotateRetriesTransientStatus(t *testing.T) {
	attempts := 0
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts < 4 {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("busy"))}, nil
		}
		return jsonResponse(`{"choices":[{"message":{"content":"{\"segments\":[{\"text\":\"原文\"}]}"}}]}`), nil
	})}, Config{Endpoint: "https://emotion.invalid", APIKey: "key", Model: "model", MaxResponseBytes: 4096, MaxRetries: 3})
	client.sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := client.Annotate(context.Background(), "原文"); err != nil {
		t.Fatal(err)
	}
	if attempts != 4 {
		t.Fatalf("attempts = %d, want 4", attempts)
	}
}

func TestFallbackUsesOriginalText(t *testing.T) {
	got := Fallback(context.Background(), "原文", func(context.Context, string) (string, error) {
		return "", errInvalidResponse
	})
	if got != "原文" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestAnnotateRejectsUnsafeStyle(t *testing.T) {
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"choices":[{"message":{"content":"{\"segments\":[{\"text\":\"原文\",\"style\":\"坏\n标签\"}]}"}}]}`), nil
	})}, Config{Endpoint: "https://emotion.invalid", APIKey: "key", Model: "model", MaxResponseBytes: 4096})
	if _, err := client.Annotate(context.Background(), "原文"); err == nil {
		t.Fatal("expected unsafe style error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
