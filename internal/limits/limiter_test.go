package limits

import (
	"context"
	"testing"
	"time"
)

func TestGateCancellation(t *testing.T) {
	gate := New(1, 0.01, 1)
	release, err := gate.AcquireConcurrency(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := gate.AcquireConcurrency(ctx); err == nil {
		t.Fatal("expected cancellation while waiting for the semaphore")
	}
}

func TestGateRateLimit(t *testing.T) {
	gate := New(1, 100, 1)
	if err := gate.WaitRate(context.Background()); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := gate.WaitRate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 5*time.Millisecond {
		t.Fatalf("rate limiter waited only %s", elapsed)
	}
}
