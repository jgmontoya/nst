package main

import (
	"context"
	"sync"

	"fiatjaf.com/nostr"
)

const (
	pubkeyBatchSize = 200
	maxConcurrent   = 3
)

// ExpandGraph performs concurrent traversal of the social graph starting from seed.
// It emits discovered nodes on the returned channel as soon as they're found,
// allowing search to start immediately while expansion continues at deeper radii.
// Cached follow lists are resolved synchronously; uncached follows are fetched
// in parallel batches with results feeding back into expansion immediately.
func ExpandGraph(ctx context.Context, pool *nostr.Pool, cache *Cache, seed nostr.PubKey, maxRadius int, relays []string) <-chan []GraphNode {
	out := make(chan []GraphNode, 64)

	go func() {
		defer close(out)

		var mu sync.Mutex
		seen := map[nostr.PubKey]bool{seed: true}
		var wg sync.WaitGroup
		sem := make(chan struct{}, maxConcurrent)

		// expand emits pubkeys for searching and recursively discovers their follows.
		// Cached follows are resolved immediately; uncached are fetched in parallel.
		var expand func([]nostr.PubKey, int)
		expand = func(pubkeys []nostr.PubKey, radius int) {
			nodes := make([]GraphNode, len(pubkeys))
			for i, pk := range pubkeys {
				nodes[i] = GraphNode{Pubkey: pk, Radius: radius}
			}
			logv("radius %d: %d pubkeys", radius, len(pubkeys))
			select {
			case out <- nodes:
			case <-ctx.Done():
				return
			}

			if radius >= maxRadius {
				return
			}

			// Split cached vs uncached follows
			var uncached []nostr.PubKey
			var cachedFollows []nostr.PubKey
			mu.Lock()
			for _, pk := range pubkeys {
				if cf, ok := cache.Follows[pk.Hex()]; ok {
					for _, hex := range cf.Pubkeys {
						if fpk, err := nostr.PubKeyFromHex(hex); err == nil {
							cachedFollows = append(cachedFollows, fpk)
						}
					}
				} else {
					uncached = append(uncached, pk)
				}
			}
			mu.Unlock()

			// Resolve cached follows immediately — no relay round-trip
			if len(cachedFollows) > 0 {
				var next []nostr.PubKey
				mu.Lock()
				for _, pk := range cachedFollows {
					if !seen[pk] {
						seen[pk] = true
						next = append(next, pk)
					}
				}
				mu.Unlock()
				if len(next) > 0 {
					logv("graph: %d new pubkeys from cached follows at radius %d", len(next), radius+1)
					expand(next, radius+1)
				}
			}

			if len(uncached) > 0 {
				logv("follows: %d cached, %d to fetch", len(pubkeys)-len(uncached), len(uncached))
			} else {
				logv("follows: all %d cached", len(pubkeys))
			}

			// Fetch uncached in parallel batches — sem limits concurrent relay requests
			for i := 0; i < len(uncached); i += pubkeyBatchSize {
				end := min(i+pubkeyBatchSize, len(uncached))
				batch := uncached[i:end]

				wg.Add(1)
				go func(authors []nostr.PubKey) {
					defer wg.Done()

					// Acquire sem only for the relay fetch, release before recursing
					select {
					case sem <- struct{}{}:
					case <-ctx.Done():
						return
					}
					logv("graph: fetching follows for %d authors from %d relays", len(authors), len(relays))
					fetched := fetchFollowsBatch(ctx, pool, authors, relays)
					logv("graph: got follow lists for %d/%d authors", len(fetched), len(authors))
					<-sem

					var next []nostr.PubKey
					mu.Lock()
					for authorHex, followHexes := range fetched {
						cache.Follows[authorHex] = CachedFollows{
							Pubkeys:   followHexes,
							FetchedAt: timeNow(),
						}
						for _, hex := range followHexes {
							if pk, err := nostr.PubKeyFromHex(hex); err == nil {
								if !seen[pk] {
									seen[pk] = true
									next = append(next, pk)
								}
							}
						}
					}
					for _, author := range authors {
						if _, ok := fetched[author.Hex()]; !ok {
							cache.Follows[author.Hex()] = CachedFollows{
								Pubkeys:   nil,
								FetchedAt: timeNow(),
							}
						}
					}
					mu.Unlock()

					if len(next) > 0 {
						logv("graph: %d new pubkeys from fetched follows at radius %d", len(next), radius+1)
						expand(next, radius+1)
					}
				}(batch)
			}
		}

		expand([]nostr.PubKey{seed}, 0)
		wg.Wait()
	}()

	return out
}

// fetchFollowsBatch fetches kind:3 (contact list) events for a batch of authors.
// Returns a map from author hex pubkey to their follow list (hex pubkeys).
func fetchFollowsBatch(ctx context.Context, pool *nostr.Pool, authors []nostr.PubKey, relays []string) map[string][]string {
	filter := nostr.Filter{
		Authors: authors,
		Kinds:   []nostr.Kind{3},
	}
	ch := pool.FetchMany(ctx, relays, filter, nostr.SubscriptionOptions{})

	latest := make(map[nostr.PubKey]nostr.Event)
	for ie := range ch {
		if ctx.Err() != nil {
			break
		}
		if prev, ok := latest[ie.PubKey]; !ok || ie.CreatedAt > prev.CreatedAt {
			latest[ie.PubKey] = ie.Event
		}
	}

	results := make(map[string][]string, len(latest))
	for authorPk, ev := range latest {
		var followHexes []string
		for _, tag := range ev.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				if _, err := nostr.PubKeyFromHex(tag[1]); err == nil {
					followHexes = append(followHexes, tag[1])
				}
			}
		}
		results[authorPk.Hex()] = followHexes
	}

	return results
}
