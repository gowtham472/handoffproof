package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gowtham472/handoffproof/internal/secretpath"
)

// Citation verdicts for one finding (F2 adversarial verifier).
type Citation struct {
	FindingID string `json:"finding_id"`
	Status    string `json:"citation_status"` // verified | missing | invalid
	Evidence  string `json:"evidence_status"` // ok | missing_ids
	Detail    string `json:"detail"`
}

type diagFinding struct {
	ID        string   `json:"finding_id"`
	Class     string   `json:"classification"`
	Evidence  []string `json:"evidence_ids"`
	Sources   []string `json:"source_references"`
	RootCause string   `json:"root_cause"`
}

var refRe = regexp.MustCompile(`^(.+):(\d+)(?:-(\d+))?$`)

// CheckCitations deterministically verifies that every finding cites real
// file:line references inside the workspace and that observed findings cite
// controller evidence. When validEvidence is non-nil, every cited
// evidence_id must resolve to a real controller record - fabricated IDs
// fail. No model call: this is the adversarial floor.
func CheckCitations(workspace string, diagnosis json.RawMessage, validEvidence map[string]bool) ([]Citation, error) {
	var d struct {
		Findings []diagFinding `json:"findings"`
	}
	if err := json.Unmarshal(diagnosis, &d); err != nil {
		return nil, fmt.Errorf("parse diagnosis: %w", err)
	}
	var out []Citation
	for _, f := range d.Findings {
		c := Citation{FindingID: f.ID, Status: "verified", Evidence: "ok"}
		if (f.Class == "observed_failure" || f.Class == "observed_pass") && len(f.Evidence) == 0 {
			c.Evidence = "missing_ids"
			c.Detail = "observed finding without evidence_id"
		}
		if validEvidence != nil {
			for _, id := range f.Evidence {
				if !validEvidence[id] {
					c.Evidence = "unknown_ids"
					c.Detail = "cited evidence_id does not exist: " + id
					break
				}
			}
		}
		if len(f.Sources) == 0 {
			c.Status = "missing"
			c.Detail = "no file:line citation; root cause is unanchored"
			out = append(out, c)
			continue
		}
		for _, ref := range f.Sources {
			m := refRe.FindStringSubmatch(strings.TrimSpace(ref))
			if m == nil {
				c.Status = "invalid"
				c.Detail = "unparseable reference " + ref
				break
			}
			full := filepath.Join(workspace, filepath.Clean(m[1]))
			if rel, rerr := filepath.Rel(workspace, full); rerr != nil || strings.HasPrefix(rel, "..") {
				c.Status = "invalid"
				c.Detail = "citation escapes workspace: " + m[1]
				break
			}
			if secretpath.Denied(m[1]) {
				c.Status = "invalid"
				c.Detail = "citation points at credential material: " + m[1]
				break
			}
			if info, lerr := os.Lstat(full); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
				c.Status = "invalid"
				c.Detail = "citation points at a symlink: " + m[1]
				break
			}
			raw, err := os.ReadFile(full)
			if err != nil {
				c.Status = "invalid"
				c.Detail = "cited file not found: " + m[1]
				break
			}
			lines := strings.Count(string(raw), "\n") + 1
			start := atoi(m[2])
			end := start
			if m[3] != "" {
				end = atoi(m[3])
			}
			if start < 1 || end > lines || end < start {
				c.Status = "invalid"
				c.Detail = fmt.Sprintf("line range %d-%d outside %s (%d lines)", start, end, m[1], lines)
				break
			}
		}
		if c.Status == "verified" && c.Detail == "" {
			c.Detail = "all citations resolve to real lines"
		}
		out = append(out, c)
	}
	return out, nil
}

func atoi(s string) int {
	n := 0
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int(ch-'0')
		}
	}
	return n
}
