package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"github.com/urfave/cli/v3"
)

// DefaultRelays are well-known public relays used when no relay list is available.
var DefaultRelays = []string{
	"wss://relay.damus.io",
	"wss://relay.nostr.band",
	"wss://nos.lol",
	"wss://relay.snort.social",
	"wss://nostr.wine",
}

func main() {
	app := &cli.Command{
		Name:  "nst",
		Usage: "Nostr Search Tool — search posts and profiles via web of trust",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "seed",
				Usage: "seed pubkey (npub or hex) to start the trust graph from",
			},
			&cli.StringFlag{
				Name:  "post",
				Usage: "search query for posts (kind:1 notes)",
			},
			&cli.StringFlag{
				Name:  "profile",
				Usage: "search query for profiles (kind:0 metadata)",
			},
			&cli.IntFlag{
				Name:  "radius",
				Usage: "max search radius in the social graph",
				Value: 2,
			},
			&cli.IntFlag{
				Name:  "limit",
				Usage: "max number of results to return",
				Value: 50,
			},
			&cli.StringSliceFlag{
				Name:  "relay",
				Usage: "relay URLs to use (can be specified multiple times)",
			},
			&cli.IntFlag{
				Name:  "min-zaps",
				Usage: "minimum total sats zapped for a result to be included",
				Value: 0,
			},
			&cli.IntFlag{
				Name:  "timeout",
				Usage: "timeout in seconds for relay operations",
				Value: 10,
			},
			&cli.StringFlag{
				Name:  "since",
				Usage: "only include posts created within this duration (e.g. 24h, 7d, 30d)",
			},
			&cli.BoolFlag{
				Name:  "no-cache",
				Usage: "ignore cached data and fetch fresh from relays (still saves results)",
			},
			&cli.BoolFlag{
				Name:  "clear-cache",
				Usage: "clear all cached data and exit",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "enable verbose logging to stderr",
			},
		},
		Action: run,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	verbose = cmd.Bool("verbose")

	if cmd.Bool("clear-cache") {
		if err := ClearCache(); err != nil {
			return fmt.Errorf("failed to clear cache: %w", err)
		}
		fmt.Fprintln(os.Stderr, "cache cleared")
		return nil
	}

	if cmd.String("seed") == "" {
		return fmt.Errorf("--seed is required")
	}

	seedPubkey, err := resolvePubkey(cmd.String("seed"))
	if err != nil {
		return fmt.Errorf("invalid seed pubkey: %w", err)
	}

	postQuery := cmd.String("post")
	profileQuery := cmd.String("profile")
	if postQuery == "" && profileQuery == "" {
		return fmt.Errorf("specify --post or --profile search query")
	}
	if postQuery != "" && profileQuery != "" {
		return fmt.Errorf("specify either --post or --profile, not both")
	}

	maxRadius := int(cmd.Int("radius"))
	limit := int(cmd.Int("limit"))
	minZaps := int64(cmd.Int("min-zaps"))
	timeout := time.Duration(cmd.Int("timeout")) * time.Second

	var since int64
	if s := cmd.String("since"); s != "" {
		dur, err := parseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid --since: %w", err)
		}
		since = time.Now().Add(-dur).Unix()
	}

	relays := cmd.StringSlice("relay")
	if len(relays) == 0 {
		relays = DefaultRelays
	}

	var cache *Cache
	if cmd.Bool("no-cache") {
		cache = NewCache()
	} else {
		cache = LoadCache()
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pool := nostr.NewPool(nostr.PoolOptions{PenaltyBox: true})

	query := postQuery
	kind := "post"
	if profileQuery != "" {
		query = profileQuery
		kind = "profile"
	}

	if since > 0 {
		logv("searching %ss for %q from %s... (max radius: %d, since: %s)",
			kind, query, seedPubkey.Hex()[:12], maxRadius, time.Unix(since, 0).Format(time.RFC3339))
	} else {
		logv("searching %ss for %q from %s... (max radius: %d)",
			kind, query, seedPubkey.Hex()[:12], maxRadius)
	}

	nodesCh := ExpandGraph(ctx, pool, cache, seedPubkey, maxRadius, relays)
	candidatesCh, searchCleanup := Search(ctx, pool, cache, nodesCh, query, kind, since, relays)
	results := CollectResults(pool, cache, candidatesCh, cancel, minZaps, limit, timeout, relays)

	if len(results) > limit {
		results = results[:limit]
	}

	fmt.Fprintf(os.Stderr, "found %d results\n", len(results))

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	err = enc.Encode(results)

	// Cleanup and cache save happen in background — process exits when main returns,
	// but we give goroutines a moment to finish so cache writes aren't lost
	done := make(chan struct{})
	go func() {
		searchCleanup()
		cache.Save()
		close(done)
	}()

	// Wait briefly for cache save, but don't block exit on slow relay teardown
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		logv("cleanup: timed out waiting for relay teardown, exiting")
	}

	return err
}

// resolvePubkey converts an npub or hex string to a PubKey.
func resolvePubkey(input string) (nostr.PubKey, error) {
	if len(input) == 64 {
		return nostr.PubKeyFromHex(input)
	}

	prefix, value, err := nip19.Decode(input)
	if err != nil {
		return nostr.PubKey{}, fmt.Errorf("failed to decode %q: %w", input, err)
	}

	if prefix != "npub" {
		return nostr.PubKey{}, fmt.Errorf("expected npub, got %s", prefix)
	}

	pk, ok := value.(nostr.PubKey)
	if !ok {
		return nostr.PubKey{}, fmt.Errorf("unexpected type from npub decode")
	}
	return pk, nil
}

// parseDuration extends time.ParseDuration with support for "d" (days).
func parseDuration(s string) (time.Duration, error) {
	if len(s) > 1 && s[len(s)-1] == 'd' {
		s = s[:len(s)-1] + "h"
		dur, err := time.ParseDuration(s)
		if err != nil {
			return 0, err
		}
		return dur * 24, nil
	}
	return time.ParseDuration(s)
}
