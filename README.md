# NST - Nostr Search Tool

A CLI tool for searching posts and profiles on Nostr using your social graph as the search space. Instead of querying a global index, NST starts from a seed pubkey and expands outward through the follow graph, so results are ranked by social proximity.

## How it works

1. **Graph expansion** — BFS-traverses the follow graph from your seed pubkey up to `--radius` hops (default: 2)
2. **Event fetching** — Fetches kind:0 (profiles) or kind:1 (posts) for all discovered pubkeys
3. **Matching** — Scores each result by match quality (exact > prefix > contains) and field priority
4. **Zap filtering** — When `--min-zaps` is set, fetches kind:9735 zap receipts and filters by total sats
5. **Ranking** — Sorts by social distance first, then match quality
6. **Output** — Structured JSON to stdout, status to stderr (details with `-v`)

## Build

```sh
go build -o nst .
```

## Usage

```sh
nst --seed <npub|hex> --post <query>      # search posts
nst --seed <npub|hex> --profile <query>   # search profiles
```

## Query Language

Queries support boolean operators, field targeting, quoted phrases, and grouping.

### Simple queries

Plain text without operators is treated as a phrase search:

```sh
nst --seed <npub> --post "hello world"       # phrase: matches "hello world" literally
nst --seed <npub> --profile jack             # single term: matches "jack" in any field
```

### Boolean operators

Keywords `AND`, `OR`, `NOT` must be **uppercase** — lowercase versions are treated as search terms.

```sh
nst --seed <npub> --post "bitcoin AND lightning"  # both must match
nst --seed <npub> --post "bitcoin OR fiat"        # either matches
nst --seed <npub> --post "bitcoin NOT shitcoin"   # bitcoin without shitcoin
nst --seed <npub> --post "bitcoin -bot"           # - is shorthand for NOT
```

Implicit AND: `bitcoin lightning` (two terms without operator) is `bitcoin AND lightning`.

### Field targeting

Target specific profile metadata fields with `field:value`:

```sh
nst --seed <npub> --profile "name:jack"
nst --seed <npub> --profile "name:jack AND about:bitcoin"
nst --seed <npub> --profile "name:jack OR nip05:jack"
```

Available fields: `name`, `display_name`, `nip05`, `about`, `content`.

### Grouping

Parentheses control precedence:

```sh
nst --seed <npub> --profile "(jack OR dorsey) -bot"
nst --seed <npub> --post "(bitcoin OR lightning) AND NOT scam"
```

### Quoted phrases

Double quotes match an exact phrase:

```sh
nst --seed <npub> --post '"proof of work"'
nst --seed <npub> --profile 'name:"jack dorsey"'
```

## Examples

Search posts from fiatjaf's social graph:

```sh
nst --seed npub180cvv07tjdrrgpa0j7j7tmnyl2yr6yr7l8j4s3evf6u64th6gkwsyjh6w6 --post "bitcoin"
```

Search profiles within 1 hop:

```sh
nst --seed npub180cvv07tjdrrgpa0j7j7tmnyl2yr6yr7l8j4s3evf6u64th6gkwsyjh6w6 --profile "jack" --radius 1
```

Only posts from the last 7 days:

```sh
nst --seed <npub> --post "nostr" --since 7d
```

Only results with at least 1000 sats zapped:

```sh
nst --seed <npub> --post "nostr" --min-zaps 1000
```

Use custom relays and limit results:

```sh
nst --seed <npub> --post "nostr" --relay wss://relay.damus.io --relay wss://nos.lol --limit 10
```

Pipe into jq:

```sh
nst --seed <npub> --profile "alice" | jq '.[].name'
```

## Flags

| Flag             | Default    | Description                                                |
| ---------------- | ---------- | ---------------------------------------------------------- |
| `--seed`         | required   | Seed pubkey (npub or hex) to start the trust graph from    |
| `--post`         |            | Search query for posts (kind:1 notes)                      |
| `--profile`      |            | Search query for profiles (kind:0 metadata)                |
| `--radius`       | `2`        | Max search radius in the social graph                      |
| `--limit`        | `50`       | Max number of results to return                            |
| `--min-zaps`     | `0`        | Minimum total sats zapped for a result to be included      |
| `--since`        |            | Only include posts within this duration (e.g. `24h`, `7d`) |
| `--relay`        | [defaults] | Relay URLs to use (repeatable)                             |
| `--timeout`      | `10`       | Timeout in seconds for relay operations                    |
| `--no-cache`     | `false`    | Ignore cached data and fetch fresh (still saves results)   |
| `--clear-cache`  |            | Clear all cached data and exit                             |
| `--verbose`/`-v` | `false`    | Enable verbose logging to stderr                           |

Default relays: `relay.damus.io`, `relay.nostr.band`, `nos.lol`, `relay.snort.social`, `nostr.wine`.

## Output

Results are a JSON array sorted by radius (closest first), then match quality:

```json
[
  {
    "pubkey": "82341f882b6eabcd...",
    "radius": 1,
    "match_quality": "exact",
    "matched_field": "name",
    "kind": "profile",
    "name": "jack",
    "about": "no state is the best state",
    "event_id": "5b9f240083555491...",
    "created_at": 1748690293,
    "zap_amount": 21000
  }
]
```

Status output goes to stderr. By default only `found N results` is printed. Use `-v` for detailed progress logs.

## Caching

NST caches data locally to speed up repeated searches:

| Data         | TTL      | File                        |
| ------------ | -------- | --------------------------- |
| Follow lists | 7 days   | `~/.cache/nst/follows.gob`  |
| Profiles     | 24 hours | `~/.cache/nst/profiles.gob` |
| Posts        | 30 days  | `~/.cache/nst/posts.gob`    |
| Zap amounts  | 2 hours  | `~/.cache/nst/zaps.gob`     |

Expired entries are pruned on load. Use `--no-cache` to force fresh fetches (results are still saved), or `--clear-cache` to wipe all cached data.

## Built with

- [fiatjaf.com/nostr](https://github.com/fiatjaf/go-nostr) — Go Nostr protocol library
- Inspired by [nak](https://github.com/fiatjaf/nak) and [whitenoise](https://github.com/marmot-protocol/whitenoise-rs)
