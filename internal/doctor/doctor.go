// Package doctor implements the `sa doctor` diagnostic command.
// It inspects the environment and reports status without exposing secrets.
package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thameem/sa/internal/config"
	"github.com/thameem/sa/internal/platform"
)

// Result holds the outcome of a doctor inspection.
type Result struct {
	Platform    PlatformInfo
	AGY         AGYInfo
	Config      ConfigInfo
	Profiles    ProfileInfo
	Network     NetworkInfo
	Credentials CredentialInfo
	Ready       bool
	Issues      []string
}

// PlatformInfo describes the current OS/architecture.
type PlatformInfo struct {
	OS   string
	Arch string
}

// AGYInfo describes the agy executable status.
type AGYInfo struct {
	Detected bool
	Path     string
	Error    string
}

// ConfigInfo describes the sa configuration state.
type ConfigInfo struct {
	Dir    string
	Status string
	Error  string
}

// ProfileInfo describes the profile registry.
type ProfileInfo struct {
	Count   int
	Current string
}

// NetworkInfo describes network requirements.
type NetworkInfo struct {
	Note string
}

// CredentialInfo describes credential storage status.
// NEVER includes actual credentials.
type CredentialInfo struct {
	Note string
}

// Run performs all diagnostic checks and returns a Result.
func Run(plat platform.Platform, mgr *config.Manager, appDataDir string) Result {
	r := Result{
		Network: NetworkInfo{
			Note: "Not required for local profile operations",
		},
		Credentials: CredentialInfo{
			Note: "Managed exclusively by Antigravity CLI (agy) — sa does not access credentials",
		},
	}

	// Platform
	r.Platform = PlatformInfo{
		OS:   plat.OS(),
		Arch: plat.Arch(),
	}

	// AGY executable
	agyCLI, err := plat.FindAGY()
	if err != nil {
		r.AGY = AGYInfo{
			Detected: false,
			Error:    err.Error(),
		}
		r.Issues = append(r.Issues, "agy executable not found")
	} else {
		r.AGY = AGYInfo{
			Detected: true,
			Path:     agyCLI,
		}
	}

	// Config directory and file
	if _, err := os.Stat(appDataDir); os.IsNotExist(err) {
		r.Config = ConfigInfo{
			Dir:    appDataDir,
			Status: "not initialised (no profiles created yet)",
		}
	} else {
		configPath := filepath.Join(appDataDir, "sa.json")
		if corrupted, desc := config.IsCorrupted(configPath); corrupted {
			r.Config = ConfigInfo{
				Dir:    appDataDir,
				Status: "CORRUPTED",
				Error:  desc,
			}
			r.Issues = append(r.Issues, fmt.Sprintf("config file corrupted: %s", desc))
		} else {
			r.Config = ConfigInfo{
				Dir:    appDataDir,
				Status: "OK",
			}
		}
	}

	// Profiles
	cfg := mgr.Get()
	r.Profiles = ProfileInfo{
		Count:   len(cfg.Profiles),
		Current: cfg.Current,
	}

	// Permissions check on appDataDir.
	if info, err := os.Stat(appDataDir); err == nil {
		mode := info.Mode()
		if mode.Perm()&0077 != 0 {
			r.Issues = append(r.Issues,
				fmt.Sprintf("config directory %q has overly permissive permissions (%v); expected 0700", appDataDir, mode.Perm()))
		}
	}

	r.Ready = len(r.Issues) == 0
	return r
}

// Print writes a human-readable doctor report to stdout.
// NEVER prints credentials, tokens, or secrets.
func Print(r Result) {
	section := func(name string) {
		fmt.Printf("\n%s\n", name)
	}
	item := func(label, value string) {
		fmt.Printf("  %-20s %s\n", label+":", value)
	}

	fmt.Println("sa doctor")

	section("Platform")
	item("OS", r.Platform.OS)
	item("Architecture", r.Platform.Arch)

	section("Antigravity CLI")
	if r.AGY.Detected {
		item("Status", "detected")
		item("Path", r.AGY.Path)
	} else {
		item("Status", "NOT FOUND")
		item("Error", r.AGY.Error)
	}

	section("Configuration")
	item("Directory", r.Config.Dir)
	item("Status", r.Config.Status)
	if r.Config.Error != "" {
		item("Error", r.Config.Error)
	}

	section("Profiles")
	item("Count", fmt.Sprintf("%d", r.Profiles.Count))
	if r.Profiles.Current != "" {
		item("Current", r.Profiles.Current)
	} else {
		item("Current", "(none)")
	}

	section("Credential Storage")
	item("Note", r.Credentials.Note)

	section("Network")
	item("Note", r.Network.Note)

	section("Status")
	if r.Ready {
		fmt.Println("  Ready")
	} else {
		fmt.Println("  Issues detected:")
		for _, issue := range r.Issues {
			fmt.Println("    - " + wrapLine(issue, 70, "      "))
		}
	}
	fmt.Println()
}

// wrapLine wraps a long line at word boundaries.
func wrapLine(s string, width int, indent string) string {
	if len(s) <= width {
		return s
	}
	words := strings.Fields(s)
	var lines []string
	current := ""
	for _, w := range words {
		if current == "" {
			current = w
		} else if len(current)+1+len(w) <= width {
			current += " " + w
		} else {
			lines = append(lines, current)
			current = indent + w
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}
