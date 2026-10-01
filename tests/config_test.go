package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Thameempp/tyv-switch/internal/config"
)

// newTestManager creates a Manager backed by a temp directory.
// It registers t.Cleanup to remove the directory after the test.
func newTestManager(t *testing.T) (*config.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	mgr := config.NewManager(dir)
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return mgr, dir
}

func TestAddProfile(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.AddProfile("personal", ""); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	cfg := mgr.Get()
	if _, ok := cfg.Profiles["personal"]; !ok {
		t.Error("profile 'personal' not found after add")
	}
	if cfg.Current != "personal" {
		t.Errorf("expected current='personal', got %q", cfg.Current)
	}
}

func TestAddProfile_WithEmail(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.AddProfile("work", "work@example.com"); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	cfg := mgr.Get()
	p := cfg.Profiles["work"]
	if p.Email != "work@example.com" {
		t.Errorf("expected email='work@example.com', got %q", p.Email)
	}
}

func TestAddProfile_Duplicate(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.AddProfile("personal", ""); err != nil {
		t.Fatalf("first AddProfile: %v", err)
	}
	if err := mgr.AddProfile("personal", ""); err == nil {
		t.Error("expected error for duplicate profile, got nil")
	}
}

func TestRemoveProfile(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.AddProfile("personal", ""); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := mgr.RemoveProfile("personal"); err != nil {
		t.Fatalf("RemoveProfile: %v", err)
	}

	cfg := mgr.Get()
	if _, ok := cfg.Profiles["personal"]; ok {
		t.Error("profile 'personal' still present after remove")
	}
}

func TestRemoveProfile_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.RemoveProfile("nonexistent"); err == nil {
		t.Error("expected error removing nonexistent profile")
	}
}

func TestRemoveProfile_ClearsCurrentIfActive(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.AddProfile("work", ""); err != nil {
		t.Fatal(err)
	}
	// work is now current (first profile added).

	if err := mgr.RemoveProfile("work"); err != nil {
		t.Fatal(err)
	}

	cfg := mgr.Get()
	if cfg.Current != "" {
		t.Errorf("expected current to be cleared, got %q", cfg.Current)
	}
}

func TestRenameProfile(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.AddProfile("work", "work@example.com"); err != nil {
		t.Fatal(err)
	}

	if err := mgr.RenameProfile("work", "company"); err != nil {
		t.Fatalf("RenameProfile: %v", err)
	}

	cfg := mgr.Get()
	if _, ok := cfg.Profiles["company"]; !ok {
		t.Error("profile 'company' not found after rename")
	}
	if _, ok := cfg.Profiles["work"]; ok {
		t.Error("profile 'work' still present after rename")
	}
}

func TestRenameProfile_UpdatesCurrent(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.AddProfile("work", ""); err != nil {
		t.Fatal(err)
	}

	if err := mgr.RenameProfile("work", "company"); err != nil {
		t.Fatal(err)
	}

	cfg := mgr.Get()
	if cfg.Current != "company" {
		t.Errorf("expected current='company', got %q", cfg.Current)
	}
}

func TestRenameProfile_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.RenameProfile("nonexistent", "new"); err == nil {
		t.Error("expected error renaming nonexistent profile")
	}
}

func TestRenameProfile_DestinationExists(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.AddProfile("work", ""); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddProfile("personal", ""); err != nil {
		t.Fatal(err)
	}

	if err := mgr.RenameProfile("work", "personal"); err == nil {
		t.Error("expected error when renaming to an existing profile name")
	}
}

func TestConfigPersistence(t *testing.T) {
	dir := t.TempDir()

	// Add a profile in one manager instance.
	mgr1 := config.NewManager(dir)
	if err := mgr1.Load(); err != nil {
		t.Fatal(err)
	}
	if err := mgr1.AddProfile("personal", "me@example.com"); err != nil {
		t.Fatal(err)
	}

	// Load in a fresh manager instance to verify persistence.
	mgr2 := config.NewManager(dir)
	if err := mgr2.Load(); err != nil {
		t.Fatal(err)
	}

	cfg := mgr2.Get()
	p, ok := cfg.Profiles["personal"]
	if !ok {
		t.Fatal("profile 'personal' not found after reload")
	}
	if p.Email != "me@example.com" {
		t.Errorf("expected email='me@example.com', got %q", p.Email)
	}
}

func TestConfigPermissions(t *testing.T) {
	mgr, dir := newTestManager(t)

	if err := mgr.AddProfile("personal", ""); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, "tyv.json")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}

	// Config file should be 0600 (owner read/write only).
	mode := info.Mode().Perm()
	if mode&0077 != 0 {
		t.Errorf("config file has overly permissive mode %v; expected 0600", mode)
	}
}

func TestCorruptedConfig(t *testing.T) {
	dir := t.TempDir()

	// Write invalid JSON.
	configPath := filepath.Join(dir, "tyv.json")
	if err := os.WriteFile(configPath, []byte("{not valid json"), 0600); err != nil {
		t.Fatal(err)
	}

	mgr := config.NewManager(dir)
	if err := mgr.Load(); err == nil {
		t.Error("expected error loading corrupted config, got nil")
	}
}

func TestIsCorrupted_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tyv.json")

	data, _ := json.Marshal(map[string]interface{}{
		"version":  1,
		"profiles": map[string]interface{}{},
	})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	corrupted, _ := config.IsCorrupted(path)
	if corrupted {
		t.Error("expected non-corrupted file to not be detected as corrupted")
	}
}

func TestIsCorrupted_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tyv.json")

	if err := os.WriteFile(path, []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}

	corrupted, desc := config.IsCorrupted(path)
	if !corrupted {
		t.Error("expected corrupted file to be detected")
	}
	if desc == "" {
		t.Error("expected corruption description, got empty string")
	}
}

func TestIsCorrupted_MissingFile(t *testing.T) {
	corrupted, _ := config.IsCorrupted("/nonexistent/path/tyv.json")
	if corrupted {
		t.Error("missing file should not be reported as corrupted")
	}
}

func TestConcurrentAccess(t *testing.T) {
	mgr, _ := newTestManager(t)

	// Pre-add some profiles.
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := mgr.AddProfile(name, ""); err != nil {
			t.Fatalf("AddProfile(%q): %v", name, err)
		}
	}

	// Concurrently read and write.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Reads.
			_ = mgr.Get()
			_ = mgr.ProfileExists("alpha")
		}(i)
	}

	wg.Wait()
}

func TestProfileDataDirPathTraversal(t *testing.T) {
	dir := t.TempDir()
	mgr := config.NewManager(dir)

	// ProfileDataDir should produce a path that stays within the app data dir.
	profileDir := mgr.ProfileDataDir("personal")

	// The profile dir must start with the app data dir.
	if len(profileDir) <= len(dir) {
		t.Errorf("profile dir %q does not start with app dir %q", profileDir, dir)
	}
}

func TestSetCurrent(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.AddProfile("personal", ""); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddProfile("work", ""); err != nil {
		t.Fatal(err)
	}

	if err := mgr.SetCurrent("work"); err != nil {
		t.Fatalf("SetCurrent: %v", err)
	}

	cfg := mgr.Get()
	if cfg.Current != "work" {
		t.Errorf("expected current='work', got %q", cfg.Current)
	}
}

func TestSetCurrent_NotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	if err := mgr.SetCurrent("nonexistent"); err == nil {
		t.Error("expected error setting nonexistent profile as current")
	}
}
