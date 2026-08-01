package upstream

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var ErrProtocol = errors.New("invalid provider response")

type Error struct {
	StatusCode int
	Category   string
	Retryable  bool
	Cause      error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Category + ": " + e.Cause.Error()
	}
	return e.Category
}

func (e *Error) Unwrap() error { return e.Cause }

type Config struct {
	Endpoint         string
	APIKey           string
	Model            string
	MaxResponseBytes int64
	MaxAudioBytes    int
	MaxRetries       int
	MaxRetryDelay    time.Duration
}

type Client struct {
	httpClient *http.Client
	config     Config
	waitRate   func(context.Context) error
	sleep      func(context.Context, time.Duration) error
}

type synthesisRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Audio    audio     `json:"audio"`
	Stream   bool      `json:"stream"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type audio struct {
	Format string `json:"format"`
	Voice  string `json:"voice"`
}

type synthesisResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Audio struct {
				Data string `json:"data"`
			} `json:"audio"`
		} `json:"message"`
	} `json:"choices"`
}

func New(httpClient *http.Client, config Config, waitRate func(context.Context) error) *Client {
	if waitRate == nil {
		waitRate = func(context.Context) error { return nil }
	}
	return &Client{httpClient: httpClient, config: config, waitRate: waitRate, sleep: sleepContext}
}

func (c *Client) Synthesize(ctx context.Context, text, voice string, speed int) ([]byte, error) {
	if c.httpClient.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.httpClient.Timeout)
		defer cancel()
	}
	payload, err := json.Marshal(synthesisRequest{
		Model: c.config.Model,
		Messages: []message{
			{Role: "user", Content: SpeedInstruction(speed)},
			{Role: "assistant", Content: text},
		},
		Audio:  audio{Format: "mp3", Voice: voice},
		Stream: false,
	})
	if err != nil {
		return nil, fmt.Errorf("encode provider request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := c.backoff(attempt, lastErr)
			if err := c.sleep(ctx, delay); err != nil {
				return nil, err
			}
		}
		if err := c.waitRate(ctx); err != nil {
			return nil, err
		}
		result, err := c.attempt(ctx, payload)
		if err == nil {
			return result, nil
		}
		lastErr = err
		var providerErr *Error
		if !errors.As(err, &providerErr) || !providerErr.Retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) attempt(ctx context.Context, payload []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, &Error{Category: "provider_request_error", Cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "mimo-tts-adapter/1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !retryableTransportError(err) {
			return nil, &Error{Category: "provider_unavailable", Cause: err}
		}
		return nil, &Error{Category: "provider_unavailable", Retryable: true, Cause: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, c.config.MaxResponseBytes+1))
	if err != nil {
		return nil, &Error{Category: "provider_unavailable", Retryable: true, Cause: err}
	}
	if int64(len(body)) > c.config.MaxResponseBytes {
		return nil, &Error{StatusCode: resp.StatusCode, Category: "provider_protocol_error", Cause: errors.New("provider response too large")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, providerStatusError(resp.StatusCode, resp.Header.Get("Retry-After"))
	}

	var decoded synthesisResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, &Error{StatusCode: resp.StatusCode, Category: "provider_protocol_error", Cause: ErrProtocol}
	}
	if len(decoded.Choices) == 0 || decoded.Choices[0].FinishReason != "stop" {
		return nil, &Error{StatusCode: resp.StatusCode, Category: "provider_protocol_error", Cause: ErrProtocol}
	}
	encoded := decoded.Choices[0].Message.Audio.Data
	if encoded == "" || base64.StdEncoding.DecodedLen(len(encoded)) > c.config.MaxAudioBytes {
		return nil, &Error{StatusCode: resp.StatusCode, Category: "provider_protocol_error", Cause: ErrProtocol}
	}
	audioBytes, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(audioBytes) == 0 || len(audioBytes) > c.config.MaxAudioBytes {
		return nil, &Error{StatusCode: resp.StatusCode, Category: "provider_protocol_error", Cause: ErrProtocol}
	}
	return audioBytes, nil
}

func SpeedInstruction(speed int) string {
	switch {
	case speed <= 10:
		return "请用很慢的语速朗读，保持自然停顿。"
	case speed <= 20:
		return "请用稍慢的语速朗读，保持自然停顿。"
	case speed <= 30:
		return "请用正常语速自然朗读。"
	case speed <= 40:
		return "请用稍快的语速清晰朗读。"
	default:
		return "请用很快但仍清晰的语速朗读。"
	}
}

func providerStatusError(status int, retryAfter string) *Error {
	category := "provider_rejected"
	retryable := false
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		category = "provider_unavailable"
		retryable = true
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden, 421:
		category = "provider_rejected"
	case http.StatusBadRequest:
		category = "provider_request_error"
	default:
		if status >= 500 {
			category = "provider_unavailable"
		}
	}
	return &Error{StatusCode: status, Category: category, Retryable: retryable, Cause: retryAfterError(retryAfter)}
}

type retryDelayError struct{ delay time.Duration }

func (e retryDelayError) Error() string { return "provider requested retry delay" }

func retryAfterError(value string) error {
	if value == "" {
		return nil
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return retryDelayError{delay: time.Duration(seconds) * time.Second}
	}
	if when, err := http.ParseTime(value); err == nil {
		return retryDelayError{delay: time.Until(when)}
	}
	return nil
}

func (c *Client) backoff(attempt int, err error) time.Duration {
	var providerErr *Error
	if errors.As(err, &providerErr) {
		var requested retryDelayError
		if errors.As(providerErr.Cause, &requested) {
			if requested.delay < 0 {
				return 0
			}
			return min(requested.delay, c.config.MaxRetryDelay)
		}
	}
	delay := time.Duration(1<<min(attempt-1, 5)) * 250 * time.Millisecond
	jitter := time.Duration(rand.Int64N(int64(delay/2 + 1)))
	return min(delay+jitter, c.config.MaxRetryDelay)
}

func retryableTransportError(err error) bool {
	var networkErr net.Error
	if !errors.As(err, &networkErr) {
		return false
	}
	return networkErr.Timeout() || networkErr.Temporary()
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
