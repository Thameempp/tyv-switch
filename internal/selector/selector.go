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
	"os"
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
	Index                    int
	Gemini, Claude           int
	GeminiReset, ClaudeReset time.Time
}

// Row is one selectable account. Usage values are percentages (0–100), or
// Unknown when no figure is available.
type Row struct {
	Name   string
	Gemini int
	Claude int
	Email  string // shown in the last column; "—" when empty

	// Current marks the profile tyv last switched to; its name and email are
	// drawn in blue.
	Current bool

	// GeminiReset and ClaudeReset are when the limiting quota window resets;
	// the zero time means unknown (no countdown is shown).
	GeminiReset, ClaudeReset time.Time
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
	keyRefresh
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
// updates (which may be nil) refresh usage figures live. When the user
// presses r, every row goes back to Loading and refresh (if non-nil) is
// called to start new lookups; r is ignored while lookups are still running.
func Run(in io.Reader, out io.Writer, rows []Row, start int, updates <-chan Update, refresh func()) (Result, int, error) {
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
				r := &rows[u.Index]
				r.Gemini, r.Claude = u.Gemini, u.Claude
				r.GeminiReset, r.ClaudeReset = u.GeminiReset, u.ClaudeReset
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
		case keyRefresh:
			if refresh == nil || anyLoading(rows) {
				continue
			}
			for i := range rows {
				rows[i].Gemini, rows[i].Claude = Loading, Loading
				rows[i].GeminiReset, rows[i].ClaudeReset = time.Time{}, time.Time{}
			}
			refresh()
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
	now := time.Now()
	nameW, gemW, claW, emailW := len("Account"), minCellW, minCellW, len("Email")
	for _, r := range rows {
		nameW = max(nameW, len([]rune(r.Name)))
		gemW = max(gemW, len([]rune(cell(r.Gemini, r.GeminiReset, frame, now))))
		claW = max(claW, len([]rune(cell(r.Claude, r.ClaudeReset, frame, now))))
		emailW = max(emailW, len([]rune(emailText(r))))
	}
	colored := UseColor()
	// padded right-aligns a usage cell to width w. Padding is computed on the
	// plain text so the colour escape codes do not disturb the alignment.
	padded := func(v int, reset time.Time, w int) string {
		plain := cell(v, reset, frame, now)
		text := plain
		if colored {
			text = colorCell(v, reset, frame, now)
		}
		return strings.Repeat(" ", max(0, w-len([]rune(plain)))) + text
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
	line(fmt.Sprintf("  %-*s  %*s  %*s  %-*s", nameW, "Account", gemW, "Gemini", claW, "Claude", emailW, "Email"))
	line("  " + strings.Repeat("─", nameW+2+gemW+2+claW+2+emailW))
	for i, r := range rows {
		marker := " "
		if i == sel {
			marker = "❯"
		}
		name := r.Name + strings.Repeat(" ", max(0, nameW-len([]rune(r.Name))))
		email := emailText(r)
		if colored && r.Current {
			name = Blue(r.Name) + strings.Repeat(" ", max(0, nameW-len([]rune(r.Name))))
			email = Blue(email)
		}
		line(fmt.Sprintf("%s %s  %s  %s  %s", marker, name,
			padded(r.Gemini, r.GeminiReset, gemW),
			padded(r.Claude, r.ClaudeReset, claW), email))
	}
	line("")
	line("  ↑/↓ Navigate   Enter Select   R Refresh   Esc Exit")

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

// minCellW keeps the usage columns from reflowing when numbers replace the
// loading spinner ("100% (6d 23h 59m)").
const minCellW = 17

// cell renders one usage figure, with the time until its quota resets in
// brackets (for example "53% (4h 12m)") when known.
func cell(v int, reset time.Time, frame int, now time.Time) string {
	s := pct(v, frame)
	if v >= 0 && !reset.IsZero() {
		s += " (" + FormatRemaining(reset.Sub(now)) + ")"
	}
	return s
}

// ANSI colours for the percentage: plenty left, getting low, nearly out.
const (
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiReset  = "\x1b[0m"
)

// quotaColor picks the colour for a remaining percentage: 50% and above is
// green, 20–49% yellow, below 20% red. Unknown and loading values have none.
func quotaColor(v int) string {
	switch {
	case v < 0:
		return ""
	case v >= 50:
		return ansiGreen
	case v >= 20:
		return ansiYellow
	default:
		return ansiRed
	}
}

// UseColor reports whether to colour output; NO_COLOR (no-color.org) or a
// dumb terminal turns it off.
func UseColor() bool {
	return os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

// ansiBlue is bright blue, which stays readable on dark terminals.
const ansiBlue = "\x1b[94m"

// Blue wraps text in blue; callers should check UseColor first.
func Blue(text string) string { return ansiBlue + text + ansiReset }

// colorCell is cell with the percentage coloured by quotaColor.
func colorCell(v int, reset time.Time, frame int, now time.Time) string {
	plain := cell(v, reset, frame, now)
	c := quotaColor(v)
	if c == "" {
		return plain
	}
	p := pct(v, frame)
	return c + p + ansiReset + strings.TrimPrefix(plain, p)
}

// FormatRemaining formats a duration for people, dropping empty leading
// units: "6d 14h 5m", "4h 12m", "35m". Under a minute (or already past) it
// is "0m".
func FormatRemaining(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	mins := int(d / time.Minute)
	days, hours, mins := mins/(24*60), mins/60%24, mins%60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
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
			case 'r', 'R':
				keys <- keyRefresh
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
