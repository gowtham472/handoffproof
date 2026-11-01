// Package score computes the deterministic Handover Score (F6):
// how ready the app is to survive its developer leaving, 0-100.
package score

import (
	"encoding/json"
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
	// Faults: the planted-fault catalogue is read from the ground-truth
	// file, never hardcoded here. Each fault counts as caught when its
	// kind has matching drill evidence or a static-scan signal. Faults
	// whose kind this scorer cannot observe are excluded from the
	// denominator instead of silently scoring zero or five.
	rep, _ := scan.Scan(filepath.Join(root, "demo-app"))
	if planted, err := loadPlanted(root); err == nil && len(planted) > 0 {
		caught, observable := 0, 0
		for _, f := range planted {
			ok, known := faultCaught(f.Kind, fails, rep, root)
			if !known {
				continue
			}
			observable++
			if ok {
				caught++
			}
		}
		if observable > 0 {
			comp["faults"] = caught * 100 / observable
		}
	}
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
	// Backup: covered share of detected data stores. When the scan
	// detects no data stores the component is unmeasured and omitted,
	// never defaulted to a perfect 100.
	stores := len(rep.DataStores)
	if stores > 0 {
		comp["backup"] = (stores - len(rep.BackupGaps)) * 100 / stores
	}
	return Result{
		Total:      weightedTotal(comp),
		Components: comp,
		Basis:      "evidence records + ground truth + static scan (deterministic)",
	}
}

var weights = map[string]float64{"workflows": 0.40, "faults": 0.30, "documentation": 0.20, "backup": 0.10}

// weightedTotal averages the measured components by their weights.
// Unmeasured components are excluded rather than scored as 0 or 100.
func weightedTotal(comp map[string]int) int {
	sum, wsum := 0.0, 0.0
	for name, w := range weights {
		if v, ok := comp[name]; ok {
			sum += w * float64(v)
			wsum += w
		}
	}
	if wsum == 0 {
		return 0
	}
	return int(math.Round(sum / wsum))
}

type plantedFault struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

func loadPlanted(root string) ([]plantedFault, error) {
	raw, err := os.ReadFile(filepath.Join(root, "eval", "ground_truth.json"))
	if err != nil {
		return nil, err
	}
	var doc struct {
		Planted []plantedFault `json:"planted_faults"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc.Planted, nil
}

// faultCaught reports whether a fault kind is caught, and whether this
// scorer can observe that kind at all.
func faultCaught(kind string, fails map[string]bool, rep scan.Report, root string) (caught, observable bool) {
	switch kind {
	case "missing_configuration":
		return fails["missing_configuration/reminder_job"] || len(rep.UndocumentedEnvVars) > 0, true
	case "silent_failure":
		return fails["withdrawn_test_token/confirmation_email"], true
	case "incomplete_backup":
		return fails["incomplete_backup/documents"] || len(rep.BackupGaps) > 0, true
	case "documentation_gap":
		return len(rep.AskPhrases) > 0, true
	case "prompt_injection":
		return securityEventSeen(root), true
	}
	return false, false
}

func securityEventSeen(root string) bool {
	re := strings.NewReplacer("\x00", "")
	for _, pat := range []string{"evidence/*.err", "evidence/*.log"} {
		ms, _ := filepath.Glob(filepath.Join(root, pat))
		for _, m := range ms {
			if b, err := os.ReadFile(m); err == nil && strings.Contains(re.Replace(string(b)), "security event: prompt_injection_attempt at ") {
				return true
			}
		}
	}
	return false
}
