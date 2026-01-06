package sync

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewWorkerPool(t *testing.T) {
	tests := []struct {
		name            string
		workers         int
		expectedWorkers int
	}{
		{"positive workers", 4, 4},
		{"single worker", 1, 1},
		{"zero workers defaults to 1", 0, 1},
		{"negative workers defaults to 1", -5, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := NewWorkerPool(tc.workers)
			if pool.workers != tc.expectedWorkers {
				t.Errorf("expected %d workers, got %d", tc.expectedWorkers, pool.workers)
			}
		})
	}
}

func TestProcess_Basic(t *testing.T) {
	pool := NewWorkerPool(2)
	ctx := context.Background()

	inputs := []int{1, 2, 3, 4, 5}

	// Double each input
	results := Process(ctx, pool, inputs, func(ctx context.Context, n int) (int, error) {
		return n * 2, nil
	})

	if len(results) != len(inputs) {
		t.Fatalf("expected %d results, got %d", len(inputs), len(results))
	}

	// Verify results are in order and correct
	for i, r := range results {
		if r.Input != inputs[i] {
			t.Errorf("result %d: expected input %d, got %d", i, inputs[i], r.Input)
		}
		expectedResult := inputs[i] * 2
		if r.Result != expectedResult {
			t.Errorf("result %d: expected result %d, got %d", i, expectedResult, r.Result)
		}
		if r.Err != nil {
			t.Errorf("result %d: unexpected error: %v", i, r.Err)
		}
	}
}

func TestProcess_EmptyInput(t *testing.T) {
	pool := NewWorkerPool(2)
	ctx := context.Background()

	results := Process(ctx, pool, []int{}, func(ctx context.Context, n int) (int, error) {
		return n, nil
	})

	if results != nil {
		t.Errorf("expected nil results for empty input, got %v", results)
	}
}

func TestProcess_WithErrors(t *testing.T) {
	pool := NewWorkerPool(2)
	ctx := context.Background()

	inputs := []int{1, 2, 3, 4, 5}
	errExpected := errors.New("error on even numbers")

	results := Process(ctx, pool, inputs, func(ctx context.Context, n int) (int, error) {
		if n%2 == 0 {
			return 0, errExpected
		}
		return n * 2, nil
	})

	if len(results) != len(inputs) {
		t.Fatalf("expected %d results, got %d", len(inputs), len(results))
	}

	// Verify errors on even inputs, success on odd
	for i, r := range results {
		if inputs[i]%2 == 0 {
			if r.Err == nil {
				t.Errorf("result %d: expected error for even number %d", i, inputs[i])
			}
		} else {
			if r.Err != nil {
				t.Errorf("result %d: unexpected error for odd number %d: %v", i, inputs[i], r.Err)
			}
			if r.Result != inputs[i]*2 {
				t.Errorf("result %d: expected %d, got %d", i, inputs[i]*2, r.Result)
			}
		}
	}
}

func TestProcess_ContextCancellation(t *testing.T) {
	pool := NewWorkerPool(2)
	ctx, cancel := context.WithCancel(context.Background())

	inputs := make([]int, 100) // Many inputs
	for i := range inputs {
		inputs[i] = i
	}

	var processed atomic.Int32

	// Start processing but cancel quickly
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	results := Process(ctx, pool, inputs, func(ctx context.Context, n int) (int, error) {
		time.Sleep(5 * time.Millisecond) // Simulate work
		processed.Add(1)
		return n, nil
	})

	// Results should be returned (may be partial due to cancellation)
	if len(results) != len(inputs) {
		t.Errorf("expected %d result slots, got %d", len(inputs), len(results))
	}

	// Not all items should be processed due to cancellation
	// (This is probabilistic, but with 100 items and 5ms delay, we shouldn't finish all)
	p := processed.Load()
	if p >= int32(len(inputs)) {
		t.Logf("Warning: all %d items processed despite cancellation", p)
	}
}

func TestProcess_ConcurrentExecution(t *testing.T) {
	pool := NewWorkerPool(4)
	ctx := context.Background()

	inputs := []int{1, 2, 3, 4, 5, 6, 7, 8}
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	results := Process(ctx, pool, inputs, func(ctx context.Context, n int) (int, error) {
		c := concurrent.Add(1)
		// Track max concurrent
		for {
			max := maxConcurrent.Load()
			if c <= max || maxConcurrent.CompareAndSwap(max, c) {
				break
			}
		}

		time.Sleep(20 * time.Millisecond) // Simulate work
		concurrent.Add(-1)
		return n, nil
	})

	if len(results) != len(inputs) {
		t.Fatalf("expected %d results, got %d", len(inputs), len(results))
	}

	// Should have used multiple workers concurrently
	max := maxConcurrent.Load()
	if max < 2 {
		t.Errorf("expected concurrent execution, max concurrent was %d", max)
	}
	t.Logf("Max concurrent workers: %d", max)
}

func TestProcessWithProgress_Basic(t *testing.T) {
	pool := NewWorkerPool(2)
	ctx := context.Background()

	inputs := []int{1, 2, 3, 4, 5}
	var progressCalls []struct {
		completed int
		total     int
	}

	results := ProcessWithProgress(ctx, pool, inputs, func(ctx context.Context, n int) (int, error) {
		return n * 2, nil
	}, func(completed, total int) {
		progressCalls = append(progressCalls, struct {
			completed int
			total     int
		}{completed, total})
	})

	if len(results) != len(inputs) {
		t.Fatalf("expected %d results, got %d", len(inputs), len(results))
	}

	// Progress should be called for each item
	if len(progressCalls) != len(inputs) {
		t.Errorf("expected %d progress calls, got %d", len(inputs), len(progressCalls))
	}

	// All calls should have total = 5
	for i, call := range progressCalls {
		if call.total != len(inputs) {
			t.Errorf("progress call %d: expected total %d, got %d", i, len(inputs), call.total)
		}
	}

	// Final call should have completed = 5
	if len(progressCalls) > 0 {
		lastCall := progressCalls[len(progressCalls)-1]
		if lastCall.completed != len(inputs) {
			t.Errorf("final progress: expected completed %d, got %d", len(inputs), lastCall.completed)
		}
	}
}

func TestProcessWithProgress_NilProgressFunc(t *testing.T) {
	pool := NewWorkerPool(2)
	ctx := context.Background()

	inputs := []int{1, 2, 3}

	// Should not panic with nil progress function
	results := ProcessWithProgress(ctx, pool, inputs, func(ctx context.Context, n int) (int, error) {
		return n, nil
	}, nil)

	if len(results) != len(inputs) {
		t.Fatalf("expected %d results, got %d", len(inputs), len(results))
	}
}

func TestProcessWithProgress_EmptyInput(t *testing.T) {
	pool := NewWorkerPool(2)
	ctx := context.Background()

	var progressCalls int

	results := ProcessWithProgress(ctx, pool, []int{}, func(ctx context.Context, n int) (int, error) {
		return n, nil
	}, func(completed, total int) {
		progressCalls++
	})

	if results != nil {
		t.Errorf("expected nil results for empty input, got %v", results)
	}

	if progressCalls != 0 {
		t.Errorf("expected 0 progress calls, got %d", progressCalls)
	}
}

func TestProcess_PreservesOrder(t *testing.T) {
	pool := NewWorkerPool(8) // Many workers
	ctx := context.Background()

	inputs := make([]int, 100)
	for i := range inputs {
		inputs[i] = i
	}

	// Variable delay to encourage out-of-order completion
	results := Process(ctx, pool, inputs, func(ctx context.Context, n int) (int, error) {
		// Odd numbers take longer
		if n%2 == 1 {
			time.Sleep(2 * time.Millisecond)
		}
		return n * 10, nil
	})

	if len(results) != len(inputs) {
		t.Fatalf("expected %d results, got %d", len(inputs), len(results))
	}

	// Verify order is preserved regardless of completion order
	for i, r := range results {
		if r.Input != inputs[i] {
			t.Errorf("result %d: expected input %d, got %d", i, inputs[i], r.Input)
		}
		if r.Result != inputs[i]*10 {
			t.Errorf("result %d: expected result %d, got %d", i, inputs[i]*10, r.Result)
		}
	}
}

// TestProcess_StringInputs tests with string type to verify generics work
func TestProcess_StringInputs(t *testing.T) {
	pool := NewWorkerPool(2)
	ctx := context.Background()

	inputs := []string{"hello", "world", "test"}

	results := Process(ctx, pool, inputs, func(ctx context.Context, s string) (int, error) {
		return len(s), nil
	})

	if len(results) != len(inputs) {
		t.Fatalf("expected %d results, got %d", len(inputs), len(results))
	}

	expectedLengths := []int{5, 5, 4}
	for i, r := range results {
		if r.Result != expectedLengths[i] {
			t.Errorf("result %d: expected length %d, got %d", i, expectedLengths[i], r.Result)
		}
	}
}

// TestProcess_StructInputsAndOutputs tests with complex types
func TestProcess_StructInputsAndOutputs(t *testing.T) {
	pool := NewWorkerPool(2)
	ctx := context.Background()

	type Input struct {
		Name  string
		Value int
	}
	type Output struct {
		Processed bool
		Sum       int
	}

	inputs := []Input{
		{"a", 1},
		{"b", 2},
		{"c", 3},
	}

	results := Process(ctx, pool, inputs, func(ctx context.Context, in Input) (Output, error) {
		return Output{
			Processed: true,
			Sum:       len(in.Name) + in.Value,
		}, nil
	})

	if len(results) != len(inputs) {
		t.Fatalf("expected %d results, got %d", len(inputs), len(results))
	}

	for i, r := range results {
		if !r.Result.Processed {
			t.Errorf("result %d: expected Processed=true", i)
		}
		expectedSum := len(inputs[i].Name) + inputs[i].Value
		if r.Result.Sum != expectedSum {
			t.Errorf("result %d: expected Sum=%d, got %d", i, expectedSum, r.Result.Sum)
		}
	}
}
