// Package score computes the deterministic Handover Score (F6):
// how ready the app is to survive its developer leaving, 0-100.
package score

import (
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/gowtham472/handoffproof/internal/controller"
	"github.com/gowtham472/handoffproof/internal/scan"
)

// Result is one scored snapshot.
type Result struct {
	Total      int            `json:"total"`
	Components map[string]int `json:"components"`
	Basis      string         `json:"basis"`
}

// Combine weights component scores: workflows 40, faults 30, docs 20, backup 10.
func Combine(workflows, faults, docs, backup int) int {
	return int(math.Round(0.40*float64(workflows) + 0.30*float64(faults) + 0.20*float64(docs) + 0.10*float64(backup)))
}

// Compute scores the repository at root from evidence, ground truth and scan.
func Compute(root string) Result {
	comp := map[string]int{}
	// Workflows: observed pass ratio over all controller records.
	matches, _ := filepath.Glob(filepath.Join(root, "evidence", "run-*.json"))
	passed, total := 0, 0
	fails := map[string]bool{}
	for _, m := range matches {
		recs, err := controller.ReadEvidence(m)
		if err != nil {
			continue
		}
		for _, r := range recs {
			total++
			if r.Passed {
				passed++
			} else {
				fails[r.Drill+"/"+r.Workflow] = true
			}
		}
	}
	if total > 0 {
		comp["workflows"] = passed * 100 / total
	}
	// Faults: planted faults with matching observed failure / static catch.
	rep, _ := scan.Scan(filepath.Join(root, "demo-app"))
	caught := 0
	if fails["missing_configuration/reminder_job"] {
		caught++
	}
	if fails["withdrawn_test_token/confirmation_email"] {
		caught++
	}
	if fails["incomplete_backup/documents"] {
		caught++
	}
	for _, f := range rep.AskPhrases {
		if f.Value == "ask Ravi" {
			caught++
		}
	}
	for _, pat := range []string{"evidence/*.err", "evidence/*.log"} {
		ms, _ := filepath.Glob(filepath.Join(root, pat))
		for _, m := range ms {
			if b, err := os.ReadFile(m); err == nil && strings.Contains(strings.ReplaceAll(string(b), "\x00", ""), "prompt_injection_attempt") {
				caught++
			}
		}
	}
	if caught > 5 {
		caught = 5
	}
	comp["faults"] = caught * 100 / 5
	// Documentation: documented share of env vars read in code.
	if len(rep.EnvVars) > 0 {
		doc := 0
		for _, ev := range rep.EnvVars {
			if ev.Documented {
				doc++
			}
		}
		comp["documentation"] = doc * 100 / len(rep.EnvVars)
	}
	// Backup: covered share of detected data stores.
	stores := len(rep.DataStores)
	if stores > 0 {
		comp["backup"] = (stores - len(rep.BackupGaps)) * 100 / stores
	} else {
		comp["backup"] = 100
	}
	return Result{
		Total:      Combine(comp["workflows"], comp["faults"], comp["documentation"], comp["backup"]),
		Components: comp,
		Basis:      "evidence records + ground truth + static scan (deterministic)",
	}
}
