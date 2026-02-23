package main

import (
	"testing"

	"fiatjaf.com/nostr"
)

func TestExtractZapAmount(t *testing.T) {
	tests := []struct {
		name     string
		tags     [][]string
		wantSats int64
	}{
		{
			name: "valid zap receipt with 21000 msats",
			tags: [][]string{
				{"description", `{"kind":9734,"tags":[["amount","21000"],["p","abc123"]]}`},
				{"e", "event123"},
			},
			wantSats: 21,
		},
		{
			name: "valid zap receipt with 1000000 msats (1000 sats)",
			tags: [][]string{
				{"description", `{"kind":9734,"tags":[["amount","1000000"],["p","abc123"]]}`},
			},
			wantSats: 1000,
		},
		{
			name: "no description tag",
			tags: [][]string{
				{"e", "event123"},
				{"bolt11", "lnbc..."},
			},
			wantSats: 0,
		},
		{
			name: "invalid description JSON",
			tags: [][]string{
				{"description", "not json"},
			},
			wantSats: 0,
		},
		{
			name: "description without amount tag",
			tags: [][]string{
				{"description", `{"kind":9734,"tags":[["p","abc123"]]}`},
			},
			wantSats: 0,
		},
		{
			name:     "empty tags",
			tags:     nil,
			wantSats: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := nostr.Event{Tags: make([]nostr.Tag, len(tt.tags))}
			for i, tag := range tt.tags {
				ev.Tags[i] = nostr.Tag(tag)
			}
			got := extractZapAmount(ev)
			if got != tt.wantSats {
				t.Errorf("extractZapAmount() = %d sats, want %d", got, tt.wantSats)
			}
		})
	}
}
