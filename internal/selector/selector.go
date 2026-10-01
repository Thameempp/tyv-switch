// Package selector implements the interactive account picker shown by `tyv`
// when run with no arguments.
//
// The picker is stdlib-only: terminal raw mode is handled per platform in
// term_unix.go / term_windows.go, and key handling here works on any
// io.Reader so it can be tested without a TTY.
package selector

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Usage figures: a percentage (0–100), or one of these markers.
const (
	Unknown = -1 // not available (shown as "—")
	Loading = -2 // lookup in progress (shown as an animated spinner)
)

// Update replaces the usage figures of rows[Index] while the picker is open.
type Update struct {
	Index          int
	Gemini, Claude int
}

// Row is one selectable account. Usage values are percentages (0–100), or
// Unknown when no figure is available.
type Row struct {
	Name   string
	Gemini int
	Claude int
	Email  string // shown in the last column; "—" when empty
}

// Result describes how the picker ended.
type Result int

const (
	Selected    Result = iota // Enter pressed on a row
	Cancelled                 // Esc or q
	Interrupted               // Ctrl+C
)

type key int

const (
	keyNone key = iota
	keyUp
	keyDown
	keyEnter
	keyEsc
	keyCtrlC
)

const (
	hideCursor = "\x1b[?25l"
	showCursor = "\x1b[?25h"
	clearLine  = "\x1b[2K"
)

// escTimeout is how long to wait after ESC for the rest of an escape
// sequence before treating it as a bare Esc key press.
const escTimeout = 50 * time.Millisecond

// spinnerFrames animate figures that are still loading; spinnerInterval is
// the time between frames.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 100 * time.Millisecond

// Run draws the picker on out and processes keys from in until the user
// selects a row or exits. The terminal must already be in raw mode.
// start is the index of the initially highlighted row. Updates received on
// updates (which may be nil) refresh usage figures live.
func Run(in io.Reader, out io.Writer, rows []Row, start int, updates <-chan Update) (Result, int, error) {
	if len(rows) == 0 {
		return Cancelled, -1, nil
	}
	sel := start
	if sel < 0 || sel >= len(rows) {
		sel = 0
	}

	fmt.Fprint(out, hideCursor)
	defer fmt.Fprint(out, showCursor)

	frame := 0
	lines := render(out, rows, sel, 0, frame)
	keys := readKeys(in)
	tick := time.NewTicker(spinnerInterval)
	defer tick.Stop()

	finish := func(r Result, idx int) (Result, int, error) {
		erase(out, lines)
		return r, idx, nil
	}

	for {
		var k key
		select {
		case kk, ok := <-keys:
			if !ok {
				// Input closed (EOF) — treat as cancel.
				return finish(Cancelled, -1)
			}
			k = kk
		case <-tick.C:
			if anyLoading(rows) {
				frame++
				lines = render(out, rows, sel, lines, frame)
			}
			continue
		case u, ok := <-updates:
			if !ok {
				updates = nil
				continue
			}
			if u.Index >= 0 && u.Index < len(rows) {
				rows[u.Index].Gemini, rows[u.Index].Claude = u.Gemini, u.Claude
				lines = render(out, rows, sel, lines, frame)
			}
			continue
		}
		switch k {
		case keyUp:
			if sel > 0 {
				sel--
			} else {
				sel = len(rows) - 1
			}
		case keyDown:
			if sel < len(rows)-1 {
				sel++
			} else {
				sel = 0
			}
		case keyEnter:
			return finish(Selected, sel)
		case keyEsc:
			return finish(Cancelled, -1)
		case keyCtrlC:
			return finish(Interrupted, -1)
		default:
			continue
		}
		lines = render(out, rows, sel, lines, frame)
	}
}

func anyLoading(rows []Row) bool {
	for _, r := range rows {
		if r.Gemini == Loading || r.Claude == Loading {
			return true
		}
	}
	return false
}

// render draws the menu, first moving the cursor up over a previous draw of
// prevLines lines. It returns the number of lines written.
func render(out io.Writer, rows []Row, sel, prevLines, frame int) int {
	nameW, emailW := len("Account"), len("Email")
	for _, r := range rows {
		if n := len([]rune(r.Name)); n > nameW {
			nameW = n
		}
		if n := len([]rune(emailText(r))); n > emailW {
			emailW = n
		}
	}

	var b strings.Builder
	if prevLines > 0 {
		fmt.Fprintf(&b, "\x1b[%dA", prevLines)
	}
	n := 0
	line := func(s string) {
		b.WriteString("\r" + clearLine + s + "\r\n")
		n++
	}

	line("AI Account Usage")
	line("")
	line(fmt.Sprintf("  %-*s  %8s  %8s  %-*s", nameW, "Account", "Gemini", "Claude", emailW, "Email"))
	line("  " + strings.Repeat("─", nameW+2+8+2+8+2+emailW))
	for i, r := range rows {
		marker := " "
		if i == sel {
			marker = "❯"
		}
		line(fmt.Sprintf("%s %-*s  %8s  %8s  %s", marker, nameW, r.Name, pct(r.Gemini, frame), pct(r.Claude, frame), emailText(r)))
	}
	line("")
	line("  ↑/↓ Navigate   Enter Select   Esc Exit")

	io.WriteString(out, b.String())
	return n
}

// erase removes a previously drawn menu of the given height.
func erase(out io.Writer, lines int) {
	var b strings.Builder
	fmt.Fprintf(&b, "\x1b[%dA", lines)
	for i := 0; i < lines; i++ {
		b.WriteString("\r" + clearLine + "\n")
	}
	fmt.Fprintf(&b, "\x1b[%dA", lines)
	io.WriteString(out, b.String())
}

func emailText(r Row) string {
	if r.Email == "" {
		return "—"
	}
	return r.Email
}

func pct(v, frame int) string {
	switch {
	case v == Loading:
		return spinnerFrames[frame%len(spinnerFrames)]
	case v < 0:
		return "—"
	}
	return fmt.Sprintf("%d%%", v)
}

// readKeys decodes bytes from in into key events on the returned channel,
// which is closed when in returns an error or EOF.
func readKeys(in io.Reader) <-chan key {
	bytes := make(chan byte)
	go func() {
		defer close(bytes)
		buf := make([]byte, 1)
		for {
			if _, err := in.Read(buf); err != nil {
				return
			}
			bytes <- buf[0]
		}
	}()

	keys := make(chan key)
	go func() {
		defer close(keys)
		for b := range bytes {
			switch b {
			case 0x03:
				keys <- keyCtrlC
			case '\r', '\n':
				keys <- keyEnter
			case 'q', 'Q':
				keys <- keyEsc
			case 0x1b:
				keys <- decodeEscape(bytes)
			}
		}
	}()
	return keys
}

// decodeEscape handles the bytes following ESC. A bare ESC (nothing follows
// within escTimeout) is the Esc key; ESC [ A/B or ESC O A/B are arrows.
func decodeEscape(bytes <-chan byte) key {
	next := func() (byte, bool) {
		select {
		case b, ok := <-bytes:
			return b, ok
		case <-time.After(escTimeout):
			return 0, false
		}
	}
	b, ok := next()
	if !ok {
		return keyEsc
	}
	if b != '[' && b != 'O' {
		return keyNone
	}
	c, ok := next()
	if !ok {
		return keyNone
	}
	switch c {
	case 'A':
		return keyUp
	case 'B':
		return keyDown
	}
	// Other CSI sequences (e.g. ESC [ 1 ; 5 C): drain parameter bytes.
	for c >= 0x30 && c <= 0x3f {
		if c, ok = next(); !ok {
			break
		}
	}
	return keyNone
}
