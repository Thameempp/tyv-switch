package antigravity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const cacheFile = "usage-cache.json"

// Entry is one profile's cached quota. It contains percentages only.
type Entry struct {
	Usage Usage     `json:"usage"`
	At    time.Time `json:"at"`
}

// Cache keeps the last successful quota reading per profile, used as a
// fallback when a fresh lookup fails.
type Cache struct {
	path    string
	Entries map[string]Entry `json:"entries"`
}

// LoadCache reads the cache under dir; a missing or corrupt file is empty.
func LoadCache(dir string) *Cache {
	c := &Cache{path: filepath.Join(dir, cacheFile), Entries: map[string]Entry{}}
	if data, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(data, c)
		if c.Entries == nil {
			c.Entries = map[string]Entry{}
		}
	}
	return c
}

// Set records a reading for profile.
func (c *Cache) Set(profile string, u Usage) {
	c.Entries[profile] = Entry{Usage: u, At: time.Now()}
}

// Delete forgets the reading for profile.
func (c *Cache) Delete(profile string) { delete(c.Entries, profile) }

// Save writes the cache atomically with owner-only permissions (best effort).
func (c *Cache) Save() {
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.path), ".tyv-usage-*.tmp")
	if err != nil {
		return
	}
	_ = tmp.Chmod(0600)
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), c.path) != nil {
		_ = os.Remove(tmp.Name())
	}
}
