package limits

import (
	"context"
	"sync"
	"time"
)

type Gate struct {
	semaphore chan struct{}
	rate      float64
	burst     float64

	mu     sync.Mutex
	tokens float64
	last   time.Time
}

func New(maxConcurrency int, ratePerSecond float64, burst int) *Gate {
	now := time.Now()
	return &Gate{
		semaphore: make(chan struct{}, maxConcurrency),
		rate:      ratePerSecond,
		burst:     float64(burst),
		tokens:    float64(burst),
		last:      now,
	}
}

func (g *Gate) AcquireConcurrency(ctx context.Context) (func(), error) {
	select {
	case g.semaphore <- struct{}{}:
		return func() { <-g.semaphore }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (g *Gate) WaitRate(ctx context.Context) error {
	for {
		wait := g.reserve(time.Now())
		if wait <= 0 {
			return nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		}
	}
}

func (g *Gate) reserve(now time.Time) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()

	elapsed := now.Sub(g.last).Seconds()
	if elapsed > 0 {
		g.tokens += elapsed * g.rate
		if g.tokens > g.burst {
			g.tokens = g.burst
		}
		g.last = now
	}
	if g.tokens >= 1 {
		g.tokens--
		return 0
	}
	return time.Duration((1 - g.tokens) / g.rate * float64(time.Second))
}
