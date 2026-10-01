//go:build darwin

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

type darwinPlatform struct{}

func newPlatform() Platform { return &darwinPlatform{} }

func (p *darwinPlatform) OS() string   { return "macOS" }
func (p *darwinPlatform) Arch() string { return runtime.GOARCH }

func (p *darwinPlatform) AppDataDir() (string, error) {
	// macOS convention: ~/Library/Application Support/<app>
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, "Library", "Application Support", "tyv"), nil
}

// agyCandidates lists search paths for the agy binary on macOS.
// We check PATH first (most flexible), then fall back to well-known locations.
func agyCandidates() []string {
	home, _ := os.UserHomeDir()
	return []string{
		// PATH lookup is always first.
		"",
		// Common install locations.
		filepath.Join(home, ".local", "bin", "agy"),
		filepath.Join(home, ".gemini", "bin", "agy"),
		"/opt/homebrew/bin/agy",
		"/usr/local/bin/agy",
		"/usr/bin/agy",
	}
}

func (p *darwinPlatform) FindAGY() (string, error) {
	return findAGY(agyCandidates())
}

func (p *darwinPlatform) Launch(executable string, args []string, env []string) error {
	// On macOS/Unix, replace the current process via execve(2).
	// This is the most efficient: no intermediate process, no wait overhead.
	argv := append([]string{executable}, args...)
	return syscall.Exec(executable, argv, env)
}

// findAGY is the shared lookup used by all Unix platforms.
// candidates[0] being empty triggers a PATH search via exec.LookPath.
func findAGY(candidates []string) (string, error) {
	// Check TYV_AGY_PATH override first.
	if override := os.Getenv("TYV_AGY_PATH"); override != "" {
		if err := checkExecutable(override); err != nil {
			return "", fmt.Errorf("TYV_AGY_PATH=%q: %w", override, err)
		}
		return override, nil
	}

	// PATH lookup.
	if path, err := exec.LookPath("agy"); err == nil {
		return path, nil
	}

	// Explicit candidate paths.
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if err := checkExecutable(c); err == nil {
			return c, nil
		}
	}

	return "", fmt.Errorf(
		"agy executable not found\n\n" +
			"Install Antigravity CLI from https://antigravity.google/download\n" +
			"or set TYV_AGY_PATH to the absolute path of the agy binary",
	)
}

func checkExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%q is a directory", path)
	}
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("%q is not executable", path)
	}
	return nil
}
