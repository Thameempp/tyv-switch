//go:build !windows

package selector

import (
	"os"
	"os/exec"
	"strings"
)

// IsTerminal reports whether both stdin and stdout are interactive terminals.
func IsTerminal() bool {
	return isChar(os.Stdin) && isChar(os.Stdout)
}

// isChar reports whether f is a character device other than /dev/null,
// which also has ModeCharDevice set but is not a terminal.
func isChar(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(fi, null) {
		return false
	}
	return true
}

// MakeRaw puts the terminal into raw mode (no echo, no line buffering, no
// signal generation) and returns a function that restores the prior state.
// It shells out to stty to stay within the standard library.
func MakeRaw() (restore func(), err error) {
	get := exec.Command("stty", "-g")
	get.Stdin = os.Stdin
	saved, err := get.Output()
	if err != nil {
		return nil, err
	}
	state := strings.TrimSpace(string(saved))

	raw := exec.Command("stty", "raw", "-echo")
	raw.Stdin = os.Stdin
	if err := raw.Run(); err != nil {
		return nil, err
	}

	return func() {
		r := exec.Command("stty", state)
		r.Stdin = os.Stdin
		_ = r.Run()
	}, nil
}
