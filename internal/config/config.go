// Package config manages tyv's own configuration file: profile list, current
// profile, and per-profile metadata. It does NOT store authentication secrets.
//
// File layout (within AppDataDir):
//
//	tyv.json          — main config file (profiles + current)
//	profiles/<name>/ — per-profile directory; its home/ subdirectory is the
//	                   isolated HOME agy runs with (holds that profile's login)
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	configFileName = "tyv.json"
	profilesDir    = "profiles"
	configVersion  = 1
)

// Profile holds non-sensitive metadata for a single profile.
// Authentication credentials are NEVER stored here.
type Profile struct {
	// Name is the canonical identifier (validated, lowercase).
	Name string `json:"name"`
	// DisplayName is an optional human-friendly label.
	DisplayName string `json:"display_name,omitempty"`
	// Email is an optional, user-provided hint. Never fetched from OAuth.
	Email string `json:"email,omitempty"`
	// CreatedAt is when the profile was created.
	CreatedAt time.Time `json:"created_at"`
	// LastUsedAt is updated when the profile is activated.
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// Config is the root configuration structure persisted to tyv.json.
type Config struct {
	Version  int                `json:"version"`
	Current  string             `json:"current,omitempty"`
	Profiles map[string]Profile `json:"profiles"`
}

// Manager handles loading, saving, and mutating the tyv configuration.
type Manager struct {
	appDataDir string
	mu         sync.RWMutex
	cfg        *Config
}

// NewManager creates a Manager rooted at appDataDir.
// It does not load the config file immediately (lazy loading via Load).
func NewManager(appDataDir string) *Manager {
	return &Manager{appDataDir: appDataDir}
}

// ConfigPath returns the absolute path to the config file.
func (m *Manager) ConfigPath() string {
	return filepath.Join(m.appDataDir, configFileName)
}

func (m *Manager) configPath() string { return m.ConfigPath() }

// ProfileDataDir returns the directory that holds everything tyv keeps for
// the given profile, including the isolated home agy runs with.
func (m *Manager) ProfileDataDir(name string) string {
	return filepath.Join(m.appDataDir, profilesDir, name)
}

// ensureDir creates a directory with restrictive permissions (0700 on Unix).
// On Windows the permissions argument is ignored.
func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf("could not create directory %q: %w", path, err)
	}
	return nil
}

// Load reads the config from disk. If the file does not exist, an empty
// config is initialised (not an error). Returns an error for corrupt files.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := m.configPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		m.cfg = &Config{
			Version:  configVersion,
			Profiles: make(map[string]Profile),
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not read config file %q: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf(
			"configuration file appears to be corrupted: %w\n\n"+
				"File: %s\n\n"+
				"Recovery options:\n"+
				"  1. Delete the file and re-add your profiles: rm %q\n"+
				"  2. Run: tyv doctor",
			err, path, path,
		)
	}

	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]Profile)
	}

	m.cfg = &cfg
	return nil
}

// save writes the in-memory config to disk using an atomic write pattern:
//  1. Write to a temporary file alongside the config file.
//  2. Sync the file to disk.
//  3. Atomically rename into place.
//
// This prevents partial writes from corrupting the config.
// Caller must hold m.mu (write lock).
func (m *Manager) save() error {
	if err := ensureDir(m.appDataDir); err != nil {
		return err
	}

	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("could not serialise config: %w", err)
	}
	data = append(data, '\n')

	// Write to a temp file in the same directory to ensure rename is atomic.
	dir := filepath.Dir(m.configPath())
	tmp, err := os.CreateTemp(dir, ".tyv-config-*.tmp")
	if err != nil {
		return fmt.Errorf("could not create temp file for config: %w", err)
	}
	tmpName := tmp.Name()

	// On error, attempt cleanup.
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("could not write config to temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("could not sync config temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not close config temp file: %w", err)
	}

	// Set restrictive permissions on the file (0600).
	if err := os.Chmod(tmpName, 0600); err != nil {
		return fmt.Errorf("could not set config file permissions: %w", err)
	}

	// Atomic rename.
	if err := os.Rename(tmpName, m.configPath()); err != nil {
		return fmt.Errorf("could not atomically save config file: %w", err)
	}
	success = true
	return nil
}

// Get returns a read-only copy of the current configuration.
// Load must be called before Get.
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cfg == nil {
		return Config{Profiles: make(map[string]Profile)}
	}
	return *m.cfg
}

// ProfileExists reports whether the named profile exists.
func (m *Manager) ProfileExists(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cfg == nil {
		return false
	}
	_, ok := m.cfg.Profiles[name]
	return ok
}

// AddProfile creates a new profile. Returns an error if the profile already
// exists.
func (m *Manager) AddProfile(name, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg == nil {
		return fmt.Errorf("config not loaded; call Load() first")
	}
	if _, exists := m.cfg.Profiles[name]; exists {
		return fmt.Errorf(
			"profile %q already exists\n\n"+
				"Use a different name or remove it first:\n"+
				"  tyv remove %s",
			name, name,
		)
	}

	// Create the profile data directory before persisting config.
	profileDir := filepath.Join(m.appDataDir, profilesDir, name)
	if err := ensureDir(profileDir); err != nil {
		return fmt.Errorf("could not create profile directory: %w", err)
	}

	m.cfg.Profiles[name] = Profile{
		Name:      name,
		Email:     email,
		CreatedAt: time.Now().UTC(),
	}

	// Set current if this is the first profile.
	if m.cfg.Current == "" {
		m.cfg.Current = name
	}

	return m.save()
}

// RemoveProfile deletes a profile and its data directory. That directory
// holds the profile's own agy login (its private ~/.gemini), which is deleted
// with it; the symlinks into the real home are removed, never followed, so
// files in the real home are untouched.
func (m *Manager) RemoveProfile(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg == nil {
		return fmt.Errorf("config not loaded; call Load() first")
	}
	if _, exists := m.cfg.Profiles[name]; !exists {
		return profileNotFoundError(name, m.cfg)
	}

	delete(m.cfg.Profiles, name)

	// If we just deleted the current profile, clear it.
	if m.cfg.Current == name {
		m.cfg.Current = ""
	}

	// Save config before removing the directory. If directory removal fails,
	// the worst case is an orphaned directory — not a config inconsistency.
	if err := m.save(); err != nil {
		return err
	}

	// Remove the profile's data directory.
	profileDir := filepath.Join(m.appDataDir, profilesDir, name)
	if err := os.RemoveAll(profileDir); err != nil {
		// Non-fatal: config is already updated.
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not remove profile directory %q: %v\n", profileDir, err)
	}

	return nil
}

// RenameProfile renames oldName to newName.
func (m *Manager) RenameProfile(oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg == nil {
		return fmt.Errorf("config not loaded; call Load() first")
	}
	profile, exists := m.cfg.Profiles[oldName]
	if !exists {
		return profileNotFoundError(oldName, m.cfg)
	}
	if _, exists := m.cfg.Profiles[newName]; exists {
		return fmt.Errorf("profile %q already exists; choose a different name", newName)
	}

	// Rename the data directory first, before updating config.
	oldDir := filepath.Join(m.appDataDir, profilesDir, oldName)
	newDir := filepath.Join(m.appDataDir, profilesDir, newName)

	// Ensure the new directory doesn't already exist.
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("profile data directory %q already exists; cannot rename", newDir)
	}

	// Create parent if needed.
	if err := ensureDir(filepath.Dir(newDir)); err != nil {
		return err
	}

	if _, err := os.Stat(oldDir); err == nil {
		// oldDir exists: rename it.
		if err := os.Rename(oldDir, newDir); err != nil {
			return fmt.Errorf("could not rename profile directory: %w", err)
		}
	} else {
		// oldDir doesn't exist yet (e.g. profile was added but never launched).
		// Create the new directory.
		if err := ensureDir(newDir); err != nil {
			return err
		}
	}

	// Update in-memory profile.
	profile.Name = newName
	delete(m.cfg.Profiles, oldName)
	m.cfg.Profiles[newName] = profile

	if m.cfg.Current == oldName {
		m.cfg.Current = newName
	}

	return m.save()
}

// SetCurrent updates the current profile without launching agy.
func (m *Manager) SetCurrent(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg == nil {
		return fmt.Errorf("config not loaded; call Load() first")
	}
	if _, exists := m.cfg.Profiles[name]; !exists {
		return profileNotFoundError(name, m.cfg)
	}

	m.cfg.Current = name
	return m.save()
}

// SetEmail records the account email shown for a profile. It is a no-op when
// the profile does not exist or the value is unchanged.
func (m *Manager) SetEmail(name, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg == nil {
		return nil
	}
	p, ok := m.cfg.Profiles[name]
	if !ok || p.Email == email {
		return nil
	}
	p.Email = email
	m.cfg.Profiles[name] = p
	return m.save()
}

// TouchLastUsed updates the LastUsedAt timestamp for the given profile.
func (m *Manager) TouchLastUsed(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg == nil {
		return nil
	}
	p, ok := m.cfg.Profiles[name]
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	p.LastUsedAt = &now
	m.cfg.Profiles[name] = p
	m.cfg.Current = name
	return m.save()
}

// profileNotFoundError returns a helpful error for missing profiles.
// Caller must hold the lock.
func profileNotFoundError(name string, cfg *Config) error {
	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}

	if len(names) == 0 {
		return fmt.Errorf(
			"profile %q does not exist\n\n"+
				"No profiles have been created yet.\n\n"+
				"Create one with:\n"+
				"  tyv add %s",
			name, name,
		)
	}

	list := ""
	for _, n := range names {
		list += "  " + n + "\n"
	}
	return fmt.Errorf(
		"profile %q does not exist\n\n"+
			"Available profiles:\n%s\n"+
			"Create it with:\n"+
			"  tyv add %s",
		name, list, name,
	)
}

// IsCorrupted attempts to detect obvious config file corruption without
// loading the full config. Returns true and a description if corruption is
// detected.
func IsCorrupted(path string) (bool, string) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, ""
	}
	if err != nil {
		return true, fmt.Sprintf("cannot open: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, 1<<20)) // 1 MiB limit
	if err != nil {
		return true, fmt.Sprintf("cannot read: %v", err)
	}

	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return true, fmt.Sprintf("invalid JSON: %v", err)
	}
	return false, ""
}
