// Package platform provides a cross-platform abstraction for OS-specific
// operations such as configuration directories, executable discovery, and
// process launching. All platform-specific code lives in separate build-tagged
// files; this file defines the shared interface.
package platform

import "os"

// Platform abstracts OS-specific operations. Use New() to obtain the correct
// implementation for the current OS.
type Platform interface {
	// OS returns a human-readable OS name (e.g. "macOS", "Linux", "Windows").
	OS() string

	// Arch returns the current CPU architecture (e.g. "arm64", "amd64").
	Arch() string

	// AppDataDir returns the root directory for tyv's application data.
	// This is where tyv stores its own configuration (not AGY config).
	// On macOS:  ~/Library/Application Support/tyv
	// On Linux:  $XDG_CONFIG_HOME/tyv  (default: ~/.config/tyv)
	// On Windows: %APPDATA%\tyv
	AppDataDir() (string, error)

	// FindAGY searches common locations for the agy executable and returns
	// the absolute path. Returns an error if not found.
	FindAGY() (string, error)

	// Launch replaces the current process with agy using the given arguments
	// and environment. On Unix this is execve(2); on Windows this starts a
	// child process and waits, then exits with the child's code.
	Launch(executable string, args []string, env []string) error
}

// New returns the Platform implementation for the current OS.
func New() Platform {
	return newPlatform()
}

// LookupEnv is a helper that returns an env var value or a default.
func LookupEnv(key, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return defaultVal
}
