// Package antigravity reads model quota for the Antigravity account that agy
// is signed into. It runs the documented headless command
//
//	agy --print /usage --output-format json
//
// and parses its structured output. tyv never reads agy's credential files
// and makes no network requests itself; agy does the authenticated lookup.
// If agy's output format changes, only this package needs to change.
package antigravity

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Unknown marks a percentage that could not be determined.
const Unknown = -1

// Timeout bounds a single quota lookup (agy can take ~10s).
const Timeout = 40 * time.Second

// Usage holds remaining quota as whole percentages (0–100).
type Usage struct {
	Gemini int
	Claude int
}

// Fetch asks agy for the quota of the account it is signed into under env.
func Fetch(ctx context.Context, agyPath string, env []string) (Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	// A profile that is not signed in makes agy print a login URL and then
	// wait for a code; stop it as soon as that message appears.
	out := &loginWatcher{cancel: cancel}
	cmd := exec.CommandContext(ctx, agyPath, "--print", "/usage", "--output-format", "json")
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = out, out // agy prints the login prompt on stderr
	cmd.WaitDelay = 2 * time.Second   // don't hang on grandchildren holding the pipe
	err := cmd.Run()

	if notSignedIn(out.buf.Bytes()) {
		return Usage{Unknown, Unknown}, errNotSignedIn
	}
	if ctx.Err() != nil {
		return Usage{Unknown, Unknown}, fmt.Errorf("usage lookup timed out")
	}
	if err != nil && out.buf.Len() == 0 {
		return Usage{Unknown, Unknown}, fmt.Errorf("agy failed: %w", err)
	}
	return Parse(out.buf.Bytes())
}

// loginWatcher collects agy's output and cancels the lookup once agy asks
// the user to log in.
type loginWatcher struct {
	buf    bytes.Buffer
	cancel context.CancelFunc
}

func (w *loginWatcher) Write(p []byte) (int, error) {
	n, err := w.buf.Write(p)
	if notSignedIn(w.buf.Bytes()) {
		w.cancel()
	}
	return n, err
}

// notSignedIn reports whether agy asked for login instead of answering.
func notSignedIn(out []byte) bool {
	s := strings.ToLower(string(out))
	return strings.Contains(s, "authentication required") || strings.Contains(s, "authentication failed")
}
