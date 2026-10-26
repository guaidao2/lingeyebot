package mode

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/guaidao2/lingeyebot/internal/store"
)

var (
	coverageFirst = regexp.MustCompile(`(?i)(?:覆盖率|coverage)\D{0,12}(\d{1,3})\s*%`)
	coverageLast  = regexp.MustCompile(`(?i)(\d{1,3})\s*%\s*(?:的)?(?:覆盖率|coverage|覆盖)`)
	flagCountRe   = regexp.MustCompile(`(?i)(\d+)\s*个?\s*flags?`)
	flagTokenRe   = regexp.MustCompile(`(?i)\b(?:flag|ctf|key|hkctf|nssctf)\{[^}\r\n]{1,200}\}`)
)

// ValidateGoal refuses a premature "met" when the goal has a measurable
// acceptance condition that the current state does not satisfy.
func (d Definition) ValidateGoal(st *store.Store, run store.Run, snap store.Snapshot, cov store.Coverage, g store.Goal) error {
	text := g.Text + " " + g.Acceptance

	if want, ok := coverageTarget(text); ok {
		got := 0
		if cov.Total > 0 {
			got = cov.Tested * 100 / cov.Total
		}
		if got < want {
			return fmt.Errorf("验收未达成：覆盖率 %d%% < %d%%（已测 %d/%d 资产），继续补测而不是提前收工", got, want, cov.Tested, cov.Total)
		}
	}

	if d.ID != "ctf" {
		return nil
	}
	wantFlags, hasFlagPhrase := flagTarget(text)
	if hasFlagPhrase {
		got := countFlags(st, snap)
		if wantFlags <= 1 {
			wantFlags = 1
		}
		if got < wantFlags {
			return fmt.Errorf("验收未达成：只找到 %d 个 flag，要求 %d 个；flag 必须来自真实证据", got, wantFlags)
		}
	}
	return nil
}

func coverageTarget(text string) (int, bool) {
	if m := coverageFirst.FindStringSubmatch(text); m != nil {
		v, err := strconv.Atoi(m[1])
		if err == nil && v > 0 && v <= 100 {
			return v, true
		}
	}
	if m := coverageLast.FindStringSubmatch(text); m != nil {
		v, err := strconv.Atoi(m[1])
		if err == nil && v > 0 && v <= 100 {
			return v, true
		}
	}
	return 0, false
}

func flagTarget(text string) (int, bool) {
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "flag") {
		if m := flagCountRe.FindStringSubmatch(text); m == nil {
			return 0, false
		}
	}
	if m := flagCountRe.FindStringSubmatch(text); m != nil {
		v, err := strconv.Atoi(m[1])
		if err == nil && v > 0 {
			return v, true
		}
	}
	return 1, true
}

func countFlags(st *store.Store, snap store.Snapshot) int {
	seen := map[string]bool{}
	add := func(s string) {
		for _, m := range flagTokenRe.FindAllString(s, -1) {
			seen[strings.ToLower(strings.TrimSpace(m))] = true
		}
	}
	for _, n := range snap.Nodes {
		add(string(n.Payload))
	}
	for _, e := range snap.Evidence {
		if body, err := st.ReadEvidence(e.ID); err == nil {
			add(string(body))
		}
	}
	return len(seen)
}

// Metadata describes a mode to the browser.
type Metadata struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Tagline     string   `json:"tagline"`
	Roles       []string `json:"roles"`
	ToolCount   int      `json:"toolCount"`
	Description string   `json:"description"`
}

// MetadataList returns browser-facing metadata for every mode.
func MetadataList() []Metadata {
	out := make([]Metadata, 0, len(definitions))
	for _, d := range definitions {
		desc := d.Prompt
		if idx := strings.Index(desc, "\n"); idx > 0 {
			desc = desc[:idx]
		}
		out = append(out, Metadata{ID: d.ID, Name: d.Name, Tagline: d.Tagline, Roles: d.Roles, ToolCount: len(d.Tools), Description: desc})
	}
	return out
}
