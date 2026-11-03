package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/controller"
	"github.com/gowtham472/handoffproof/internal/scan"
)

// drillRec is one controller record reduced to the fields arm scoring needs.
type drillRec struct {
	Drill    string
	Workflow string
	Passed   bool
}

// cmdEval scores detection arms against ground truth and writes
// docs/RESULTS.md (hp-eval-results-table). Measured numbers only.
func cmdEval(ctx context.Context, root string, args []string) error {
	evDir := filepath.Join(root, "evidence")
	var models []string
	reps := 3
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--evidence-dir":
			if i+1 < len(args) {
				evDir = args[i+1]
				i++
			}
		case "--models":
			if i+1 < len(args) {
				for _, m := range strings.Split(args[i+1], ",") {
					if m = strings.TrimSpace(m); m != "" {
						models = append(models, m)
					}
				}
				i++
			}
		case "--reps":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil && n >= 1 {
					reps = n
				}
				i++
			}
		}
	}
	demoScan, err := scan.Scan(filepath.Join(root, "demo-app"))
	if err != nil {
		return err
	}
	heldScan, err := scan.Scan(filepath.Join(root, "eval", "fixtures", "heldout-app"))
	if err != nil {
		return err
	}
	recs := recordsFromDir(evDir)
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
	events := parseSecurityEvents(evDir)
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
	held.name, held.app = "static_scan_only (second fixture, author-written)", "heldout-app"
	held.falseAl = countFalseAlarms(heldScan, map[string]bool{"API_KEY": true, "ask Sam": true})

	commit := gitShort(root)
	tokens := liveTokens(evDir)
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
	b.WriteString(modelQualitySection(evDir, demoTruth))
	if len(models) > 0 {
		b.WriteString(runModelBenchmark(ctx, root, evDir, models, reps))
	}
	out := filepath.Join(root, "docs", "RESULTS.md")
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Println("results:", out)
	fmt.Print(b.String())
	return nil
}

func recordsFromDir(evDir string) []drillRec {
	matches, _ := filepath.Glob(filepath.Join(evDir, "run-*.json"))
	var out []drillRec
	for _, m := range matches {
		recs, err := controller.ReadEvidence(m)
		if err != nil {
			continue
		}
		for _, r := range recs {
			out = append(out, drillRec{r.Drill, r.Workflow, r.Passed})
		}
	}
	return out
}

func hasFail(recs []drillRec, drill, workflow string) bool {
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
	Description    string `json:"description"`
	ExpectedStatic string `json:"expected_static"`
	Evidence       []struct {
		Path string `json:"path"`
	} `json:"evidence"`
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
func drillSignal(recs []drillRec, kind string) bool {
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

func parseSecurityEvents(evDir string) []securityEvent {
	var out []securityEvent
	for _, pat := range []string{"*.err", "*.log"} {
		matches, _ := filepath.Glob(filepath.Join(evDir, pat))
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

func liveTokens(evDir string) string {
	total := 0
	found := false
	for _, name := range []string{"live-scan.log", "live-full.log"} {
		b, err := os.ReadFile(filepath.Join(evDir, name))
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

// ---- Model quality: computed only from existing artifacts in evDir. ----

func runLabel(path string) string {
	if info, err := os.Stat(path); err == nil {
		return filepath.Base(path) + " (modified " + info.ModTime().Format("2006-01-02") + ")"
	}
	return filepath.Base(path)
}

type citationCounts struct {
	Total, Verified, Invalid, Missing, UnknownIDs int
	Found                                         bool
	Label                                         string
}

func countCitations(evDir string) citationCounts {
	path := filepath.Join(evDir, "verification.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return citationCounts{Label: "verification.json"}
	}
	var doc struct {
		Verdicts []struct {
			CitationOK bool     `json:"citation_ok"`
			EvidenceOK bool     `json:"evidence_ok"`
			Issues     []string `json:"issues"`
		} `json:"verdicts"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return citationCounts{Label: runLabel(path)}
	}
	c := citationCounts{Found: true, Label: runLabel(path)}
	for _, v := range doc.Verdicts {
		c.Total++
		switch {
		case v.CitationOK && v.EvidenceOK:
			c.Verified++
		case !v.CitationOK:
			c.Invalid++
		default:
			c.UnknownIDs++
		}
		for _, issue := range v.Issues {
			if strings.Contains(strings.ToLower(issue), "missing") {
				c.Missing++
			}
		}
	}
	return c
}

type mapDependency struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Sources []struct {
		Path string `json:"path"`
	} `json:"source_references"`
}

// phaseHeaderRe matches both the legacy header ("tokens ...") and the F1
// header carrying model/provider/latency fields.
var phaseHeaderRe = regexp.MustCompile(`--- (\w+) \([^()]*tokens(?: in=(\d+) out=(\d+))? total=(\d+)[^()]*\) ---`)

// extractDependencyMap finds the dependency_map phase JSON in a live log.
func extractDependencyMap(text string) ([]mapDependency, bool) {
	i := strings.Index(text, "--- dependency_map")
	if i < 0 {
		return nil, false
	}
	j := strings.Index(text[i:], "{")
	if j < 0 {
		return nil, false
	}
	start := i + j
	depth := 0
	for k := start; k < len(text); k++ {
		switch text[k] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				var doc struct {
					Dependencies []mapDependency `json:"dependencies"`
				}
				if json.Unmarshal([]byte(text[start:k+1]), &doc) == nil {
					return doc.Dependencies, true
				}
				return nil, false
			}
		}
	}
	return nil, false
}

var depKindsForFault = map[string]map[string]bool{
	"missing_configuration": {"environment_variable": true, "credential": true},
	"silent_failure":        {"credential": true, "external_service": true},
	"incomplete_backup":     {"backup_gap": true, "file_storage": true},
	"documentation_gap":     {"documentation_gap": true},
}

var envTokenRe = regexp.MustCompile(`[A-Z][A-Z0-9_]{2,}`)

// mapRecall matches ground-truth faults to model dependencies by kind
// (compatibility table above) and target (an env-style token from the
// fault description in the dependency name/id, or a shared source path).
func mapRecall(deps []mapDependency, truth []truthFault) (matched, observable int, details []string) {
	for _, f := range truth {
		kinds, ok := depKindsForFault[f.Kind]
		if !ok {
			continue
		}
		observable++
		tokens := envTokenRe.FindAllString(f.Description, -1)
		paths := map[string]bool{}
		for _, e := range f.Evidence {
			paths[strings.TrimPrefix(e.Path, "demo-app/")] = true
		}
		hit := ""
		for _, d := range deps {
			if !kinds[d.Kind] {
				continue
			}
			for _, tok := range tokens {
				if strings.Contains(d.Name, tok) || strings.Contains(d.ID, strings.ToLower(tok)) {
					hit = d.ID
				}
			}
			for _, s := range d.Sources {
				if paths[s.Path] {
					hit = d.ID
				}
			}
			if hit != "" {
				break
			}
		}
		if hit != "" {
			matched++
			details = append(details, f.ID+" -> "+hit)
		} else {
			details = append(details, f.ID+" -> no matching dependency")
		}
	}
	return matched, observable, details
}

type phaseStat struct {
	Phase, Label, Tokens string
}

var totalNumRe = regexp.MustCompile(`total=(\d+)`)

func phaseTotal(tokens string) int {
	m := totalNumRe.FindStringSubmatch(tokens)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func phaseStats(evDir string) ([]phaseStat, int, string) {
	var out []phaseStat
	retries := 0
	retryRe := regexp.MustCompile(`(?i)schema validation failed|previous output failed schema validation`)
	var labels []string
	for _, name := range []string{"live-scan.log", "live-full.log", "verify.log"} {
		path := filepath.Join(evDir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := strings.ReplaceAll(string(raw), "\x00", "")
		label := runLabel(path)
		labels = append(labels, label)
		retries += len(retryRe.FindAllString(text, -1))
		for _, m := range phaseHeaderRe.FindAllStringSubmatch(text, -1) {
			tok := "total=" + m[4]
			if m[2] != "" {
				tok = "in=" + m[2] + " out=" + m[3] + " total=" + m[4]
			}
			out = append(out, phaseStat{Phase: m[1], Label: label, Tokens: tok})
		}
	}
	return out, retries, strings.Join(labels, ", ")
}

func modelQualitySection(evDir string, demoTruth []truthFault) string {
	var b strings.Builder
	b.WriteString("\n## Model quality (from existing artifacts only)\n\n")
	cc := countCitations(evDir)
	if cc.Found {
		fmt.Fprintf(&b, "Citation verification, run `%s`: %d verdicts, %d verified, %d invalid, %d missing-citation issues, %d unknown-evidence-id verdicts.\n\n", cc.Label, cc.Total, cc.Verified, cc.Invalid, cc.Missing, cc.UnknownIDs)
	} else {
		b.WriteString("Citation verification: cannot compute - no parseable `verification.json` in the evidence directory.\n\n")
	}
	scanLog := filepath.Join(evDir, "live-scan.log")
	if raw, err := os.ReadFile(scanLog); err == nil {
		text := strings.ReplaceAll(string(raw), "\x00", "")
		if deps, ok := extractDependencyMap(text); ok {
			matched, observable, details := mapRecall(deps, demoTruth)
			fmt.Fprintf(&b, "Dependency-map recall, run `%s`: %d of %d observable ground-truth faults have a matching dependency (%s). Fault kinds with no dependency form (for example prompt_injection) are excluded from the denominator.\n\n", runLabel(scanLog), matched, observable, strings.Join(details, "; "))
		} else {
			b.WriteString("Dependency-map recall: cannot compute - no dependency_map phase JSON found in `live-scan.log`.\n\n")
		}
	} else {
		b.WriteString("Dependency-map recall: cannot compute - `live-scan.log` not present in the evidence directory.\n\n")
	}
	stats, retries, labels := phaseStats(evDir)
	if len(stats) > 0 {
		fmt.Fprintf(&b, "Tokens and schema retries, runs `%s`:\n\n", labels)
		verifierTotal, otherTotal := 0, 0
		for _, s := range stats {
			fmt.Fprintf(&b, "- `%s` from `%s`: %s tokens.\n", s.Phase, s.Label, s.Tokens)
			if n := phaseTotal(s.Tokens); s.Phase == "verification" {
				verifierTotal += n
			} else {
				otherTotal += n
			}
		}
		fmt.Fprintf(&b, "- Schema-retry markers recorded across those logs: %d (the loop logs a line only when retries are exhausted, so a zero count means none were recorded, not that none occurred).\n", retries)
		if verifierTotal > 0 {
			fmt.Fprintf(&b, "- The `Tokens/run` figure for full HandoffProof above excludes the adversarial verifier (%d tokens), so total recorded model usage across these runs is about %d tokens.\n", verifierTotal, verifierTotal+otherTotal)
		}
	} else {
		b.WriteString("Tokens and schema retries: cannot compute - no phase token headers found in the live logs.\n")
	}
	b.WriteString("\nThese model-quality numbers come from one recorded run with five findings and are not a general accuracy claim.\n")
	return b.String()
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
