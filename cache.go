package main

import (
	"encoding/gob"
	"os"
	"path/filepath"
	"time"
)

const (
	followsTTL  = 7 * 24 * time.Hour
	profilesTTL = 24 * time.Hour
	postsTTL    = 30 * 24 * time.Hour
	zapsTTL     = 2 * time.Hour
)

// CachedFollows stores a pubkey's follow list with a fetch timestamp.
type CachedFollows struct {
	Pubkeys   []string // hex pubkeys
	FetchedAt time.Time
}

// CachedProfile stores a pubkey's kind:0 metadata content with a fetch timestamp.
type CachedProfile struct {
	Content   string // raw JSON from kind:0 event
	EventID   string // hex event ID
	CreatedAt int64  // unix timestamp
	FetchedAt time.Time
}

// CachedPosts stores a pubkey's recent posts with a fetch timestamp.
type CachedPosts struct {
	Posts     []CachedPost
	FetchedAt time.Time
}

// CachedPost stores a single post event.
type CachedPost struct {
	Content   string
	EventID   string
	CreatedAt int64
}

// CachedZap stores the total sats zapped for an event with a fetch timestamp.
type CachedZap struct {
	Amount    int64 // total sats
	FetchedAt time.Time
}

// Cache holds follow lists, profile metadata, posts, and zap amounts across runs.
type Cache struct {
	Follows  map[string]CachedFollows // hex pubkey -> follows
	Profiles map[string]CachedProfile // hex pubkey -> profile
	Posts    map[string]CachedPosts   // hex pubkey -> posts
	Zaps     map[string]CachedZap    // hex event ID -> zap amount
}

func NewCache() *Cache {
	return &Cache{
		Follows:  make(map[string]CachedFollows),
		Profiles: make(map[string]CachedProfile),
		Posts:    make(map[string]CachedPosts),
		Zaps:     make(map[string]CachedZap),
	}
}

func cacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "nst"), nil
}

// LoadCache loads the cache from disk. Returns an empty cache if files don't exist.
func LoadCache() *Cache {
	c := NewCache()

	dir, err := cacheDir()
	if err != nil {
		return c
	}

	loadGob(filepath.Join(dir, "follows.gob"), &c.Follows)
	loadGob(filepath.Join(dir, "profiles.gob"), &c.Profiles)
	loadGob(filepath.Join(dir, "posts.gob"), &c.Posts)
	loadGob(filepath.Join(dir, "zaps.gob"), &c.Zaps)

	// Prune expired entries on load
	now := time.Now()
	for k, v := range c.Follows {
		if now.Sub(v.FetchedAt) > followsTTL {
			delete(c.Follows, k)
		}
	}
	for k, v := range c.Profiles {
		if now.Sub(v.FetchedAt) > profilesTTL {
			delete(c.Profiles, k)
		}
	}
	for k, v := range c.Posts {
		if now.Sub(v.FetchedAt) > postsTTL {
			delete(c.Posts, k)
		}
	}
	for k, v := range c.Zaps {
		if now.Sub(v.FetchedAt) > zapsTTL {
			delete(c.Zaps, k)
		}
	}

	logv("cache: %d follow lists, %d profiles, %d post authors, %d zap entries loaded", len(c.Follows), len(c.Profiles), len(c.Posts), len(c.Zaps))
	return c
}

// Save persists the cache to disk.
func (c *Cache) Save() {
	dir, err := cacheDir()
	if err != nil {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	saveGob(filepath.Join(dir, "follows.gob"), c.Follows)
	saveGob(filepath.Join(dir, "profiles.gob"), c.Profiles)
	saveGob(filepath.Join(dir, "posts.gob"), c.Posts)
	saveGob(filepath.Join(dir, "zaps.gob"), c.Zaps)
}

// ClearCache removes all cache files from disk.
func ClearCache() error {
	dir, err := cacheDir()
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func loadGob(path string, target any) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	gob.NewDecoder(f).Decode(target)
}

// timeNow returns the current time. Exists as a variable for testability.
var timeNow = time.Now

func saveGob(path string, data any) {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}

	if err := gob.NewEncoder(f).Encode(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return
	}

	f.Close()
	os.Rename(tmp, path)
}
