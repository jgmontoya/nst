package main

import (
	"testing"
	"time"
)

func TestResolvePubkeyHex(t *testing.T) {
	hex := "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"
	pk, err := resolvePubkey(hex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pk.Hex() != hex {
		t.Errorf("got %s, want %s", pk.Hex(), hex)
	}
}

func TestResolvePubkeyNpub(t *testing.T) {
	npub := "npub180cvv07tjdrrgpa0j7j7tmnyl2yr6yr7l8j4s3evf6u64th6gkwsyjh6w6"
	expectedHex := "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"

	pk, err := resolvePubkey(npub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pk.Hex() != expectedHex {
		t.Errorf("got %s, want %s", pk.Hex(), expectedHex)
	}
}

func TestResolvePubkeyInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"garbage", "not-a-valid-key"},
		{"too short hex", "abcd1234"},
		{"nsec rejected", "nsec1vl029mgpspedva04g90vltkh6fvh240zqtv9k0t9af8935ke9laqsnlfe5"},
	}

	for _, tt := range tests {
		_, err := resolvePubkey(tt.input)
		if err == nil {
			t.Errorf("resolvePubkey(%q) should return error for %s", tt.input, tt.name)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"24h", 24 * time.Hour},
		{"1h", time.Hour},
		{"30m", 30 * time.Minute},
		{"7d", 7 * 24 * time.Hour},
		{"1d", 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
	}

	for _, tt := range tests {
		got, err := parseDuration(tt.input)
		if err != nil {
			t.Errorf("parseDuration(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseDurationInvalid(t *testing.T) {
	invalid := []string{"", "abc", "7x", "d"}
	for _, s := range invalid {
		_, err := parseDuration(s)
		if err == nil {
			t.Errorf("parseDuration(%q) should return error", s)
		}
	}
}
