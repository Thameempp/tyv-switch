//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type windowsPlatform struct{}

func newPlatform() Platform { return &windowsPlatform{} }

func (p *windowsPlatform) OS() string   { return "Windows" }
func (p *windowsPlatform) Arch() string { return runtime.GOARCH }

func (p *windowsPlatform) AppDataDir() (string, error) {
	// Windows convention: %APPDATA%\sa
	appData := os.Getenv("APPDATA")
	if appData == "" {
		// Fallback: use USERPROFILE\AppData\Roaming
		profile := os.Getenv("USERPROFILE")
		if profile == "" {
			return "", fmt.Errorf("could not determine AppData directory: APPDATA and USERPROFILE are not set")
		}
		appData = filepath.Join(profile, "AppData", "Roaming")
	}
	return filepath.Join(appData, "sa"), nil
}

func agyCandidates() []string {
	localAppData := os.Getenv("LOCALAPPDATA")
	appData := os.Getenv("APPDATA")
	return []string{
		"",
		filepath.Join(localAppData, "Programs", "agy", "agy.exe"),
		filepath.Join(appData, "agy", "agy.exe"),
		filepath.Join(os.Getenv("PROGRAMFILES"), "agy", "agy.exe"),
	}
}

func (p *windowsPlatform) FindAGY() (string, error) {
	return findAGY(agyCandidates())
}

// Launch on Windows starts agy as a child process, waits for it, then exits
// with the child's exit code. Windows does not have execve(2).
func (p *windowsPlatform) Launch(executable string, args []string, env []string) error {
	cmd := exec.Command(executable, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("failed to launch agy: %w", err)
	}
	os.Exit(0)
	return nil // unreachable
}

func findAGY(candidates []string) (string, error) {
	if override := os.Getenv("SA_AGY_PATH"); override != "" {
		if err := checkExecutable(override); err != nil {
			return "", fmt.Errorf("SA_AGY_PATH=%q: %w", override, err)
		}
		return override, nil
	}

	// Try agy.exe on PATH.
	if path, err := exec.LookPath("agy"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("agy.exe"); err == nil {
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
			"or set SA_AGY_PATH to the absolute path of the agy executable",
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
	return nil
}
