package main

import (
	"encoding/json"
	"sort"
	"strings"

	"fiatjaf.com/nostr"
)

func matchProfile(ev nostr.Event, query Query, radius int) (SearchResult, bool) {
	var meta struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		About       string `json:"about"`
		Nip05       string `json:"nip05"`
	}
	if err := json.Unmarshal([]byte(ev.Content), &meta); err != nil {
		return SearchResult{}, false
	}

	fields := map[MatchedField]string{
		FieldName:        strings.ToLower(meta.Name),
		FieldNip05:       strings.ToLower(meta.Nip05),
		FieldDisplayName: strings.ToLower(meta.DisplayName),
		FieldAbout:       strings.ToLower(meta.About),
	}

	bestQuality, bestField := query.Evaluate(fields)
	if bestQuality == MatchNone {
		return SearchResult{}, false
	}

	return SearchResult{
		Pubkey:       ev.PubKey.Hex(),
		Radius:       radius,
		MatchQuality: bestQuality.String(),
		MatchedField: bestField.String(),
		Kind:         "profile",
		Name:         meta.Name,
		DisplayName:  meta.DisplayName,
		Nip05:        meta.Nip05,
		About:        meta.About,
		EventID:      ev.ID.Hex(),
		CreatedAt:    int64(ev.CreatedAt),
		qualityRank:  bestQuality,
		fieldRank:    bestField,
	}, true
}

func matchCachedProfile(pk nostr.PubKey, cached CachedProfile, query Query, radius int) (SearchResult, bool) {
	ev := nostr.Event{PubKey: pk, Content: cached.Content}
	r, ok := matchProfile(ev, query, radius)
	if ok {
		r.EventID = cached.EventID
		r.CreatedAt = cached.CreatedAt
	}
	return r, ok
}

func matchPost(ev nostr.Event, query Query, radius int) (SearchResult, bool) {
	fields := map[MatchedField]string{
		FieldContent: strings.ToLower(ev.Content),
	}

	q, _ := query.Evaluate(fields)
	if q == MatchNone {
		return SearchResult{}, false
	}

	return SearchResult{
		Pubkey:       ev.PubKey.Hex(),
		Radius:       radius,
		MatchQuality: q.String(),
		MatchedField: FieldContent.String(),
		Kind:         "post",
		Content:      ev.Content,
		EventID:      ev.ID.Hex(),
		CreatedAt:    int64(ev.CreatedAt),
		qualityRank:  q,
		fieldRank:    FieldContent,
	}, true
}

func matchCachedPost(pk nostr.PubKey, cached CachedPost, query Query, radius int) (SearchResult, bool) {
	ev := nostr.Event{PubKey: pk, Content: cached.Content}
	r, ok := matchPost(ev, query, radius)
	if ok {
		r.EventID = cached.EventID
		r.CreatedAt = cached.CreatedAt
	}
	return r, ok
}

// matchString checks if value matches query. Returns MatchQuality or MatchNone.
func matchString(value, query string) MatchQuality {
	if value == query {
		return MatchExact
	}
	if strings.HasPrefix(value, query) {
		return MatchPrefix
	}
	if strings.Contains(value, query) {
		return MatchContains
	}
	return MatchNone
}

func sortResults(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.Radius != b.Radius {
			return a.Radius < b.Radius
		}
		if a.qualityRank != b.qualityRank {
			return a.qualityRank < b.qualityRank
		}
		return a.fieldRank < b.fieldRank
	})
}
