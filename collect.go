package main

import (
	"context"
	"sync"
	"time"

	"fiatjaf.com/nostr"
)

const zapBatchSize = 50

// CollectResults reads search candidates from the channel and returns up to
// limit results. When minZaps > 0, candidates are batched and zap amounts are
// fetched in parallel workers — this runs concurrently with Search still
// producing candidates. When limit is reached, stopSearch is called to cancel
// the search pipeline.
func CollectResults(pool *nostr.Pool, cache *Cache, candidatesCh <-chan SearchResult, stopSearch context.CancelFunc, minZaps int64, limit int, timeout time.Duration, relays []string) []SearchResult {
	if minZaps <= 0 {
		return collectSimple(candidatesCh, stopSearch, limit)
	}
	return collectWithZaps(pool, cache, candidatesCh, stopSearch, minZaps, limit, timeout, relays)
}

// collectSimple collects candidates directly with limit enforcement.
func collectSimple(candidatesCh <-chan SearchResult, stopSearch context.CancelFunc, limit int) []SearchResult {
	var results []SearchResult
	for r := range candidatesCh {
		results = append(results, r)
		if limit > 0 && len(results) >= limit {
			logv("collect: limit %d reached, stopping search", limit)
			stopSearch()
			break
		}
	}
	sortResults(results)
	return results
}

// collectWithZaps runs a parallel zap filtering pipeline:
//
//	candidatesCh → [dispatcher] → batch → [zap workers] → filteredCh → results
//
// Multiple zap workers fetch zap amounts concurrently while Search continues
// producing candidates.
func collectWithZaps(pool *nostr.Pool, cache *Cache, candidatesCh <-chan SearchResult, stopSearch context.CancelFunc, minZaps int64, limit int, timeout time.Duration, relays []string) []SearchResult {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	filteredCh := make(chan SearchResult, 64)
	var wg sync.WaitGroup
	var zapMu sync.Mutex // protects cache.Zaps across parallel workers
	sem := make(chan struct{}, maxConcurrent)

	dispatchBatch := func(batch []SearchResult) {
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			eventIDs := make([]string, 0, len(batch))
			for _, r := range batch {
				if r.EventID != "" {
					eventIDs = append(eventIDs, r.EventID)
				}
			}

			zapCtx, zapCancel := context.WithTimeout(ctx, timeout)
			defer zapCancel()
			amounts := fetchZapAmounts(zapCtx, pool, cache, &zapMu, eventIDs, relays)

			for i := range batch {
				batch[i].ZapAmount = amounts[batch[i].EventID]
				if batch[i].ZapAmount >= minZaps {
					select {
					case filteredCh <- batch[i]:
					case <-ctx.Done():
						return
					}
				}
			}
			logv("zaps: batch %d candidates → %d passed min-zaps=%d",
				len(batch), countPassing(batch, minZaps), minZaps)
		}()
	}

	// Dispatcher: read candidates, batch, dispatch to workers.
	// Uses a short flush timer so small batches don't wait for the full
	// zapBatchSize — critical for cached runs where few candidates match.
	go func() {
		var batch []SearchResult
		flush := time.NewTimer(0)
		flush.Stop()

		for {
			select {
			case r, ok := <-candidatesCh:
				if !ok {
					if len(batch) > 0 {
						dispatchBatch(batch)
					}
					wg.Wait()
					close(filteredCh)
					return
				}
				if ctx.Err() != nil {
					for range candidatesCh {
					}
					wg.Wait()
					close(filteredCh)
					return
				}
				batch = append(batch, r)
				if len(batch) >= zapBatchSize {
					flush.Stop()
					dispatchBatch(batch)
					batch = nil
				} else if len(batch) == 1 {
					flush.Reset(50 * time.Millisecond)
				}
			case <-flush.C:
				if len(batch) > 0 {
					dispatchBatch(batch)
					batch = nil
				}
			}
		}
	}()

	// Collect filtered results
	var results []SearchResult
	for r := range filteredCh {
		results = append(results, r)
		if limit > 0 && len(results) >= limit {
			logv("collect: limit %d reached with zap filtering, stopping", limit)
			stopSearch()
			cancel()
			break
		}
	}
	// Drain remaining to allow dispatcher goroutine to finish
	for range filteredCh {
	}

	sortResults(results)
	return results
}

func countPassing(batch []SearchResult, minZaps int64) int {
	n := 0
	for _, r := range batch {
		if r.ZapAmount >= minZaps {
			n++
		}
	}
	return n
}
