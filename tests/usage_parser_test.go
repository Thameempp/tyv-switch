package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Thameempp/tyv-switch/internal/providers/antigravity"
)

const sampleUsage = `{"status":"SUCCESS","command":{"name":"usage","data":{"groups":[
{"name":"Gemini Models","buckets":[{"remaining_fraction":0.9991},{"remaining_fraction":0.9949}]},
{"name":"Claude and GPT models","buckets":[{"remaining_fraction":0.6634},{"remaining_fraction":0}]}]}}}`

func TestParseUsageUsesLowestBucket(t *testing.T) {
	u, err := antigravity.Parse([]byte(sampleUsage))
	if err != nil {
		t.Fatal(err)
	}
	if u.Gemini != 99 || u.Claude != 0 {
		t.Errorf("got gemini=%d claude=%d, want 99/0", u.Gemini, u.Claude)
	}
}

func TestParseUsageErrors(t *testing.T) {
	if _, err := antigravity.Parse([]byte("Authentication required. Please visit the URL")); err == nil {
		t.Error("want not-signed-in error")
	}
	if _, err := antigravity.Parse([]byte("garbage")); err == nil {
		t.Error("want parse error")
	}
	if _, err := antigravity.Parse([]byte(`{"command":{"data":{"groups":[]}}}`)); err == nil {
		t.Error("want no-data error")
	}
}

func TestUsageCacheRoundTripAndDelete(t *testing.T) {
	dir := t.TempDir()
	c := antigravity.LoadCache(dir)
	c.Set("work", antigravity.Usage{Gemini: 11, Claude: 77})
	c.Save()

	got := antigravity.LoadCache(dir).Entries["work"]
	if got.Usage.Gemini != 11 || got.Usage.Claude != 77 || got.At.IsZero() {
		t.Fatalf("unexpected cache entry: %+v", got)
	}

	c.Delete("work")
	c.Save()
	if _, ok := antigravity.LoadCache(dir).Entries["work"]; ok {
		t.Error("entry should be deleted")
	}
}

func TestUsageCacheCorruptFileIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "usage-cache.json"), []byte("{nope"), 0600); err != nil {
		t.Fatal(err)
	}
	if n := len(antigravity.LoadCache(dir).Entries); n != 0 {
		t.Errorf("corrupt cache should load empty, got %d entries", n)
	}
}

func TestReadEmail(t *testing.T) {
	home := t.TempDir()
	write := func(content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(home, ".gemini"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".gemini", "google_accounts.json"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	if got := antigravity.ReadEmail(home); got != "" {
		t.Errorf("missing file: got %q", got)
	}
	write(`{"active":"me@example.com","old":["x@y.z"]}`)
	if got := antigravity.ReadEmail(home); got != "me@example.com" {
		t.Errorf("got %q", got)
	}
	for _, bad := range []string{`{"active":"no-at-sign"}`, "{nope", `{"active":"a@b.c\u001b[31m"}`, `{}`} {
		write(bad)
		if got := antigravity.ReadEmail(home); got != "" {
			t.Errorf("%s: want empty, got %q", bad, got)
		}
	}
}

func TestParseUsageNotSignedInVariants(t *testing.T) {
	for _, out := range []string{
		"Error: authentication required. Run 'agy' to log in, then retry.\n",
		`{"status":"ERROR","error":"authentication failed or timed out"}`,
	} {
		if _, err := antigravity.Parse([]byte(out)); err == nil {
			t.Errorf("want not-signed-in error for %q", out)
		}
	}
}

func TestReadEmailFromLogs(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".gemini", "antigravity-cli", "log")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	logf := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	logf("cli-20261002_005645.log", "I1002 00:56:45 server_oauth.go:203] OAuth: authenticated successfully as old@example.com\n")
	logf("cli-20261002_005726.log", "I1002 x applyAuthResult: email=new.person@example.com, authMethod=consumer, quotaProject=\nnoise\n")

	if got := antigravity.ReadEmail(home); got != "new.person@example.com" {
		t.Errorf("got %q, want the newest log's account", got)
	}

	// The account index takes priority over logs.
	if err := os.WriteFile(filepath.Join(home, ".gemini", "google_accounts.json"), []byte(`{"active":"idx@example.com"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := antigravity.ReadEmail(home); got != "idx@example.com" {
		t.Errorf("got %q, want index email", got)
	}
}
