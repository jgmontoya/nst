package main

import (
	"testing"

	"fiatjaf.com/nostr"
)

func TestMatchString(t *testing.T) {
	tests := []struct {
		value string
		query string
		want  MatchQuality
	}{
		{"alice", "alice", MatchExact},
		{"Alice", "alice", MatchNone}, // case-sensitive — caller lowercases
		{"alice", "ali", MatchPrefix},
		{"alice", "lic", MatchContains},
		{"alice", "bob", MatchNone},
		{"", "", MatchExact},
		{"hello world", "hello", MatchPrefix},
		{"hello world", "world", MatchContains},
		{"hello world", "hello world", MatchExact},
	}

	for _, tt := range tests {
		got := matchString(tt.value, tt.query)
		if got != tt.want {
			t.Errorf("matchString(%q, %q) = %v, want %v", tt.value, tt.query, got, tt.want)
		}
	}
}

func mustParse(t *testing.T, input string) Query {
	t.Helper()
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse(%q): %v", input, err)
	}
	return q
}

func TestMatchProfile(t *testing.T) {
	ev := nostr.Event{
		PubKey:  nostr.MustPubKeyFromHex("82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2"),
		Content: `{"name":"jack","display_name":"Jack Dorsey","about":"founder","nip05":"jack@cash.app"}`,
	}

	tests := []struct {
		query     string
		wantMatch bool
		wantField string
		wantQual  string
	}{
		{"jack", true, "name", "exact"},
		{"jac", true, "name", "prefix"},
		{"founder", true, "about", "exact"},
		{"cash.app", true, "nip05", "contains"},
		{"nonexistent", false, "", ""},
		{"jack dorsey", true, "display_name", "exact"},  // plain text → phrase search
		{`"jack dorsey"`, true, "display_name", "exact"}, // explicit quotes → same result
	}

	for _, tt := range tests {
		q := mustParse(t, tt.query)
		r, ok := matchProfile(ev, q, 1)
		if ok != tt.wantMatch {
			t.Errorf("matchProfile(query=%q) match=%v, want %v", tt.query, ok, tt.wantMatch)
			continue
		}
		if !ok {
			continue
		}
		if r.MatchedField != tt.wantField {
			t.Errorf("matchProfile(query=%q) field=%q, want %q", tt.query, r.MatchedField, tt.wantField)
		}
		if r.MatchQuality != tt.wantQual {
			t.Errorf("matchProfile(query=%q) quality=%q, want %q", tt.query, r.MatchQuality, tt.wantQual)
		}
	}
}

func TestMatchProfileBestField(t *testing.T) {
	ev := nostr.Event{
		PubKey:  nostr.MustPubKeyFromHex("82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2"),
		Content: `{"name":"alice","display_name":"alice","about":"alice is great"}`,
	}

	q := mustParse(t, "alice")
	r, ok := matchProfile(ev, q, 0)
	if !ok {
		t.Fatal("expected match")
	}
	if r.MatchedField != "name" {
		t.Errorf("expected best field 'name', got %q", r.MatchedField)
	}
	if r.MatchQuality != "exact" {
		t.Errorf("expected quality 'exact', got %q", r.MatchQuality)
	}
}

func TestMatchProfileInvalidJSON(t *testing.T) {
	ev := nostr.Event{
		PubKey:  nostr.MustPubKeyFromHex("82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2"),
		Content: "not json",
	}

	q := mustParse(t, "test")
	_, ok := matchProfile(ev, q, 0)
	if ok {
		t.Error("expected no match for invalid JSON")
	}
}

func TestMatchPost(t *testing.T) {
	ev := nostr.Event{
		PubKey:  nostr.MustPubKeyFromHex("82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2"),
		Content: "Hello world, this is a test post",
	}

	tests := []struct {
		query     string
		wantMatch bool
		wantQual  string
	}{
		{"hello world, this is a test post", true, "exact"},  // plain text → phrase
		{"hello", true, "prefix"},
		{"test post", true, "contains"},  // plain text → phrase
		{"nonexistent", false, ""},
	}

	for _, tt := range tests {
		q := mustParse(t, tt.query)
		r, ok := matchPost(ev, q, 2)
		if ok != tt.wantMatch {
			t.Errorf("matchPost(query=%q) match=%v, want %v", tt.query, ok, tt.wantMatch)
			continue
		}
		if ok && r.MatchQuality != tt.wantQual {
			t.Errorf("matchPost(query=%q) quality=%q, want %q", tt.query, r.MatchQuality, tt.wantQual)
		}
	}
}

func TestMatchCachedProfile(t *testing.T) {
	pk := nostr.MustPubKeyFromHex("82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2")
	cached := CachedProfile{
		Content:   `{"name":"jack","about":"no state is the best state"}`,
		EventID:   "abc123",
		CreatedAt: 1700000000,
	}

	q := mustParse(t, "jack")
	r, ok := matchCachedProfile(pk, cached, q, 1)
	if !ok {
		t.Fatal("expected match")
	}
	if r.EventID != "abc123" {
		t.Errorf("expected event_id 'abc123', got %q", r.EventID)
	}
	if r.CreatedAt != 1700000000 {
		t.Errorf("expected created_at 1700000000, got %d", r.CreatedAt)
	}
	if r.Radius != 1 {
		t.Errorf("expected radius 1, got %d", r.Radius)
	}
}

func TestSortResults(t *testing.T) {
	results := []SearchResult{
		{Radius: 2, qualityRank: MatchExact, fieldRank: FieldName},
		{Radius: 1, qualityRank: MatchContains, fieldRank: FieldAbout},
		{Radius: 1, qualityRank: MatchExact, fieldRank: FieldDisplayName},
		{Radius: 1, qualityRank: MatchExact, fieldRank: FieldName},
		{Radius: 0, qualityRank: MatchPrefix, fieldRank: FieldName},
	}

	sortResults(results)

	expected := []struct {
		radius  int
		quality MatchQuality
		field   MatchedField
	}{
		{0, MatchPrefix, FieldName},
		{1, MatchExact, FieldName},
		{1, MatchExact, FieldDisplayName},
		{1, MatchContains, FieldAbout},
		{2, MatchExact, FieldName},
	}

	for i, want := range expected {
		got := results[i]
		if got.Radius != want.radius || got.qualityRank != want.quality || got.fieldRank != want.field {
			t.Errorf("position %d: got (r=%d, q=%d, f=%d), want (r=%d, q=%d, f=%d)",
				i, got.Radius, got.qualityRank, got.fieldRank,
				want.radius, want.quality, want.field)
		}
	}
}

// --- Compound query tests for match functions ---

func TestMatchProfileAndQuery(t *testing.T) {
	ev := nostr.Event{
		PubKey:  nostr.MustPubKeyFromHex("82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2"),
		Content: `{"name":"jack","display_name":"Jack Dorsey","about":"bitcoin maximalist","nip05":"jack@cash.app"}`,
	}

	q := mustParse(t, "name:jack AND about:bitcoin")
	r, ok := matchProfile(ev, q, 1)
	if !ok {
		t.Fatal("expected match for name:jack AND about:bitcoin")
	}
	if r.MatchQuality != "prefix" {
		t.Errorf("expected quality 'prefix', got %q", r.MatchQuality)
	}
}

func TestMatchProfileOrQuery(t *testing.T) {
	ev := nostr.Event{
		PubKey:  nostr.MustPubKeyFromHex("82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2"),
		Content: `{"name":"alice","about":"developer"}`,
	}

	q := mustParse(t, "jack OR alice")
	r, ok := matchProfile(ev, q, 1)
	if !ok {
		t.Fatal("expected match for jack OR alice")
	}
	if r.MatchQuality != "exact" {
		t.Errorf("expected quality 'exact', got %q", r.MatchQuality)
	}
}

func TestMatchProfileNotQuery(t *testing.T) {
	ev := nostr.Event{
		PubKey:  nostr.MustPubKeyFromHex("82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2"),
		Content: `{"name":"alice","about":"developer"}`,
	}

	// Should match: alice is not a bot
	q := mustParse(t, "alice -bot")
	r, ok := matchProfile(ev, q, 1)
	if !ok {
		t.Fatal("expected match for alice -bot")
	}
	if r.MatchQuality != "contains" {
		t.Errorf("expected quality 'contains' (AND of exact + NOT), got %q", r.MatchQuality)
	}

	// Should not match: alice IS alice
	q2 := mustParse(t, "alice -alice")
	_, ok = matchProfile(ev, q2, 1)
	if ok {
		t.Error("expected no match for alice -alice")
	}
}
