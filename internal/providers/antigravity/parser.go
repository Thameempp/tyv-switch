package antigravity

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

var errNotSignedIn = errors.New("not signed in (run: tyv <profile>)")

type usageResponse struct {
	Status  string `json:"status"`
	Command struct {
		Data struct {
			Groups []struct {
				Name    string `json:"name"`
				Buckets []struct {
					RemainingFraction float64 `json:"remaining_fraction"`
					ResetTime         string  `json:"reset_time"`
				} `json:"buckets"`
			} `json:"groups"`
		} `json:"data"`
	} `json:"command"`
}

// Parse converts agy's /usage JSON into per-family remaining percentages and
// reset times. Each family has a 5-hour and a weekly bucket; the lower of the
// two is the limit that actually applies, so that bucket's percentage and
// reset time are reported (on a tie, the sooner reset).
func Parse(out []byte) (Usage, error) {
	u := Usage{Gemini: Unknown, Claude: Unknown}

	if notSignedIn(out) {
		return u, errNotSignedIn
	}
	var resp usageResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return u, errors.New("unrecognised /usage output")
	}

	for _, g := range resp.Command.Data.Groups {
		if len(g.Buckets) == 0 {
			continue
		}
		min := 2.0
		var reset time.Time
		for _, b := range g.Buckets {
			t, _ := time.Parse(time.RFC3339, b.ResetTime)
			sooner := !t.IsZero() && (reset.IsZero() || t.Before(reset))
			if b.RemainingFraction < min || (b.RemainingFraction == min && sooner) {
				min, reset = b.RemainingFraction, t
			}
		}
		pct := int(math.Round(math.Max(0, math.Min(1, min)) * 100))

		name := strings.ToLower(g.Name)
		switch {
		case strings.Contains(name, "gemini"):
			u.Gemini, u.GeminiReset = pct, reset
		case strings.Contains(name, "claude"):
			u.Claude, u.ClaudeReset = pct, reset
		}
	}
	if u.Gemini == Unknown && u.Claude == Unknown {
		return u, errors.New("no quota data in /usage output")
	}
	return u, nil
}
