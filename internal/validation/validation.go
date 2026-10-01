// Package validation provides safe input validation for profile names.
// It enforces strict rules to prevent path traversal, command injection,
// and other security issues with user-controlled names.
package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// reservedNames is the complete set of command names that cannot be used as
// profile names, preventing ambiguity in the command dispatcher.
var reservedNames = map[string]bool{
	"add":     true,
	"list":    true,
	"current": true,
	"remove":  true,
	"rename":  true,
	"doctor":  true,
	"version": true,
	"help":    true,
	"login":   true,
	"logout":  true,
}

// profileNameRe matches valid profile names: lowercase letters, digits,
// hyphens, and underscores. Must start with a letter or digit.
var profileNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9\-_]{0,62}$`)

// ProfileName validates a profile name and returns a descriptive error if
// invalid. Valid names are:
//   - 1–63 characters
//   - lowercase letters, digits, hyphens, underscores only
//   - must start with a letter or digit
//   - must not be a reserved command name
//   - must not contain path separators or traversal sequences
func ProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}

	// Reject any path separators or traversal patterns before regex check.
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("profile name %q contains invalid character: path separators are not allowed", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("profile name %q contains invalid sequence: '..' is not allowed", name)
	}

	// Control characters and shell metacharacters.
	for _, ch := range name {
		if ch < 0x20 || ch == 0x7F {
			return fmt.Errorf("profile name %q contains a control character", name)
		}
	}

	if !profileNameRe.MatchString(name) {
		return fmt.Errorf(
			"profile name %q is invalid\n\n"+
				"Profile names must:\n"+
				"  - Start with a lowercase letter or digit\n"+
				"  - Contain only lowercase letters, digits, hyphens, or underscores\n"+
				"  - Be 1–63 characters long\n\n"+
				"Examples: personal, work, my-work, work2",
			name,
		)
	}

	if reservedNames[name] {
		return fmt.Errorf(
			"profile name %q is a reserved command name\n\n"+
				"Reserved names: add, list, current, remove, rename, doctor, version, help, login, logout\n\n"+
				"Choose a different name, for example: %s-profile",
			name, name,
		)
	}

	return nil
}

// IsReserved reports whether name is a reserved command name.
func IsReserved(name string) bool {
	return reservedNames[name]
}
