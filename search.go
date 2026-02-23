package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"fiatjaf.com/nostr"
)

// Search consumes graph nodes from expansion and concurrently fetches and matches
// events. Candidates are emitted to the returned channel as they're found. The
// caller is responsible for limit enforcement and zap filtering. Call the cleanup
// function before saving cache to wait for in-flight goroutines.
func Search(ctx context.Context, pool *nostr.Pool, cache *Cache, nodesCh <-chan []GraphNode, queryStr string, kind string, since int64, relays []string) (<-chan SearchResult, func()) {
	out := make(chan SearchResult, 64)

	query, err := Parse(queryStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid query: %v\n", err)
		close(out)
		return out, func() {}
	}
	logv("search: parsed query %q", queryStr)

	ctx, cancel := context.WithCancel(ctx)

	var kinds []nostr.Kind
	var matcher func(nostr.Event, Query, int) (SearchResult, bool)
	switch kind {
	case "post":
		kinds = []nostr.Kind{1}
		matcher = matchPost
	case "profile":
		kinds = []nostr.Kind{0}
		matcher = matchProfile
	}

	emit := func(r SearchResult) {
		select {
		case out <- r:
		case <-ctx.Done():
		}
	}

	go func() {
		defer close(out)

		var mu sync.Mutex // protects cache map writes
		var wg sync.WaitGroup
		sem := make(chan struct{}, maxConcurrent)

		for radiusBatch := range nodesCh {
			if ctx.Err() != nil {
				break
			}

			radiusByPubkey := make(map[nostr.PubKey]int, len(radiusBatch))
			pubkeys := make([]nostr.PubKey, len(radiusBatch))
			for i, n := range radiusBatch {
				radiusByPubkey[n.Pubkey] = n.Radius
				pubkeys[i] = n.Pubkey
			}

			radius := radiusBatch[0].Radius

			// Match cached entries, collect uncached pubkeys for relay fetch
			var uncached []nostr.PubKey
			mu.Lock()
			if kind == "profile" {
				for _, pk := range pubkeys {
					if ctx.Err() != nil {
						break
					}
					if cached, ok := cache.Profiles[pk.Hex()]; ok {
						if r, ok := matchCachedProfile(pk, cached, query, radius); ok {
							logv("search: cached match %s (quality=%s field=%s)", r.Pubkey[:12], r.MatchQuality, r.MatchedField)
							mu.Unlock()
							emit(r)
							mu.Lock()
						}
					} else {
						uncached = append(uncached, pk)
					}
				}
			} else if kind == "post" {
				for _, pk := range pubkeys {
					if ctx.Err() != nil {
						break
					}
					if cached, ok := cache.Posts[pk.Hex()]; ok {
						for _, cp := range cached.Posts {
							if since > 0 && cp.CreatedAt < since {
								continue
							}
							if r, ok := matchCachedPost(pk, cp, query, radius); ok {
								logv("search: cached match %s (quality=%s field=%s)", r.Pubkey[:12], r.MatchQuality, r.MatchedField)
								mu.Unlock()
								emit(r)
								mu.Lock()
							}
						}
					} else {
						uncached = append(uncached, pk)
					}
				}
			} else {
				uncached = pubkeys
			}
			mu.Unlock()

			logv("searching radius %d: %d cached, %d to fetch", radius, len(pubkeys)-len(uncached), len(uncached))
			pubkeys = uncached

			for i := 0; i < len(pubkeys); i += pubkeyBatchSize {
				if ctx.Err() != nil {
					logv("search: skipping remaining batches (context cancelled)")
					break
				}

				end := min(i+pubkeyBatchSize, len(pubkeys))
				batch := pubkeys[i:end]

				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					logv("search: skipping remaining batches (context cancelled)")
					goto batchesDone
				}

				wg.Add(1)
				go func(authors []nostr.PubKey, rmap map[nostr.PubKey]int, k string) {
					defer wg.Done()
					defer func() { <-sem }()

					filter := nostr.Filter{
						Authors: authors,
						Kinds:   kinds,
						Limit:   len(authors) * 10,
					}
					if since > 0 {
						filter.Since = nostr.Timestamp(since)
					}

					logv("search: fetching %ss for %d authors", k, len(authors))
					ch := pool.FetchMany(ctx, relays, filter, nostr.SubscriptionOptions{})

					for ie := range ch {
						if ctx.Err() != nil {
							break
						}
						if since > 0 && int64(ie.CreatedAt) < since {
							continue
						}

						mu.Lock()
						if k == "profile" {
							cache.Profiles[ie.PubKey.Hex()] = CachedProfile{
								Content:   ie.Content,
								EventID:   ie.ID.Hex(),
								CreatedAt: int64(ie.CreatedAt),
								FetchedAt: timeNow(),
							}
						} else if k == "post" {
							pkHex := ie.PubKey.Hex()
							cp := cache.Posts[pkHex]
							cp.FetchedAt = timeNow()
							cp.Posts = append(cp.Posts, CachedPost{
								Content:   ie.Content,
								EventID:   ie.ID.Hex(),
								CreatedAt: int64(ie.CreatedAt),
							})
							cache.Posts[pkHex] = cp
						}
						mu.Unlock()

						if r, ok := matcher(ie.Event, query, rmap[ie.PubKey]); ok {
							logv("search: match %s (quality=%s field=%s)", ie.PubKey.Hex()[:12], r.MatchQuality, r.MatchedField)
							emit(r)
						}
					}
				}(batch, radiusByPubkey, kind)
			}
		batchesDone:
		}

		wg.Wait()
	}()

	cleanup := func() {
		cancel()
		for range nodesCh {
		}
		logv("search: cleanup complete")
	}

	return out, cleanup
}
