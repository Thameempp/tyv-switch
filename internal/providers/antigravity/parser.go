package antigravity

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
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
				} `json:"buckets"`
			} `json:"groups"`
		} `json:"data"`
	} `json:"command"`
}

// Parse converts agy's /usage JSON into per-family remaining percentages.
// Each family has a 5-hour and a weekly bucket; the lower of the two is the
// limit that actually applies, so that is what is reported.
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
		min := 1.0
		for _, b := range g.Buckets {
			if b.RemainingFraction < min {
				min = b.RemainingFraction
			}
		}
		pct := int(math.Round(math.Max(0, math.Min(1, min)) * 100))

		name := strings.ToLower(g.Name)
		switch {
		case strings.Contains(name, "gemini"):
			u.Gemini = pct
		case strings.Contains(name, "claude"):
			u.Claude = pct
		}
	}
	if u.Gemini == Unknown && u.Claude == Unknown {
		return u, errors.New("no quota data in /usage output")
	}
	return u, nil
}
