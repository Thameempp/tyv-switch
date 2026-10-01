// Package exitcode defines standard exit codes for the sa CLI.
package exitcode

const (
	// Success indicates a successful command.
	Success = 0
	// GeneralError indicates a general/unexpected error.
	GeneralError = 1
	// InvalidUsage indicates incorrect command-line usage.
	InvalidUsage = 2
	// ProfileNotFound indicates the requested profile does not exist.
	ProfileNotFound = 3
	// AuthConfigError indicates an authentication or configuration error.
	AuthConfigError = 4
	// ExecutableNotFound indicates the agy executable was not found.
	ExecutableNotFound = 5
	// PermissionError indicates a filesystem permission error.
	PermissionError = 6
)
