// Package sync provides parallel processing utilities for vault synchronization.
package sync

import (
	"context"
	"sync"
)

// WorkerPool manages a pool of workers for parallel task execution.
type WorkerPool struct {
	workers int
}

// NewWorkerPool creates a new worker pool with the specified number of workers.
func NewWorkerPool(workers int) *WorkerPool {
	if workers < 1 {
		workers = 1
	}
	return &WorkerPool{
		workers: workers,
	}
}

// Task represents a unit of work to be processed.
type Task[T any, R any] struct {
	Input  T
	Result R
	Err    error
}

// Process executes tasks in parallel using the worker pool.
// It returns results in the same order as inputs.
func Process[T any, R any](ctx context.Context, pool *WorkerPool, inputs []T, fn func(context.Context, T) (R, error)) []Task[T, R] {
	if len(inputs) == 0 {
		return nil
	}

	// Create channels for work distribution.
	type indexedInput struct {
		index int
		input T
	}
	type indexedResult struct {
		index  int
		result R
		err    error
	}

	inputCh := make(chan indexedInput, len(inputs))
	resultCh := make(chan indexedResult, len(inputs))

	// Start workers.
	var wg sync.WaitGroup
	for i := 0; i < pool.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-inputCh:
					if !ok {
						return
					}
					result, err := fn(ctx, item.input)
					resultCh <- indexedResult{
						index:  item.index,
						result: result,
						err:    err,
					}
				}
			}
		}()
	}

	// Send inputs to workers.
	go func() {
		for i, input := range inputs {
			select {
			case <-ctx.Done():
				break
			case inputCh <- indexedInput{index: i, input: input}:
			}
		}
		close(inputCh)
	}()

	// Wait for workers to finish and close result channel.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results in order.
	results := make([]Task[T, R], len(inputs))
	for i := range inputs {
		results[i].Input = inputs[i]
	}

	for result := range resultCh {
		results[result.index].Result = result.result
		results[result.index].Err = result.err
	}

	return results
}

// ProcessWithProgress executes tasks in parallel and reports progress.
func ProcessWithProgress[T any, R any](
	ctx context.Context,
	pool *WorkerPool,
	inputs []T,
	fn func(context.Context, T) (R, error),
	progress func(completed, total int),
) []Task[T, R] {
	if len(inputs) == 0 {
		return nil
	}

	// Create channels for work distribution.
	type indexedInput struct {
		index int
		input T
	}
	type indexedResult struct {
		index  int
		result R
		err    error
	}

	inputCh := make(chan indexedInput, len(inputs))
	resultCh := make(chan indexedResult, len(inputs))

	// Start workers.
	var wg sync.WaitGroup
	for i := 0; i < pool.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-inputCh:
					if !ok {
						return
					}
					result, err := fn(ctx, item.input)
					resultCh <- indexedResult{
						index:  item.index,
						result: result,
						err:    err,
					}
				}
			}
		}()
	}

	// Send inputs to workers.
	go func() {
		for i, input := range inputs {
			select {
			case <-ctx.Done():
				break
			case inputCh <- indexedInput{index: i, input: input}:
			}
		}
		close(inputCh)
	}()

	// Wait for workers to finish and close result channel.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results in order and report progress.
	results := make([]Task[T, R], len(inputs))
	for i := range inputs {
		results[i].Input = inputs[i]
	}

	completed := 0
	total := len(inputs)
	for result := range resultCh {
		results[result.index].Result = result.result
		results[result.index].Err = result.err
		completed++
		if progress != nil {
			progress(completed, total)
		}
	}

	return results
}
