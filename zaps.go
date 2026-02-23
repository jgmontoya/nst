package main

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"

	"fiatjaf.com/nostr"
)

// fetchZapAmounts returns total sats zapped per event ID, using cached values
// where available and fetching uncached from relays. Results are written back
// to cache. The mutex protects concurrent access to cache.Zaps.
func fetchZapAmounts(ctx context.Context, pool *nostr.Pool, cache *Cache, zapMu *sync.Mutex, eventIDs []string, relays []string) map[string]int64 {
	amounts := make(map[string]int64, len(eventIDs))
	if len(eventIDs) == 0 {
		return amounts
	}

	// Resolve cached entries
	var uncached []string
	zapMu.Lock()
	for _, id := range eventIDs {
		if cz, ok := cache.Zaps[id]; ok {
			amounts[id] = cz.Amount
		} else {
			uncached = append(uncached, id)
		}
	}
	zapMu.Unlock()
	logv("zaps: %d cached, %d to fetch", len(eventIDs)-len(uncached), len(uncached))

	if len(uncached) == 0 {
		return amounts
	}

	for i := 0; i < len(uncached); i += pubkeyBatchSize {
		if ctx.Err() != nil {
			break
		}

		end := min(i+pubkeyBatchSize, len(uncached))
		batch := uncached[i:end]

		filter := nostr.Filter{
			Kinds: []nostr.Kind{9735},
			Tags:  nostr.TagMap{"e": batch},
		}

		logv("zaps: fetching zap receipts for %d events", len(batch))
		ch := pool.FetchMany(ctx, relays, filter, nostr.SubscriptionOptions{})

		for ie := range ch {
			if ctx.Err() != nil {
				break
			}

			sats := extractZapAmount(ie.Event)
			if sats <= 0 {
				continue
			}

			for _, tag := range ie.Tags {
				if len(tag) >= 2 && tag[0] == "e" {
					amounts[tag[1]] += sats
				}
			}
		}
	}

	// Write fetched amounts back to cache (including 0 for events with no zaps)
	now := timeNow()
	zapMu.Lock()
	for _, id := range uncached {
		cache.Zaps[id] = CachedZap{
			Amount:    amounts[id],
			FetchedAt: now,
		}
	}
	zapMu.Unlock()

	logv("zaps: got amounts for %d events", len(amounts))
	return amounts
}

// extractZapAmount parses the NIP-57 description tag from a kind:9735 zap receipt
// to extract the zap amount in sats. The description contains a serialized kind:9734
// zap request event with an "amount" tag in millisats.
func extractZapAmount(ev nostr.Event) int64 {
	var desc string
	for _, tag := range ev.Tags {
		if len(tag) >= 2 && tag[0] == "description" {
			desc = tag[1]
			break
		}
	}
	if desc == "" {
		return 0
	}

	var zapRequest struct {
		Tags [][]string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(desc), &zapRequest); err != nil {
		return 0
	}

	for _, tag := range zapRequest.Tags {
		if len(tag) >= 2 && tag[0] == "amount" {
			msats, err := strconv.ParseInt(tag[1], 10, 64)
			if err != nil {
				return 0
			}
			return msats / 1000
		}
	}

	return 0
}
