package tests

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Thameempp/tyv-switch/internal/selector"
)

var rows = []selector.Row{
	{Name: "personal", Gemini: 100, Claude: 98},
	{Name: "work", Gemini: 11, Claude: 77},
	{Name: "university", Gemini: selector.Unknown, Claude: selector.Unknown},
}

func runKeys(t *testing.T, input string, start int) (selector.Result, int, string) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	var out bytes.Buffer
	res, idx, err := selector.Run(strings.NewReader(input), &out, rows, start, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res, idx, out.String()
}

func TestSelectorNavigationAndEnter(t *testing.T) {
	res, idx, out := runKeys(t, "\x1b[B\x1b[B\r", 0)
	if res != selector.Selected || idx != 2 {
		t.Fatalf("got res=%v idx=%d, want Selected/2", res, idx)
	}
	for _, want := range []string{"AI Account Usage", "❯ personal", "100%", "98%", "↑/↓ Navigate"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestSelectorUpWraps(t *testing.T) {
	if _, idx, _ := runKeys(t, "\x1b[A\r", 0); idx != 2 {
		t.Fatalf("idx = %d, want 2 (wrap)", idx)
	}
}

func TestSelectorExitKeys(t *testing.T) {
	if res, idx, _ := runKeys(t, "q", 0); res != selector.Cancelled || idx != -1 {
		t.Errorf("q: got %v/%d", res, idx)
	}
	if res, _, _ := runKeys(t, "\x1b", 0); res != selector.Cancelled {
		t.Errorf("Esc: got %v", res)
	}
	if res, _, _ := runKeys(t, "\x03", 0); res != selector.Interrupted {
		t.Errorf("Ctrl+C: got %v", res)
	}
}

func TestSelectorLiveUpdate(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	r := []selector.Row{{Name: "a", Gemini: selector.Loading, Claude: selector.Loading}}
	upd := make(chan selector.Update, 1)
	upd <- selector.Update{Index: 0, Gemini: 42, Claude: 7}
	close(upd)

	pr, pw := io.Pipe()
	var out bytes.Buffer
	done := make(chan struct{})
	go func() {
		selector.Run(pr, &out, r, 0, upd, nil)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	pw.Write([]byte("q"))
	<-done
	if s := out.String(); !strings.Contains(s, "⠋") || !strings.Contains(s, "42%") || !strings.Contains(s, "7%") {
		t.Errorf("expected spinner then updated figures, got %q", s)
	}
}

func TestFormatRemaining(t *testing.T) {
	cases := map[time.Duration]string{
		0:                              "0m",
		-time.Hour:                     "0m",
		59 * time.Second:               "0m",
		5*time.Minute + 59*time.Second: "5m",
		4*time.Hour + 12*time.Minute:   "4h 12m",
		3 * time.Hour:                  "3h 0m",
		2*24*time.Hour + 5*time.Minute: "2d 0h 5m",
		6*24*time.Hour + 14*time.Hour + 5*time.Minute: "6d 14h 5m",
	}
	for d, want := range cases {
		if got := selector.FormatRemaining(d); got != want {
			t.Errorf("FormatRemaining(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestSelectorShowsResetCountdown(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	r := []selector.Row{{
		Name: "a", Gemini: 100, Claude: 0, Email: "a@b.c",
		GeminiReset: time.Now().Add(2*24*time.Hour + 3*time.Hour + 30*time.Minute + 30*time.Second),
		ClaudeReset: time.Now().Add(95*time.Minute + 30*time.Second),
	}}
	var out bytes.Buffer
	if _, _, err := selector.Run(strings.NewReader("q"), &out, r, 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"100% (2d 3h 30m)", "0% (1h 35m)", "a@b.c"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
}

func TestSelectorColoursPercentages(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	rows := []selector.Row{
		{Name: "a", Gemini: 100, Claude: 53},
		{Name: "b", Gemini: 49, Claude: 20},
		{Name: "c", Gemini: 19, Claude: 0},
		{Name: "d", Gemini: selector.Unknown, Claude: selector.Loading},
	}
	var out bytes.Buffer
	if _, _, err := selector.Run(strings.NewReader("q"), &out, rows, 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{
		"\x1b[32m100%\x1b[0m", "\x1b[32m53%\x1b[0m", // green: 50% and up
		"\x1b[33m49%\x1b[0m", "\x1b[33m20%\x1b[0m", // yellow: 20-49%
		"\x1b[31m19%\x1b[0m", "\x1b[31m0%\x1b[0m", // red: below 20%
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}

	t.Setenv("NO_COLOR", "1")
	out.Reset()
	selector.Run(strings.NewReader("q"), &out, rows, 0, nil, nil)
	if strings.Contains(out.String(), "\x1b[3") {
		t.Error("NO_COLOR must disable colours")
	}
}

func TestSelectorRefreshKey(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	r := []selector.Row{{Name: "a", Gemini: 80, Claude: 40}}
	upd := make(chan selector.Update, 1)
	calls := 0
	refresh := func() {
		calls++
		upd <- selector.Update{Index: 0, Gemini: 10, Claude: 5}
	}

	pr, pw := io.Pipe()
	var out bytes.Buffer
	done := make(chan struct{})
	go func() {
		selector.Run(pr, &out, r, 0, upd, refresh)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	pw.Write([]byte("r"))
	time.Sleep(150 * time.Millisecond) // spinner shows, then the update lands
	pw.Write([]byte("q"))
	<-done

	if calls != 1 {
		t.Errorf("refresh called %d times, want 1", calls)
	}
	s := out.String()
	for _, want := range []string{"80%", "⠋", "10%", "R Refresh"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestSelectorRefreshIgnoredWhileLoading(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	r := []selector.Row{{Name: "a", Gemini: selector.Loading, Claude: selector.Loading}}
	calls := 0
	_, _, err := selector.Run(strings.NewReader("rrq"), io.Discard, r, 0, nil, func() { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Errorf("refresh called %d times while loading, want 0", calls)
	}
}

func TestSelectorHighlightsCurrentInBlue(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	rows := []selector.Row{
		{Name: "pro1", Gemini: 99, Claude: 100, Email: "me@x.io", Current: true},
		{Name: "pro2", Gemini: 10, Claude: 10, Email: "you@x.io"},
	}
	var out bytes.Buffer
	if _, _, err := selector.Run(strings.NewReader("q"), &out, rows, 1, nil, nil); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"\x1b[94mpro1\x1b[0m", "\x1b[94mme@x.io\x1b[0m"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(s, "\x1b[94mpro2") || strings.Contains(s, "\x1b[94myou@x.io") {
		t.Error("only the current profile may be blue")
	}
}
