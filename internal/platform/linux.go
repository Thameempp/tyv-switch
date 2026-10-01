//go:build linux

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

type linuxPlatform struct{}

func newPlatform() Platform { return &linuxPlatform{} }

func (p *linuxPlatform) OS() string   { return "Linux" }
func (p *linuxPlatform) Arch() string { return runtime.GOARCH }

func (p *linuxPlatform) AppDataDir() (string, error) {
	// XDG Base Directory Specification.
	// XDG_CONFIG_HOME defaults to $HOME/.config
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "tyv"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "tyv"), nil
}

func agyCandidates() []string {
	home, _ := os.UserHomeDir()
	return []string{
		"",
		filepath.Join(home, ".local", "bin", "agy"),
		filepath.Join(home, ".gemini", "bin", "agy"),
		"/usr/local/bin/agy",
		"/usr/bin/agy",
		"/bin/agy",
	}
}

func (p *linuxPlatform) FindAGY() (string, error) {
	return findAGY(agyCandidates())
}

func (p *linuxPlatform) Launch(executable string, args []string, env []string) error {
	argv := append([]string{executable}, args...)
	return syscall.Exec(executable, argv, env)
}

// findAGY and checkExecutable are shared via darwin.go at compile time only
// on darwin. On Linux we need them here directly.

func findAGY(candidates []string) (string, error) {
	if override := os.Getenv("TYV_AGY_PATH"); override != "" {
		if err := checkExecutable(override); err != nil {
			return "", fmt.Errorf("TYV_AGY_PATH=%q: %w", override, err)
		}
		return override, nil
	}

	if path, err := exec.LookPath("agy"); err == nil {
		return path, nil
	}

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
