package emotion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAnnotateRendersRuneRangesWithJSONSchema(t *testing.T) {
	const original = "　　“诶，那你不是也有机会吗？”姬灵若美目发亮。"
	client := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer emotion-key" {
			t.Fatal("missing emotion authorization")
		}
		var payload request
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.ResponseFormat == nil || payload.ResponseFormat.Type != "json_schema" {
			t.Fatalf("response format = %#v", payload.ResponseFormat)
		}
		if payload.ResponseFormat.JSONSchema == nil || payload.ResponseFormat.JSONSchema.Name != "emotion_styles" || !payload.ResponseFormat.JSONSchema.Strict {
			t.Fatalf("json schema = %#v", payload.ResponseFormat.JSONSchema)
		}
		var schema map[string]any
		if err := json.Unmarshal(payload.ResponseFormat.JSONSchema.Schema, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["additionalProperties"] != false || !strings.Contains(string(payload.ResponseFormat.JSONSchema.Schema), `"required":["start","end","style"]`) {
			t.Fatalf("schema = %s", payload.ResponseFormat.JSONSchema.Schema)
		}
		if len(payload.Messages) != 2 || payload.Messages[1].Content != original || !strings.Contains(payload.Messages[0].Content, "Unicode rune") {
			t.Fatalf("messages = %#v", payload.Messages)
		}
		return contentResponse(`{"styles":[{"start":2,"end":16,"style":"语气惊讶且兴奋，带着点调侃"},{"start":16,"end":24,"style":"语调上扬，透出期待与雀跃"}]}`), nil
	})}, Config{
		Endpoint:         "https://emotion.invalid",
		APIKey:           "emotion-key",
		Model:            "model",
		MaxResponseBytes: 4096,
		ResponseFormat:   true,
	})
	got, err := client.Annotate(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	want := "请按以下分段指导朗读，指导文字不要读出：以““诶，那你不是也有机会吗？””开头的片段使用“语气惊讶且兴奋，带着点调侃”的表达；以“姬灵若美目发亮。”开头的片段使用“语调上扬，透出期待与雀跃”的表达"
	if got != want {
		t.Fatalf("instruction = %q, want %q", got, want)
	}
}

func TestAnnotateOmitsResponseFormatWhenDisabled(t *testing.T) {
	client := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var payload request
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.ResponseFormat != nil {
			t.Fatalf("response format = %#v", payload.ResponseFormat)
		}
		return contentResponse(`{"styles":[]}`), nil
	})}, testConfig())
	if got, err := client.Annotate(context.Background(), "原文"); err != nil || got != "" {
		t.Fatalf("instruction = %q, error = %v", got, err)
	}
}

func TestAnnotateRetriesTransientStatus(t *testing.T) {
	attempts := 0
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts < 4 {
			return statusResponse(http.StatusTooManyRequests, "busy"), nil
		}
		return contentResponse(`{"styles":[]}`), nil
	})}, Config{
		Endpoint:         "https://emotion.invalid",
		APIKey:           "key",
		Model:            "model",
		MaxResponseBytes: 4096,
		MaxRetries:       3,
	})
	client.sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := client.Annotate(context.Background(), "原文"); err != nil {
		t.Fatal(err)
	}
	if attempts != 4 {
		t.Fatalf("attempts = %d, want 4", attempts)
	}
}

func TestAnnotateLogsSuccessfulContent(t *testing.T) {
	var entry ResponseLogEntry
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return contentResponse(`{"styles":[{"start":0,"end":2,"style":"轻声"}]}`), nil
	})}, Config{
		Endpoint:         "https://emotion.invalid",
		APIKey:           "key",
		Model:            "model",
		MaxResponseBytes: 4096,
		LogResponse: func(_ context.Context, value ResponseLogEntry) {
			value.RequestID = "request-1"
			entry = value
		},
	})
	if _, err := client.Annotate(context.Background(), "原文"); err != nil {
		t.Fatal(err)
	}
	if entry.Status != "success" || entry.ErrorCategory != "" || entry.Attempts != 1 || entry.RequestID != "request-1" {
		t.Fatalf("entry = %#v", entry)
	}
	if !strings.Contains(entry.Content, `"style":"轻声"`) || entry.StyleInstruction != "请按以下分段指导朗读，指导文字不要读出：以“原文”开头的片段使用“轻声”的表达" {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestAnnotateLogsProviderStatusDetails(t *testing.T) {
	const providerMessage = "response_format type json_schema is not supported"
	var entry ResponseLogEntry
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]any{
			"error": map[string]string{"message": providerMessage},
		})
		return statusResponse(http.StatusBadRequest, string(body)), nil
	})}, Config{
		Endpoint:         "https://emotion.invalid",
		APIKey:           "key",
		Model:            "model",
		MaxResponseBytes: 4096,
		LogResponse: func(_ context.Context, value ResponseLogEntry) {
			entry = value
		},
	})
	_, err := client.Annotate(context.Background(), "原文")
	requireCategory(t, err, categoryProviderStatus)
	if entry.ProviderStatus != http.StatusBadRequest || entry.ProviderError != providerMessage {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestAnnotateTruncatesProviderError(t *testing.T) {
	var entry ResponseLogEntry
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]any{
			"error": map[string]string{"message": strings.Repeat("错", maxProviderErrorRunes+10)},
		})
		return statusResponse(http.StatusBadRequest, string(body)), nil
	})}, Config{
		Endpoint:         "https://emotion.invalid",
		APIKey:           "key",
		Model:            "model",
		MaxResponseBytes: 4096,
		LogResponse: func(_ context.Context, value ResponseLogEntry) {
			entry = value
		},
	})
	if _, err := client.Annotate(context.Background(), "原文"); err == nil {
		t.Fatal("expected provider status error")
	}
	if got := len([]rune(entry.ProviderError)); got != maxProviderErrorRunes+1 || !strings.HasSuffix(entry.ProviderError, "…") {
		t.Fatalf("provider error length = %d, value = %q", got, entry.ProviderError)
	}
}

func TestAnnotateBoundsProviderErrorBody(t *testing.T) {
	var entry ResponseLogEntry
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return statusResponse(http.StatusBadRequest, strings.Repeat("x", maxProviderErrorBytes+1)), nil
	})}, Config{
		Endpoint:         "https://emotion.invalid",
		APIKey:           "key",
		Model:            "model",
		MaxResponseBytes: 1 << 20,
		LogResponse: func(_ context.Context, value ResponseLogEntry) {
			entry = value
		},
	})
	_, err := client.Annotate(context.Background(), "原文")
	requireCategory(t, err, categoryProviderStatus)
	if entry.ProviderStatus != http.StatusBadRequest || entry.ProviderError != "" {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestAnnotateDoesNotLogUnstructuredProviderBody(t *testing.T) {
	var entry ResponseLogEntry
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return statusResponse(http.StatusBadRequest, "private raw error body"), nil
	})}, Config{
		Endpoint:         "https://emotion.invalid",
		APIKey:           "key",
		Model:            "model",
		MaxResponseBytes: 4096,
		LogResponse: func(_ context.Context, value ResponseLogEntry) {
			entry = value
		},
	})
	if _, err := client.Annotate(context.Background(), "原文"); err == nil {
		t.Fatal("expected provider status error")
	}
	if entry.ProviderError != "" {
		t.Fatalf("provider error = %q", entry.ProviderError)
	}
}

func TestAnnotateLogsFailureCategories(t *testing.T) {
	tests := []struct {
		name        string
		client      *http.Client
		maxBytes    int64
		want        string
		wantContent bool
	}{
		{
			name: "provider status",
			client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return statusResponse(http.StatusBadRequest, "bad"), nil
			})},
			maxBytes: 4096,
			want:     categoryProviderStatus,
		},
		{
			name: "response too large",
			client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse(strings.Repeat("x", 17)), nil
			})},
			maxBytes: 16,
			want:     categoryResponseTooLarge,
		},
		{
			name: "provider json",
			client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonResponse("not-json"), nil
			})},
			maxBytes: 4096,
			want:     categoryProviderJSON,
		},
		{
			name: "content json",
			client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return contentResponse("not-json-private-text"), nil
			})},
			maxBytes:    4096,
			want:        categoryContentJSON,
			wantContent: true,
		},
		{
			name: "invalid range",
			client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return contentResponse(`{"styles":[{"start":0,"end":99,"style":"轻声"}]}`), nil
			})},
			maxBytes:    4096,
			want:        categoryInvalidRange,
			wantContent: true,
		},
		{
			name: "invalid style",
			client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return contentResponse(`{"styles":[{"start":0,"end":2,"style":"(轻声)"}]}`), nil
			})},
			maxBytes:    4096,
			want:        categoryInvalidStyle,
			wantContent: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entry ResponseLogEntry
			client := New(tt.client, Config{
				Endpoint:         "https://emotion.invalid",
				APIKey:           "key",
				Model:            "model",
				MaxResponseBytes: tt.maxBytes,
				LogResponse: func(_ context.Context, value ResponseLogEntry) {
					entry = value
				},
			})
			if _, err := client.Annotate(context.Background(), "原文"); err == nil {
				t.Fatal("expected error")
			}
			if entry.Status != "error" || entry.ErrorCategory != tt.want || entry.Attempts != 1 {
				t.Fatalf("entry = %#v", entry)
			}
			if (entry.Content != "") != tt.wantContent || entry.StyleInstruction != "" {
				t.Fatalf("entry = %#v", entry)
			}
		})
	}
}

func TestAnnotateLogsTimeoutAndCancelled(t *testing.T) {
	tests := []struct {
		name  string
		cause error
		want  string
	}{
		{name: "timeout", cause: context.DeadlineExceeded, want: "timeout"},
		{name: "cancelled", cause: context.Canceled, want: "cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entry ResponseLogEntry
			client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, tt.cause
			})}, Config{
				Endpoint:         "https://emotion.invalid",
				APIKey:           "key",
				Model:            "model",
				MaxResponseBytes: 4096,
				LogResponse: func(_ context.Context, value ResponseLogEntry) {
					entry = value
				},
			})
			var ctx context.Context
			if errors.Is(tt.cause, context.DeadlineExceeded) {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
				defer cancel()
				time.Sleep(time.Millisecond)
			} else {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}
			if _, err := client.Annotate(ctx, "原文"); !errors.Is(err, tt.cause) {
				t.Fatalf("error = %v", err)
			}
			if entry.ErrorCategory != tt.want {
				t.Fatalf("entry = %#v", entry)
			}
			if entry.Attempts < 0 || entry.Attempts > 1 {
				t.Fatalf("attempts = %d", entry.Attempts)
			}
		})
	}
}

func TestAnnotateAcceptsSingleJSONFence(t *testing.T) {
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return contentResponse("```json\n{\"styles\":[{\"start\":0,\"end\":2,\"style\":\"平静\"}]}\n```"), nil
	})}, testConfig())
	got, err := client.Annotate(context.Background(), "原文")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "平静") {
		t.Fatalf("instruction = %q", got)
	}
}

func TestAnnotateRejectsFenceWithExtraText(t *testing.T) {
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return contentResponse("说明：\n```json\n{\"styles\":[]}\n```"), nil
	})}, testConfig())
	_, err := client.Annotate(context.Background(), "原文")
	requireCategory(t, err, categoryContentJSON)
}

func TestAnnotateWithNoStylesReturnsEmptyInstruction(t *testing.T) {
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return contentResponse(`{"styles":[]}`), nil
	})}, testConfig())
	got, err := client.Annotate(context.Background(), "原文")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("instruction = %q", got)
	}
}

func TestRenderUsesUnicodeRuneIndexes(t *testing.T) {
	const original = "　“你好”，world"
	instruction, err := render(original, []styleRange{
		{Start: 1, End: 5, Style: "惊喜"},
		{Start: 6, End: 11, Style: "平静"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instruction, "“你好”") || !strings.Contains(instruction, "world") {
		t.Fatalf("instruction = %q", instruction)
	}
}

func TestRenderRejectsInvalidRanges(t *testing.T) {
	tests := []struct {
		name   string
		styles []styleRange
	}{
		{name: "nil", styles: nil},
		{name: "negative", styles: []styleRange{{Start: -1, End: 1, Style: "平静"}}},
		{name: "empty", styles: []styleRange{{Start: 1, End: 1, Style: "平静"}}},
		{name: "reversed", styles: []styleRange{{Start: 2, End: 1, Style: "平静"}}},
		{name: "out of bounds", styles: []styleRange{{Start: 0, End: 4, Style: "平静"}}},
		{name: "overlap", styles: []styleRange{{Start: 0, End: 2, Style: "平静"}, {Start: 1, End: 3, Style: "激动"}}},
		{name: "out of order", styles: []styleRange{{Start: 2, End: 3, Style: "激动"}, {Start: 0, End: 1, Style: "平静"}}},
		{name: "whitespace only", styles: []styleRange{{Start: 0, End: 1, Style: "平静"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := "中文啊"
			if tt.name == "whitespace only" {
				original = "　正文"
			}
			_, err := render(original, tt.styles)
			requireCategory(t, err, categoryInvalidRange)
		})
	}
}

func TestRenderRejectsTooManyStyles(t *testing.T) {
	styles := make([]styleRange, maxStyles+1)
	_, err := render(strings.Repeat("a", len(styles)), styles)
	requireCategory(t, err, categoryInvalidRange)
}

func TestRenderRejectsOversizedInstruction(t *testing.T) {
	style := strings.Repeat("情", maxStyleRunes)
	styles := make([]styleRange, maxStyles)
	var original strings.Builder
	position := 0
	for i := range styles {
		text := strings.Repeat("文", maxAnchorRunes) + string(rune('甲'+i%10))
		length := len([]rune(text))
		styles[i] = styleRange{Start: position, End: position + length, Style: style}
		position += length
		original.WriteString(text)
	}
	_, err := render(original.String(), styles)
	requireCategory(t, err, categoryInvalidStyle)
}

func TestAnnotateRejectsUnsafeStyle(t *testing.T) {
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return contentResponse(`{"styles":[{"start":0,"end":2,"style":"坏\n标签"}]}`), nil
	})}, testConfig())
	_, err := client.Annotate(context.Background(), "原文")
	requireCategory(t, err, categoryInvalidStyle)
}

func TestAnnotateLimitsConcurrency(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		entered <- struct{}{}
		<-release
		return contentResponse(`{"styles":[]}`), nil
	})}, testConfig())

	var wg sync.WaitGroup
	errorsCh := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.Annotate(context.Background(), "原文")
			errorsCh <- err
		}()
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter transport")
	}
	select {
	case <-entered:
		t.Fatal("second request entered before the first released")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnnotateTimeoutIncludesConcurrencyWait(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	var secondEntry ResponseLogEntry
	client := New(&http.Client{
		Timeout: 30 * time.Millisecond,
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			close(entered)
			<-release
			return contentResponse(`{"styles":[]}`), nil
		}),
	}, Config{
		Endpoint:         "https://emotion.invalid",
		APIKey:           "key",
		Model:            "model",
		MaxResponseBytes: 4096,
		LogResponse: func(_ context.Context, value ResponseLogEntry) {
			if value.Status == "error" {
				secondEntry = value
			}
		},
	})
	firstDone := make(chan error, 1)
	go func() {
		_, err := client.Annotate(context.Background(), "第一段")
		firstDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter transport")
	}
	started := time.Now()
	if _, err := client.Annotate(context.Background(), "第二段"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 300*time.Millisecond {
		t.Fatalf("queue timeout took %s", elapsed)
	}
	if calls.Load() != 1 || secondEntry.ErrorCategory != "timeout" || secondEntry.Attempts != 0 {
		t.Fatalf("calls = %d, entry = %#v", calls.Load(), secondEntry)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestAnnotateCancelsWhileWaitingForConcurrency(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	client := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return contentResponse(`{"styles":[]}`), nil
	})}, testConfig())

	firstDone := make(chan error, 1)
	go func() {
		_, err := client.Annotate(context.Background(), "第一段")
		firstDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter transport")
	}
	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		_, err := client.Annotate(ctx, "第二段")
		secondDone <- err
	}()
	cancel()
	select {
	case err := <-secondDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued request did not cancel")
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func requireCategory(t *testing.T, err error, want string) {
	t.Helper()
	var emotionErr *Error
	if !errors.As(err, &emotionErr) || emotionErr.Category != want {
		t.Fatalf("error = %v, want category %q", err, want)
	}
}

func testConfig() Config {
	return Config{
		Endpoint:         "https://emotion.invalid",
		APIKey:           "key",
		Model:            "model",
		MaxResponseBytes: 4096,
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func contentResponse(content string) *http.Response {
	body, _ := json.Marshal(response{
		Choices: []struct {
			Message message `json:"message"`
		}{{Message: message{Role: "assistant", Content: content}}},
	})
	return jsonResponse(string(body))
}

func jsonResponse(body string) *http.Response {
	return statusResponse(http.StatusOK, body)
}

func statusResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
