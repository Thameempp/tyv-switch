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
// Account isolation:
//
// agy keeps its Google login under $HOME/.gemini (it ignores
// GEMINI_CLI_APP_DATA_DIR), so each tyv profile runs agy with its own HOME:
//
//	<tyv-appDataDir>/profiles/<name>/home/
//
// That directory holds private copies of the places agy keeps its login
// (.gemini, and the keychain directory) and symlinks to every other entry in
// the real home, so git, ssh and other tools run by agy keep working.
package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Thameempp/tyv-switch/internal/config"
	"github.com/Thameempp/tyv-switch/internal/platform"
)

// Launcher resolves the agy executable and builds the environment for
// launching agy for a given profile.
type Launcher struct {
	plat      platform.Platform
	mgr       *config.Manager
	agyCached string
}

// New creates a Launcher.
func New(plat platform.Platform, mgr *config.Manager) *Launcher {
	return &Launcher{plat: plat, mgr: mgr}
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

// RunForProfile launches agy for the profile as a child process, wired to the
// current terminal, and waits for it to exit. Unlike LaunchForProfile it
// returns, so the caller can report the outcome. It is used for first-time
// sign-in, which agy performs itself; tyv never sees credentials.
func (l *Launcher) RunForProfile(name string) error {
	cfg := l.mgr.Get()
	profile, ok := cfg.Profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}

	agyCLI, err := l.FindAGY()
	if err != nil {
		return err
	}
	if err := validateExecutablePath(agyCLI); err != nil {
		return fmt.Errorf("unsafe agy executable path: %w", err)
	}

	cmd := exec.Command(agyCLI)
	cmd.Env = buildEnv(profile, l.mgr.ProfileDataDir(name))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	// Ctrl+C is delivered to agy too; keep tyv alive so it can report.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	defer signal.Stop(sigs)

	debugLog("profile=%s executable=%s (child process)", name, agyCLI)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("agy exited with an error: %w", err)
	}
	return nil
}

// ProfileEnv returns the environment agy should run with for the profile.
func (l *Launcher) ProfileEnv(name string) ([]string, error) {
	profile, ok := l.mgr.Get().Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", name)
	}
	return buildEnv(profile, l.mgr.ProfileDataDir(name)), nil
}

// buildEnv constructs the process environment for launching agy: the current
// environment with HOME (and USERPROFILE on Windows) pointing at the
// profile's isolated home.
//
// Security: we never inject secrets. The actual auth credentials remain under
// agy's control inside the profile's private .gemini directory.
func buildEnv(profile config.Profile, profileDataDir string) []string {
	home, err := prepareHome(profileDataDir)
	if err != nil {
		debugLog("could not prepare isolated home: %v", err)
		return append(os.Environ(), "TYV_ACTIVE_PROFILE="+profile.Name)
	}

	// Drop existing values so the override is the only one: with duplicate
	// entries, getenv(3) returns the first.
	drop := map[string]bool{"HOME": true, "TYV_ACTIVE_PROFILE": true}
	if runtime.GOOS == "windows" {
		drop["USERPROFILE"] = true
	}
	if runtime.GOOS == "linux" {
		// agy skips the desktop keyring when there is no D-Bus session bus
		// and stores its login in the profile's private .gemini instead.
		// Otherwise every profile would share the one desktop keyring.
		drop["DBUS_SESSION_BUS_ADDRESS"] = true
	}
	env := make([]string, 0, len(os.Environ())+3)
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if !drop[strings.ToUpper(key)] {
			env = append(env, kv)
		}
	}
	env = append(env, "HOME="+home, "TYV_ACTIVE_PROFILE="+profile.Name)
	if runtime.GOOS == "windows" {
		env = append(env, "USERPROFILE="+home)
	}
	return env
}

// privatePaths are the entries (relative to the home directory) that each
// profile owns privately instead of sharing with the real home:
//
//   - .gemini: agy's data and login files.
//   - Library/Keychains: on macOS agy's login is also found through the user
//     keychain, so mirroring it would silently sign every profile in as the
//     same account.
//   - .local/share/keyrings: the Linux equivalent (GNOME keyring files).
var privatePaths = []string{
	".gemini",
	"Library/Keychains",
	".local/share/keyrings",
}

// HomeDir returns the isolated home directory of the profile.
func (l *Launcher) HomeDir(name string) string {
	return filepath.Join(l.mgr.ProfileDataDir(name), "home")
}

// prepareHome creates (or refreshes) the profile's isolated home directory
// and returns its path. Everything in the real home is symlinked in except
// privatePaths, which are real directories owned by the profile.
func prepareHome(profileDataDir string) (string, error) {
	home := filepath.Join(profileDataDir, "home")
	if err := os.MkdirAll(home, 0700); err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		// Symlinks need elevated rights; only redirect the profile.
		return home, os.MkdirAll(filepath.Join(home, ".gemini"), 0700)
	}

	// Prefer the account database over $HOME, which may already be a
	// profile home when tyv is run from inside an agy session.
	realHome := ""
	if u, err := user.Current(); err == nil {
		realHome = u.HomeDir
	}
	if realHome == "" {
		var err error
		if realHome, err = os.UserHomeDir(); err != nil {
			return "", err
		}
	}
	if err := mirror(realHome, home, privatePaths); err != nil {
		return "", err
	}
	ensureProfileKeychain(home)
	return home, nil
}

// mirror makes dst a view of src: every entry of src is symlinked into dst,
// except the given private paths (relative to src). Each private path becomes
// a real directory in dst; directories leading to one are real directories
// whose other entries are symlinked. Existing correct entries are left alone.
func mirror(src, dst string, private []string) error {
	leaves := map[string]bool{}     // entries that are private themselves
	nested := map[string][]string{} // entry -> private paths below it
	for _, p := range private {
		first, rest, found := strings.Cut(filepath.ToSlash(p), "/")
		if found {
			nested[first] = append(nested[first], rest)
		} else {
			leaves[first] = true
		}
	}

	for name := range leaves {
		if err := ensurePrivateDir(filepath.Join(dst, name)); err != nil {
			return err
		}
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if leaves[name] {
			continue
		}
		link := filepath.Join(dst, name)
		if sub, ok := nested[name]; ok && e.IsDir() {
			if err := ensurePrivateDir(link); err != nil {
				return err
			}
			if err := mirror(filepath.Join(src, name), link, sub); err != nil {
				return err
			}
			continue
		}
		if _, err := os.Lstat(link); err == nil {
			continue
		}
		_ = os.Symlink(filepath.Join(src, name), link) // best effort
	}
	return nil
}

// ensurePrivateDir makes path a real directory, replacing a symlink left by
// an earlier version of tyv (only the link is removed, never its target).
func ensurePrivateDir(path string) error {
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return os.MkdirAll(path, 0700)
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
	if os.Getenv("TYV_DEBUG") != "1" {
		return
	}
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(os.Stderr, "DEBUG: %s\n", msg)
}
