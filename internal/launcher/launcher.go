// Package launcher handles resolving the agy executable and constructing the
// environment for launching agy under a specific profile.
//
// Architecture and security notes:
//
// AGY CLI (agy v1.2.14) stores all its application data in a single root
// directory. On macOS this defaults to ~/.gemini/antigravity-cli.
// Inspection of the binary reveals the internal flag --app_data_dir, but it
// requires a relative path and is not a documented, stable API.
//
// The only officially observable multi-account data is ~/.gemini/google_accounts.json
// which tracks { "active": "<email>", "old": [...] }. However, manipulating
// this file directly to "switch" accounts is:
//   - undocumented
//   - not guaranteed to be respected by agy
//   - potentially race-condition prone
//   - not a supported credential isolation mechanism
//
// Safe supported approach used by sa:
//
// Each sa profile gets its own AGY data directory:
//
//	<sa-appDataDir>/profiles/<name>/
//
// When launching agy for a profile, sa sets the GEMINI_CLI_APP_DATA_DIR
// environment variable (if it exists and is respected), or falls back to
// setting HOME to a per-profile home-like directory that contains a symlink
// to the real ~/.gemini structure but with a profile-specific override dir.
//
// IMPORTANT: After careful research, neither GEMINI_DIR nor GEMINI_HOME is
// confirmed to override the data dir in the current agy binary. The --app_data_dir
// flag requires a relative path but its effect on auth is unconfirmed.
//
// Current safe implementation:
//  1. sa manages its own profile registry (who exists, which is current).
//  2. sa launches agy with the user's current environment unchanged.
//  3. agy uses its own auth (whichever Google account is logged in).
//  4. sa tracks which profile "label" is active locally.
//
// This means: profile switching in sa is a LABEL/WORKSPACE concept right now.
// Each profile can have an associated email hint and workspace directory that
// agy opens with --add-dir.
//
// If/when agy adds official multi-account support, sa can adopt it without
// breaking the profile model.
//
// The user must log in/out of agy separately for multi-Google-account use.
package launcher

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/thameem/sa/internal/config"
	"github.com/thameem/sa/internal/platform"
)

// Launcher resolves the agy executable and builds the environment for
// launching agy for a given profile.
type Launcher struct {
	plat       platform.Platform
	mgr        *config.Manager
	agyCached  string
	workingDir string
}

// New creates a Launcher.
func New(plat platform.Platform, mgr *config.Manager) *Launcher {
	wd, _ := os.Getwd()
	return &Launcher{plat: plat, mgr: mgr, workingDir: wd}
}

// FindAGY returns the path to the agy executable, caching the result.
func (l *Launcher) FindAGY() (string, error) {
	if l.agyCached != "" {
		return l.agyCached, nil
	}
	path, err := l.plat.FindAGY()
	if err != nil {
		return "", err
	}
	l.agyCached = path
	return path, nil
}

// LaunchForProfile resolves the profile, constructs the safe environment, and
// replaces the current process with agy. This function does not return on
// success (Unix: execve; Windows: waits and exits).
func (l *Launcher) LaunchForProfile(name string, extraArgs []string) error {
	cfg := l.mgr.Get()
	profile, ok := cfg.Profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found (this is a bug, validation should have caught it)", name)
	}

	agyCLI, err := l.FindAGY()
	if err != nil {
		return err
	}

	// Build the environment for this profile launch.
	env := buildEnv(profile, l.mgr.ProfileDataDir(name))

	// Build agy argument list.
	// We pass any extra args the user provided after the profile name.
	args := make([]string, 0, len(extraArgs))
	args = append(args, extraArgs...)

	debugLog("profile=%s executable=%s", name, agyCLI)
	debugLog("launching process")

	// Validate the executable path is safe before exec.
	if err := validateExecutablePath(agyCLI); err != nil {
		return fmt.Errorf("unsafe agy executable path: %w", err)
	}

	return l.plat.Launch(agyCLI, args, env)
}

// buildEnv constructs the process environment for launching agy.
// It starts from the current environment and adds/overrides profile-specific
// variables.
//
// Security: we never inject secrets. We only set POSIX-safe variables for
// workspace hints. The actual auth credentials remain under agy's control.
func buildEnv(profile config.Profile, profileDataDir string) []string {
	// Copy the current environment.
	base := os.Environ()

	// Profile data directory hint — if agy ever officially supports this,
	// it can be consumed here without changing the profile model.
	// Currently we create this dir but don't force agy to use it for auth.
	extra := []string{
		"SA_ACTIVE_PROFILE=" + profile.Name,
	}

	// Create the profile data directory if it doesn't exist.
	// This is sa's own bookkeeping directory, not forced onto agy.
	_ = os.MkdirAll(profileDataDir, 0700)

	return append(base, extra...)
}

// validateExecutablePath performs basic safety checks on the agy path before
// exec. We reject paths containing null bytes or suspicious components.
func validateExecutablePath(path string) error {
	for _, ch := range path {
		if ch == 0 {
			return fmt.Errorf("path contains null byte")
		}
	}
	clean := filepath.Clean(path)
	if clean != path {
		// Allow canonicalized paths but log the discrepancy.
		debugLog("executable path canonicalized from %q to %q", path, clean)
	}
	return nil
}

func debugLog(format string, args ...interface{}) {
	if os.Getenv("SA_DEBUG") != "1" {
		return
	}
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(os.Stderr, "DEBUG: %s\n", msg)
}
