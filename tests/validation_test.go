package tests

import (
	"testing"

	"github.com/Thameempp/tyv-switch/internal/validation"
)

func TestProfileName_Valid(t *testing.T) {
	valid := []string{
		"personal",
		"work",
		"my-work",
		"work2",
		"a",
		"a1",
		"work-2",
		"my_profile",
		"university",
		"a-b-c-d",
		"profile123",
		"x",
	}
	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			if err := validation.ProfileName(name); err != nil {
				t.Errorf("expected %q to be valid, got error: %v", name, err)
			}
		})
	}
}

func TestProfileName_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		reason string
	}{
		{"", "empty"},
		{"-leading-hyphen", "starts with hyphen"},
		{"_leading-underscore", "starts with underscore"},
		{"UPPER", "uppercase letters"},
		{"spa ce", "space"},
		{"tab\there", "tab character"},
		{"new\nline", "newline"},
		{"../../secret", "path traversal"},
		{"../etc", "path traversal"},
		{"profile/subdir", "path separator slash"},
		{"profile\\subdir", "path separator backslash"},
		{"profile; rm -rf /", "semicolon shell metachar"},
		{"$(echo)", "dollar paren"},
		{"`backtick`", "backtick"},
		{"profile&bg", "ampersand"},
		{"profile|pipe", "pipe"},
		{"profile>redirect", "redirect"},
		{"profile<redirect", "redirect lt"},
		{"profile*glob", "glob star"},
		{"profile?glob", "glob question"},
		// Path traversal inside name.
		{"my..name", "double dot"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			if err := validation.ProfileName(tt.name); err == nil {
				t.Errorf("expected %q to be invalid (%s), but no error returned", tt.name, tt.reason)
			}
		})
	}
}

func TestProfileName_Reserved(t *testing.T) {
	reserved := []string{
		"add", "list", "current", "remove", "rename",
		"doctor", "version", "help", "login", "logout",
	}
	for _, name := range reserved {
		t.Run(name, func(t *testing.T) {
			if err := validation.ProfileName(name); err == nil {
				t.Errorf("expected reserved name %q to be rejected, but no error returned", name)
			}
		})
	}
}

func TestIsReserved(t *testing.T) {
	if !validation.IsReserved("add") {
		t.Error("expected 'add' to be reserved")
	}
	if validation.IsReserved("personal") {
		t.Error("expected 'personal' to not be reserved")
	}
}
