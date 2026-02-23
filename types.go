package main

import "fiatjaf.com/nostr"

// MatchQuality ranks how well a query matched a field.
// Lower value = better match.
type MatchQuality int

const (
	MatchExact    MatchQuality = 0
	MatchPrefix   MatchQuality = 1
	MatchContains MatchQuality = 2
	MatchNone     MatchQuality = -1
)

func (m MatchQuality) String() string {
	switch m {
	case MatchExact:
		return "exact"
	case MatchPrefix:
		return "prefix"
	case MatchContains:
		return "contains"
	default:
		return "unknown"
	}
}

// MatchedField identifies which metadata field matched.
// Lower priority = more relevant.
type MatchedField int

const (
	FieldName        MatchedField = 0
	FieldNip05       MatchedField = 1
	FieldDisplayName MatchedField = 2
	FieldAbout       MatchedField = 3
	FieldContent     MatchedField = 4
)

func (f MatchedField) String() string {
	switch f {
	case FieldName:
		return "name"
	case FieldNip05:
		return "nip05"
	case FieldDisplayName:
		return "display_name"
	case FieldAbout:
		return "about"
	case FieldContent:
		return "content"
	default:
		return "unknown"
	}
}

// SearchResult represents a single matched result.
type SearchResult struct {
	Pubkey       string `json:"pubkey"`
	Radius       int    `json:"radius"`
	MatchQuality string `json:"match_quality"`
	MatchedField string `json:"matched_field"`
	Kind         string `json:"kind"` // "profile" or "post"
	Content      string `json:"content,omitempty"`
	Name         string `json:"name,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	Nip05        string `json:"nip05,omitempty"`
	About        string `json:"about,omitempty"`
	EventID      string `json:"event_id,omitempty"`
	CreatedAt    int64  `json:"created_at,omitempty"`
	ZapAmount    int64  `json:"zap_amount"` // total sats zapped

	qualityRank MatchQuality
	fieldRank   MatchedField
}

// GraphNode tracks a pubkey discovered during BFS traversal.
type GraphNode struct {
	Pubkey nostr.PubKey
	Radius int
}
