package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/controller"
	"github.com/gowtham472/handoffproof/internal/scan"
)

// cmdEval scores detection arms against ground truth and writes
// docs/RESULTS.md (hp-eval-results-table). Measured numbers only.
func cmdEval(root string, args []string) error {
	demoScan, err := scan.Scan(filepath.Join(root, "demo-app"))
	if err != nil {
		return err
	}
	heldScan, err := scan.Scan(filepath.Join(root, "eval", "fixtures", "heldout-app"))
	if err != nil {
		return err
	}
	recs := recordsFrom(root)
	demoTruth, err := loadTruth(filepath.Join(root, "eval", "ground_truth.json"))
	if err != nil {
		return err
	}
	heldTruth, err := loadTruth(filepath.Join(root, "eval", "heldout_truth.json"))
	if err != nil {
		return err
	}
	type arm struct {
		name    string
		app     string
		inject  int
		caught  []string
		missed  []string
		falseAl int
	}
	split := func(truth []truthFault, caught func(truthFault) bool) arm {
		a := arm{inject: len(truth)}
		for _, f := range truth {
			if caught(f) {
				a.caught = append(a.caught, f.ID)
			} else {
				a.missed = append(a.missed, f.ID)
			}
		}
		sort.Strings(a.caught)
		sort.Strings(a.missed)
		return a
	}
	// Demo app, static arm: every caught/missed verdict is computed from
	// the scan output for that fault kind, never listed by hand.
	demoStatic := split(demoTruth, func(f truthFault) bool { return staticSignal(demoScan, f.Kind) })
	demoStatic.name, demoStatic.app = "static_scan_only", "demo-app"
	demoStatic.falseAl = countFalseAlarms(demoScan, map[string]bool{"SCHEDULER_TOKEN": true, "ask Ravi": true})
	// Demo app, drills arm: static scan plus controller drill evidence,
	// with no model involved. The prompt-injection fault needs the agent's
	// security event, so this arm cannot catch it.
	drills := split(demoTruth, func(f truthFault) bool {
		return staticSignal(demoScan, f.Kind) || drillSignal(recs, f.Kind)
	})
	drills.name, drills.app = "scan + drills, no model", "demo-app"
	// Demo app, full arm: the drills arm plus the agent security event
	// recorded when the model read the injected comment.
	events := parseSecurityEvents(root)
	full := split(demoTruth, func(f truthFault) bool {
		if staticSignal(demoScan, f.Kind) || drillSignal(recs, f.Kind) {
			return true
		}
		return f.Kind == "prompt_injection" && hasSecurityEvent(events, "prompt_injection_attempt")
	})
	full.name, full.app = "full HandoffProof", "demo-app"
	// Second fixture (author-written), static arm: each expected_static
	// signal in the truth file is checked against the scan output.
	held := split(heldTruth, func(f truthFault) bool { return expectedStaticSignal(heldScan, f.ExpectedStatic) })
	held.name, held.app = "static_scan_only (second fixture (author-written))", "heldout-app"
	held.falseAl = countFalseAlarms(heldScan, map[string]bool{"API_KEY": true, "ask Sam": true})

	commit := gitShort(root)
	tokens := liveTokens(root)
	var b strings.Builder
	b.WriteString("# Results\n\n")
	b.WriteString("Measured on the demo app and a second fixture (author-written). Misses are reported plainly; anything not run is labelled a target.\n\n")
	fmt.Fprintf(&b, "Date: %s · Commit: %s · Model: %s\n\n", time.Now().Format("2006-01-02"), commit, os.Getenv("META_MODEL_ID"))
	b.WriteString("| Arm | App | Injected | Caught | Missed | False alarms | Tokens/run |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	joinOr := func(xs []string) string {
		if len(xs) == 0 {
			return "-"
		}
		return strings.Join(xs, ", ")
	}
	for _, a := range []arm{demoStatic, drills, full, held} {
		tok := "0 (deterministic)"
		if a.name == "full HandoffProof" {
			tok = tokens
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d (%s) | %d (%s) | %d | %s |\n",
			a.name, a.app, a.inject, len(a.caught), joinOr(a.caught), len(a.missed), joinOr(a.missed), a.falseAl, tok)
	}
	b.WriteString("\nEvidence behind each row: `evidence/run-*.json`, `evidence/diagnosis-*.json`, `evidence/verification.json`.\n")
	b.WriteString("Outsider fault slot: add `eval/outsider_fault.json` (kind + target) and rerun `handoffproof eval`; until then the outsider arm is a **target**, not a result.\n")
	out := filepath.Join(root, "docs", "RESULTS.md")
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Println("results:", out)
	fmt.Print(b.String())
	return nil
}

func recordsFrom(root string) []struct {
	Drill    string
	Workflow string
	Passed   bool
} {
	matches, _ := filepath.Glob(filepath.Join(root, "evidence", "run-*.json"))
	var out []struct {
		Drill    string
		Workflow string
		Passed   bool
	}
	for _, m := range matches {
		recs, err := controller.ReadEvidence(m)
		if err != nil {
			continue
		}
		for _, r := range recs {
			out = append(out, struct {
				Drill    string
				Workflow string
				Passed   bool
			}{r.Drill, r.Workflow, r.Passed})
		}
	}
	return out
}

func hasFail(recs []struct {
	Drill    string
	Workflow string
	Passed   bool
}, drill, workflow string) bool {
	for _, r := range recs {
		if r.Drill == drill && r.Workflow == workflow && !r.Passed {
			return true
		}
	}
	return false
}

func hasAsk(rep scan.Report, want string) bool {
	for _, f := range rep.AskPhrases {
		if f.Value == want {
			return true
		}
	}
	return false
}

func demoScanNames(rep scan.Report) []string { return rep.UndocumentedEnvVars }

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func countFalseAlarms(rep scan.Report, allowed map[string]bool) int {
	n := 0
	for _, v := range rep.UndocumentedEnvVars {
		if !allowed[v] {
			n++
		}
	}
	for _, f := range rep.AskPhrases {
		if !allowed[f.Value] {
			n++
		}
	}
	return n
}

// truthFault is one planted fault from an eval truth file.
type truthFault struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	ExpectedStatic string `json:"expected_static"`
}

func loadTruth(path string) ([]truthFault, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Planted []truthFault `json:"planted_faults"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(doc.Planted) == 0 {
		return nil, fmt.Errorf("no planted faults in %s", path)
	}
	return doc.Planted, nil
}

// staticSignal reports whether the static scan alone surfaces a fault of
// this kind. Kinds with no static signature honestly return false.
func staticSignal(rep scan.Report, kind string) bool {
	switch kind {
	case "missing_configuration":
		return len(rep.UndocumentedEnvVars) > 0
	case "incomplete_backup":
		return len(rep.BackupGaps) > 0
	case "documentation_gap":
		return len(rep.AskPhrases) > 0
	}
	return false
}

// drillSignal reports whether controller drill evidence (no model) shows
// the workflow this fault kind breaks actually failing.
func drillSignal(recs []struct {
	Drill    string
	Workflow string
	Passed   bool
}, kind string) bool {
	switch kind {
	case "missing_configuration":
		return hasFail(recs, "missing_configuration", "reminder_job")
	case "silent_failure":
		return hasFail(recs, "withdrawn_test_token", "confirmation_email")
	case "incomplete_backup":
		return hasFail(recs, "incomplete_backup", "documents")
	}
	return false
}

// expectedStaticSignal checks one truth-file signal such as
// "undocumented_env:API_KEY" against the scan output.
func expectedStaticSignal(rep scan.Report, expected string) bool {
	kind, value, ok := strings.Cut(expected, ":")
	if !ok {
		return false
	}
	switch kind {
	case "undocumented_env":
		return contains(rep.UndocumentedEnvVars, value)
	case "backup_gap":
		return contains(rep.BackupGaps, value)
	case "ask_phrase":
		return hasAsk(rep, value)
	}
	return false
}

// securityEvent is one structured agent security event parsed from a run
// log line of the form "security event: <kind> at <path> (...)".
type securityEvent struct {
	Kind string
	Path string
}

var securityEventRe = regexp.MustCompile(`security event: ([a-z0-9_]+) at (\S+)`)

func parseSecurityEvents(root string) []securityEvent {
	var out []securityEvent
	for _, pat := range []string{"evidence/*.err", "evidence/*.log"} {
		matches, _ := filepath.Glob(filepath.Join(root, pat))
		for _, m := range matches {
			b, err := os.ReadFile(m)
			if err != nil {
				continue
			}
			// PowerShell redirection writes UTF-16; strip NULs first.
			text := strings.ReplaceAll(string(b), "\x00", "")
			for _, line := range strings.Split(text, "\n") {
				if mm := securityEventRe.FindStringSubmatch(line); mm != nil {
					out = append(out, securityEvent{Kind: mm[1], Path: mm[2]})
				}
			}
		}
	}
	return out
}

func hasSecurityEvent(events []securityEvent, kind string) bool {
	for _, e := range events {
		if e.Kind == kind && e.Path != "" {
			return true
		}
	}
	return false
}

var tokenRe = regexp.MustCompile(`total=(\d+)`)

func liveTokens(root string) string {
	total := 0
	found := false
	for _, name := range []string{"evidence/live-scan.log", "evidence/live-full.log"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		for _, m := range tokenRe.FindAllStringSubmatch(strings.ReplaceAll(string(b), "\x00", ""), -1) {
			var n int
			fmt.Sscanf(m[1], "%d", &n)
			total += n
			found = true
		}
	}
	if !found {
		return "target (no live run logged)"
	}
	return fmt.Sprintf("%d (scan + one full loop, logged)", total)
}

func gitShort(root string) string {
	cmd := exec.Command("git", "-c", "safe.directory="+filepath.ToSlash(root), "rev-parse", "--short", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
