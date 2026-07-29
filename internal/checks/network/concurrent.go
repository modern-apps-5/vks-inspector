package network

import (
	"context"
	"sync"
)

// probeConcurrency bounds how many probes a single check has in flight.
//
// Network probes are latency-bound, not CPU-bound: a config with seven
// endpoints and two resolvers is fourteen DNS lookups, and doing them in
// sequence at a five-second timeout means an operator waits over a minute for a
// check that could finish in five seconds. Bounded rather than unlimited so the
// tool never looks like a scan to a customer's network monitoring.
const probeConcurrency = 8

// mapConcurrent applies fn to every item with bounded concurrency, preserving
// input order in the output.
//
// Order preservation matters: report rows and baseline artifacts must be
// deterministic, and results that arrive in completion order would reshuffle
// every run and make drift diffs meaningless.
func mapConcurrent[T, R any](ctx context.Context, items []T, fn func(context.Context, T) R) []R {
	out := make([]R, len(items))
	if len(items) == 0 {
		return out
	}
	if len(items) == 1 {
		out[0] = fn(ctx, items[0])
		return out
	}

	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup

	for i := range items {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			out[i] = fn(ctx, items[i])
		}(i)
	}
	wg.Wait()
	return out
}
