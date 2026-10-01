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
	var out bytes.Buffer
	res, idx, err := selector.Run(strings.NewReader(input), &out, rows, start, nil)
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
	r := []selector.Row{{Name: "a", Gemini: selector.Loading, Claude: selector.Loading}}
	upd := make(chan selector.Update, 1)
	upd <- selector.Update{Index: 0, Gemini: 42, Claude: 7}
	close(upd)

	pr, pw := io.Pipe()
	var out bytes.Buffer
	done := make(chan struct{})
	go func() {
		selector.Run(pr, &out, r, 0, upd)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	pw.Write([]byte("q"))
	<-done
	if s := out.String(); !strings.Contains(s, "⠋") || !strings.Contains(s, "42%") || !strings.Contains(s, "7%") {
		t.Errorf("expected spinner then updated figures, got %q", s)
	}
}
