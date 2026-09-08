package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGastownSessionEvidence(t *testing.T) {
	healthy := `{"agents":[{"session":"hq-mayor","role":"mayor","running":true}],"rigs":[{"agents":[{"session":"ah-witness","role":"witness","running":true},{"session":"ah-refinery","role":"refinery","running":false}]}]}`
	const clean = "✓ zombie-sessions All 2 Gas Town sessions have running Claude processes"
	for _, tc := range []struct {
		name, status          string
		sessions              []string
		startup, doctor, want string
	}{
		{"present agents healthy; idle refinery and absent boot allowed", healthy, []string{"hq-mayor", "ah-witness"}, clean, clean, ""},
		{"false zombie in status", `{"agents":[{"session":"hq-mayor","role":"mayor","running":false}]}`, []string{"hq-mayor"}, clean, clean, "hq-mayor"},
		{"startup cleanup must not hide zombies", healthy, []string{"hq-mayor"}, "zombie-sessions Found 5 zombie session(s) (fixing)... No zombie sessions found (fixed)", clean, "startup"},
		{"current doctor detects zombies", healthy, []string{"hq-mayor"}, clean, "zombie-sessions Found 1 zombie session(s)", "doctor"},
		{"no agents is not success", healthy, []string{"dashboard"}, clean, clean, "no persistent"},
		{"missing doctor check", healthy, []string{"hq-mayor"}, clean, "", "doctor"},
		{"invalid status", "invalid", []string{"hq-mayor"}, clean, clean, "status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGastownSessionEvidence([]byte(tc.status), tc.sessions, tc.startup, tc.doctor)
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func validateGastownSessionEvidence(status []byte, sessions []string, startup, doctor string) error {
	for _, check := range []struct{ name, output string }{{"startup", startup}, {"doctor", doctor}} {
		healthy := strings.Contains(check.output, "No zombie sessions found") ||
			regexp.MustCompile(`All [1-9][0-9]* Gas Town sessions have running Claude processes`).MatchString(check.output)
		if regexp.MustCompile(`Found [1-9][0-9]* zombie session`).MatchString(check.output) ||
			!healthy {
			return fmt.Errorf("%s zombie check did not pass: %s", check.name, check.output)
		}
	}
	type agent struct {
		Session string
		Role    string
		Running bool
	}
	var town struct {
		Agents []agent
		Rigs   []struct{ Agents []agent }
	}
	if err := json.Unmarshal(status, &town); err != nil {
		return fmt.Errorf("invalid gt status: %w", err)
	}
	agents := append([]agent(nil), town.Agents...)
	for _, rig := range town.Rigs {
		agents = append(agents, rig.Agents...)
	}
	present := make(map[string]bool)
	for _, session := range sessions {
		present[session] = true
	}
	checked := 0
	for _, a := range agents {
		// Boot and dogs can finish normally between the two observations.
		if !present[a.Session] || a.Role == "boot" || a.Role == "dog" {
			continue
		}
		checked++
		if !a.Running {
			return fmt.Errorf("present session %s is not recognized as running by Gastown", a.Session)
		}
	}
	if checked == 0 {
		return fmt.Errorf("no persistent agent session was checked")
	}
	return nil
}
