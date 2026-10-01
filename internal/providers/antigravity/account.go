package antigravity

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// accountsFile is the account index agy keeps in its data directory. It
// holds the active account's email address and no tokens.
const accountsFile = ".gemini/google_accounts.json"

// logDir holds agy's CLI logs. When agy keeps its login in the OS keychain it
// does not write the account index, but it logs who signed in.
const logDir = ".gemini/antigravity-cli/log"

// maxLogBytes bounds how much of one log file is read.
const maxLogBytes = 1 << 20

// ReadEmail returns the email of the Google account agy is signed into under
// the given home directory, or "" if it cannot be determined. It checks, in
// order, the "active" field of agy's account index and the sign-in line in
// agy's most recent logs. Credential files and the keychain are never read.
func ReadEmail(home string) string {
	if email := emailFromAccountIndex(home); email != "" {
		return email
	}
	return emailFromLogs(home)
}

func emailFromAccountIndex(home string) string {
	data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(accountsFile)))
	if err != nil {
		return ""
	}
	var f struct {
		Active string `json:"active"`
	}
	if json.Unmarshal(data, &f) != nil {
		return ""
	}
	email := strings.TrimSpace(f.Active)
	if !plausibleEmail(email) {
		return ""
	}
	return email
}

// plausibleEmail rejects values that are not safe to print in a terminal.
func plausibleEmail(s string) bool {
	if len(s) < 3 || len(s) > 254 || strings.Count(s, "@") != 1 {
		return false
	}
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

// signInLine matches the line agy logs after a successful sign-in, e.g.
// "OAuth: authenticated successfully as <email>" or "applyAuthResult: email=<email>".
var signInLine = regexp.MustCompile(`(?:authenticated successfully as |applyAuthResult: email=)([^\s,]+)`)

// emailFromLogs returns the account named by the latest sign-in line in the
// newest log file that has one. Only that single value is extracted.
func emailFromLogs(home string) string {
	dir := filepath.Join(home, filepath.FromSlash(logDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names))) // names embed a timestamp
	if len(names) > 5 {
		names = names[:5]
	}
	for _, name := range names {
		if email := lastSignIn(filepath.Join(dir, name)); email != "" {
			return email
		}
	}
	return ""
}

func lastSignIn(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > maxLogBytes {
		_, _ = f.Seek(fi.Size()-maxLogBytes, io.SeekStart)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxLogBytes))
	if err != nil {
		return ""
	}
	matches := signInLine.FindAllSubmatch(data, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		if email := string(matches[i][1]); plausibleEmail(email) {
			return email
		}
	}
	return ""
}
