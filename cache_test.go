package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	c := NewCache()
	if c.Follows == nil || c.Profiles == nil || c.Posts == nil || c.Zaps == nil {
		t.Fatal("NewCache should initialize maps")
	}
	if len(c.Follows) != 0 || len(c.Profiles) != 0 || len(c.Posts) != 0 || len(c.Zaps) != 0 {
		t.Fatal("NewCache should return empty maps")
	}
}

func TestCacheSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	c := NewCache()
	c.Follows["abc123"] = CachedFollows{
		Pubkeys:   []string{"def456", "ghi789"},
		FetchedAt: time.Now(),
	}
	c.Profiles["abc123"] = CachedProfile{
		Content:   `{"name":"alice"}`,
		EventID:   "evt001",
		CreatedAt: 1700000000,
		FetchedAt: time.Now(),
	}

	// Save to temp dir
	saveGob(filepath.Join(dir, "follows.gob"), c.Follows)
	saveGob(filepath.Join(dir, "profiles.gob"), c.Profiles)

	// Load back
	loaded := NewCache()
	loadGob(filepath.Join(dir, "follows.gob"), &loaded.Follows)
	loadGob(filepath.Join(dir, "profiles.gob"), &loaded.Profiles)

	if len(loaded.Follows) != 1 {
		t.Fatalf("expected 1 follow entry, got %d", len(loaded.Follows))
	}
	f := loaded.Follows["abc123"]
	if len(f.Pubkeys) != 2 || f.Pubkeys[0] != "def456" || f.Pubkeys[1] != "ghi789" {
		t.Errorf("follows data mismatch: %+v", f)
	}

	if len(loaded.Profiles) != 1 {
		t.Fatalf("expected 1 profile entry, got %d", len(loaded.Profiles))
	}
	p := loaded.Profiles["abc123"]
	if p.Content != `{"name":"alice"}` || p.EventID != "evt001" || p.CreatedAt != 1700000000 {
		t.Errorf("profile data mismatch: %+v", p)
	}
}

func TestCacheTTLPruning(t *testing.T) {
	dir := t.TempDir()

	fresh := time.Now()
	staleFollows := time.Now().Add(-8 * 24 * time.Hour) // 8 days ago, beyond 7-day TTL
	staleProfile := time.Now().Add(-25 * time.Hour)      // 25 hours ago, beyond 24-hour TTL

	follows := map[string]CachedFollows{
		"fresh":  {Pubkeys: []string{"a"}, FetchedAt: fresh},
		"stale":  {Pubkeys: []string{"b"}, FetchedAt: staleFollows},
	}
	profiles := map[string]CachedProfile{
		"fresh":  {Content: `{"name":"a"}`, FetchedAt: fresh},
		"stale":  {Content: `{"name":"b"}`, FetchedAt: staleProfile},
	}

	saveGob(filepath.Join(dir, "follows.gob"), follows)
	saveGob(filepath.Join(dir, "profiles.gob"), profiles)

	// Load and prune manually (simulating LoadCache logic)
	loaded := NewCache()
	loadGob(filepath.Join(dir, "follows.gob"), &loaded.Follows)
	loadGob(filepath.Join(dir, "profiles.gob"), &loaded.Profiles)

	now := time.Now()
	for k, v := range loaded.Follows {
		if now.Sub(v.FetchedAt) > followsTTL {
			delete(loaded.Follows, k)
		}
	}
	for k, v := range loaded.Profiles {
		if now.Sub(v.FetchedAt) > profilesTTL {
			delete(loaded.Profiles, k)
		}
	}

	if len(loaded.Follows) != 1 {
		t.Errorf("expected 1 fresh follow, got %d", len(loaded.Follows))
	}
	if _, ok := loaded.Follows["fresh"]; !ok {
		t.Error("fresh follow entry should survive pruning")
	}

	if len(loaded.Profiles) != 1 {
		t.Errorf("expected 1 fresh profile, got %d", len(loaded.Profiles))
	}
	if _, ok := loaded.Profiles["fresh"]; !ok {
		t.Error("fresh profile entry should survive pruning")
	}
}

func TestLoadGobMissingFile(t *testing.T) {
	var m map[string]CachedFollows
	loadGob("/nonexistent/path.gob", &m)
	// Should not panic, m stays nil
	if m != nil {
		t.Error("expected nil map for missing file")
	}
}

func TestSaveGobAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.gob")

	data := map[string]string{"key": "value"}
	saveGob(path, data)

	// The file should exist at the final path
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file at %s: %v", path, err)
	}
	// The tmp file should not exist
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("temp file should be cleaned up after save")
	}
}
