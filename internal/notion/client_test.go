package notion

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestNew(t *testing.T) {
	client := New("test-token")

	if client == nil {
		t.Fatal("New() returned nil")
	}

	if client.api == nil {
		t.Error("api client is nil")
	}

	if client.limiter == nil {
		t.Error("limiter is nil")
	}

	if client.batchSize != DefaultBatchSize {
		t.Errorf("batchSize = %d, expected %d", client.batchSize, DefaultBatchSize)
	}
}

func TestNewWithOptions(t *testing.T) {
	client := New("test-token",
		WithRateLimit(5.0),
		WithBatchSize(50),
	)

	if client == nil {
		t.Fatal("New() returned nil")
	}

	if client.batchSize != 50 {
		t.Errorf("batchSize = %d, expected 50", client.batchSize)
	}

	// Verify rate limit by checking the limiter's limit.
	// Note: rate.Limiter doesn't expose its limit directly,
	// so we just verify the limiter was set.
	if client.limiter == nil {
		t.Error("limiter is nil after WithRateLimit")
	}
}

func TestWithRateLimit(t *testing.T) {
	tests := []struct {
		name string
		rps  float64
	}{
		{"low rate", 1.0},
		{"default rate", 3.0},
		{"high rate", 10.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New("test-token", WithRateLimit(tt.rps))

			if client.limiter == nil {
				t.Error("limiter is nil")
			}
		})
	}
}

func TestWithBatchSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		expected int
	}{
		{"small batch", 10, 10},
		{"default batch", 100, 100},
		{"large batch", 200, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New("test-token", WithBatchSize(tt.size))

			if client.batchSize != tt.expected {
				t.Errorf("batchSize = %d, expected %d", client.batchSize, tt.expected)
			}
		})
	}
}

func TestWait(t *testing.T) {
	// Create a client with a very high rate limit to avoid blocking.
	client := New("test-token", WithRateLimit(1000))

	ctx := context.Background()

	// First call should succeed immediately.
	start := time.Now()
	err := client.wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("wait() error = %v", err)
	}

	// Should be nearly instant with high rate limit.
	if elapsed > 100*time.Millisecond {
		t.Errorf("wait() took too long: %v", elapsed)
	}
}

func TestWaitWithContextCancellation(t *testing.T) {
	// Create a client with a very low rate limit.
	client := &Client{
		limiter: rate.NewLimiter(rate.Every(10*time.Second), 1),
	}

	// Consume the burst.
	ctx := context.Background()
	_ = client.limiter.Allow()

	// Create a context that cancels quickly.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()

	// Wait should fail due to context cancellation.
	err := client.wait(ctx)
	if err == nil {
		t.Error("expected error from cancelled context, got nil")
	}
}

func TestAPI(t *testing.T) {
	client := New("test-token")

	api := client.API()
	if api == nil {
		t.Error("API() returned nil")
	}

	if api != client.api {
		t.Error("API() returned different client than internal api")
	}
}

func TestDefaultConstants(t *testing.T) {
	if DefaultRateLimit != 3 {
		t.Errorf("DefaultRateLimit = %d, expected 3", DefaultRateLimit)
	}

	if DefaultBatchSize != 100 {
		t.Errorf("DefaultBatchSize = %d, expected 100", DefaultBatchSize)
	}
}
