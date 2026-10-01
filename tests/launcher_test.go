package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/thameem/tyv/internal/launcher"
	"github.com/thameem/tyv/internal/platform"
)

func TestProfileEnvIsolatesHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("home mirroring is Unix-only")
	}
	mgr, dir := newTestManager(t)
	if err := mgr.AddProfile("work", ""); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", "/should/be/replaced")
	t.Setenv("TYV_ACTIVE_PROFILE", "stale")

	env, err := launcher.New(platform.New(), mgr).ProfileEnv("work")
	if err != nil {
		t.Fatal(err)
	}

	wantHome := filepath.Join(dir, "profiles", "work", "home")
	counts := map[string]int{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "HOME":
			counts[k]++
			if v != wantHome {
				t.Errorf("HOME = %q, want %q", v, wantHome)
			}
		case "TYV_ACTIVE_PROFILE":
			counts[k]++
			if v != "work" {
				t.Errorf("TYV_ACTIVE_PROFILE = %q, want work", v)
			}
		}
	}
	if counts["HOME"] != 1 || counts["TYV_ACTIVE_PROFILE"] != 1 {
		t.Errorf("HOME/TYV_ACTIVE_PROFILE must appear exactly once, got %v", counts)
	}

	gemini := filepath.Join(wantHome, ".gemini")
	if fi, err := os.Lstat(gemini); err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf(".gemini must be a private real directory (err=%v)", err)
	}
}

func TestProfileEnvUnknownProfile(t *testing.T) {
	mgr, _ := newTestManager(t)
	if _, err := launcher.New(platform.New(), mgr).ProfileEnv("nope"); err == nil {
		t.Error("expected error for unknown profile")
	}
}

func TestRemoveProfileDoesNotFollowSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need elevated rights on Windows")
	}
	mgr, dir := newTestManager(t)
	if err := mgr.AddProfile("work", ""); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	keep := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "profiles", "work", "home")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, "linked")); err != nil {
		t.Fatal(err)
	}

	if err := mgr.RemoveProfile("work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("removing a profile must not delete files behind symlinks: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "profiles", "work")); !os.IsNotExist(err) {
		t.Error("profile directory should be gone")
	}
}

func TestProfileHomeKeepsKeychainPrivate(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS keychain directory only")
	}
	mgr, dir := newTestManager(t)
	if err := mgr.AddProfile("work", ""); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "profiles", "work", "home")

	// Simulate a home created by an older tyv, where Library was one symlink.
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(home, "Library")); err != nil {
		t.Fatal(err)
	}

	if _, err := launcher.New(platform.New(), mgr).ProfileEnv("work"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"Library", "Library/Keychains"} {
		fi, err := os.Lstat(filepath.Join(home, p))
		if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s must be a real directory owned by the profile (err=%v)", p, err)
		}
	}
}

func TestProfileKeychainIsolatedOnMac(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	mgr, _ := newTestManager(t)
	if err := mgr.AddProfile("work", ""); err != nil {
		t.Fatal(err)
	}
	env, err := launcher.New(platform.New(), mgr).ProfileEnv("work")
	if err != nil {
		t.Fatal(err)
	}
	home := ""
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "HOME="); ok {
			home = v
		}
	}
	kc := filepath.Join(home, "Library", "Keychains", "login.keychain-db")
	if _, err := os.Stat(kc); err != nil {
		t.Fatalf("profile keychain not created: %v", err)
	}
	// Stand-in for what agy does: store a secret via `security` with the
	// profile's HOME. It must land in the profile keychain, not the user's.
	const svc = "tyv-test-isolation-svc"
	cmd := exec.Command("/usr/bin/security", "add-generic-password", "-U", "-s", svc, "-a", "t", "-w", "x")
	cmd.Env = append(os.Environ(), "HOME="+home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add-generic-password: %v: %s", err, out)
	}
	find := exec.Command("/usr/bin/security", "find-generic-password", "-s", svc, kc)
	if err := find.Run(); err != nil {
		t.Errorf("secret not found in profile keychain: %v", err)
	}
}
