// Command handoffproof is the CLI: scan, drill, report.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gowtham472/handoffproof/internal/agent"
	"github.com/gowtham472/handoffproof/internal/controller"
	"github.com/gowtham472/handoffproof/internal/evidence"
	"github.com/gowtham472/handoffproof/internal/report"
	"github.com/gowtham472/handoffproof/internal/scan"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	root, _ := os.Getwd()
	loadDotEnv(filepath.Join(root, ".env"))
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "scan":
		err = cmdScan(ctx, root, os.Args[2:])
	case "drill":
		err = cmdDrill(ctx, root, os.Args[2:])
	case "repair":
		err = cmdRepair(ctx, root, os.Args[2:])
	case "serve":
		err = cmdServe(root, os.Args[2:])
	case "verify":
		err = cmdVerify(ctx, root, os.Args[2:])
	case "eval":
		err = cmdEval(root, os.Args[2:])
	case "pr":
		err = cmdPR(ctx, root, os.Args[2:])
	case "score":
		err = cmdScore(root, os.Args[2:])
	case "report":
		err = cmdReport(root, os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: handoffproof <scan|drill|report> [flags]")
	fmt.Fprintln(os.Stderr, "  scan [--repo demo-app]           static scan, plus agent intake/dependency map when configured")
	fmt.Fprintln(os.Stderr, "  drill --drill NAME [--diagnose]  run one drill; diagnose when a Meta key is configured")
	fmt.Fprintln(os.Stderr, "  repair --diff FILE               apply a repair diff to a sandbox copy and rerun acceptance")
	fmt.Fprintln(os.Stderr, "  report --evidence FILE [--diagnosis FILE] --out FILE")
	fmt.Fprintln(os.Stderr, "  serve [--addr :8787]             Control Room frontend")
	fmt.Fprintln(os.Stderr, "  verify --diagnosis FILE          adversarial citation + evidence check")
	fmt.Fprintln(os.Stderr, "  eval                             regenerate docs/RESULTS.md")
	fmt.Fprintln(os.Stderr, "  pr --diff FILE [--execute]       guardrailed remediation PR (dry-run default)")
	fmt.Fprintln(os.Stderr, "  score                            Handover Score + trend append")
}

// loadDotEnv reads KEY=VALUE pairs without ever printing values.
func loadDotEnv(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, strings.TrimSpace(v))
		}
	}
}

func specPath(root string) string {
	return filepath.Join(root, "prompts", "handoffproof_agent_prompt.json")
}

func agentRunner(root, repo string, ev *memEvidence, ctrl *controller.Controller, runTag string) (*agent.Runner, error) {
	cfg := agent.ConfigFromEnv()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	spec, err := agent.LoadSpec(specPath(root))
	if err != nil {
		return nil, err
	}
	exec := &agent.Executor{
		Workspace: repo,
		Evidence:  ev,
		Drills:    drillAdapter{ctrl: ctrl, ev: ev},
		Repairs:   repairAdapter{ctrl: ctrl, tag: runTag},
		OnSecurity: func(kind, detail string) {
			fmt.Fprintf(os.Stderr, "security event: %s at %s (ignored, task unchanged)\n", kind, detail)
		},
	}
	if p := os.Getenv("HANDOFFPROOF_OWNERSHIP_FILE"); p != "" {
		exec.Ownership = p
	}
	return agent.NewRunner(cfg, spec, exec), nil
}

// loadEvidenceStore reads every persisted drill record for tool access.
func loadEvidenceStore(root string) *memEvidence {
	ev := newMemEvidence()
	matches, _ := filepath.Glob(filepath.Join(root, "evidence", "run-*.json"))
	for _, m := range matches {
		if recs, err := controller.ReadEvidence(m); err == nil {
			ev.add(recs)
		}
	}
	return ev
}

type memEvidence struct{ byID map[string]evidence.Record }

func newMemEvidence() *memEvidence { return &memEvidence{byID: map[string]evidence.Record{}} }
func (m *memEvidence) add(recs []evidence.Record) {
	for _, r := range recs {
		m.byID[r.ID] = r
	}
}
func (m *memEvidence) ByID(id string) (evidence.Record, bool) {
	r, ok := m.byID[id]
	return r, ok
}
func (m *memEvidence) All() []evidence.Record {
	out := make([]evidence.Record, 0, len(m.byID))
	for _, r := range m.byID {
		out = append(out, r)
	}
	return out
}

type drillAdapter struct {
	ctrl *controller.Controller
	ev   *memEvidence
}

func (a drillAdapter) RequestDrill(ctx context.Context, drill string, params map[string]string, _ string) (string, error) {
	_, recs, err := a.ctrl.RunDrill(ctx, drill, params)
	if err != nil {
		return "", err
	}
	a.ev.add(recs)
	raw, err := json.Marshal(map[string]any{"status": "complete", "records": recs})
	return string(raw), err
}

type repairAdapter struct {
	ctrl *controller.Controller
	tag  string
}

func (a repairAdapter) SubmitRepair(ctx context.Context, diff, _ string, _ []string) (string, error) {
	appDir, err := a.ctrl.ApplyRepairToCopy(ctx, diff)
	if err != nil {
		return "", err
	}
	passed, out, err := a.ctrl.RerunAcceptance(ctx, appDir, a.tag)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("rerun passed=%v\n%s", passed, out), nil
}

func cmdScan(ctx context.Context, root string, args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	repo := fs.String("repo", filepath.Join(root, "demo-app"), "app under test")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rep, err := scan.Scan(*repo)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		return err
	}
	runner, err := agentRunner(root, *repo, newMemEvidence(), controller.New(root), "scan")
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent skipped:", err)
		return nil
	}
	for _, phase := range []string{"intake_summary", "dependency_map"} {
		res, err := runner.RunPhase(ctx, phase, "Analyse the app under test in the workspace. Start with static_scan_report, then list_files/read_file as needed. Return the "+phase+" JSON.")
		if err != nil {
			return fmt.Errorf("phase %s: %w", phase, err)
		}
		fmt.Printf("--- %s (tokens in=%d out=%d total=%d) ---\n%s\n", phase, res.Usage.InputTokens, res.Usage.OutputTokens, res.Usage.TotalTokens, res.JSON)
	}
	return nil
}

func cmdDrill(ctx context.Context, root string, args []string) error {
	fs := flag.NewFlagSet("drill", flag.ContinueOnError)
	name := fs.String("drill", "", "drill name")
	paramsCSV := fs.String("params", "", "comma-separated key=value drill parameters")
	diagnose := fs.Bool("diagnose", true, "run agent diagnosis when configured")
	full := fs.Bool("full", false, "continue to repair proposal, controller rerun, and assessment")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--drill is required")
	}
	params := map[string]string{}
	for _, kv := range strings.Split(*paramsCSV, ",") {
		if kv == "" {
			continue
		}
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("bad --params entry %q", kv)
		}
		params[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	ctrl := controller.New(root)
	path, recs, err := ctrl.RunDrill(ctx, *name, params)
	if err != nil {
		return err
	}
	ev := newMemEvidence()
	ev.add(recs)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(recs)
	fmt.Fprintln(os.Stderr, "evidence:", path)
	if !*diagnose {
		return nil
	}
	runner, err := agentRunner(root, filepath.Join(root, "demo-app"), ev, ctrl, *name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "diagnosis skipped:", err)
		return nil
	}
	summary, _ := json.Marshal(recs)
	res, err := runner.RunPhase(ctx, "diagnosis", "Diagnose from this controller evidence only. Cite evidence_id for every observed claim.\n"+string(summary))
	if err != nil {
		return fmt.Errorf("diagnosis: %w", err)
	}
	out := filepath.Join(root, "evidence", "diagnosis-"+*name+".json")
	if err := os.WriteFile(out, res.JSON, 0o644); err != nil {
		return err
	}
	fmt.Printf("--- diagnosis (tokens in=%d out=%d total=%d) ---\n%s\n", res.Usage.InputTokens, res.Usage.OutputTokens, res.Usage.TotalTokens, res.JSON)
	fmt.Fprintln(os.Stderr, "diagnosis:", out)
	if !*full {
		return nil
	}
	return runRepairLoop(ctx, root, ctrl, runner, *name, recs, res.JSON)
}

// runRepairLoop asks the agent for a repair, applies it to a sandbox copy,
// reruns the identical acceptance suite, then asks for the assessment.
func runRepairLoop(ctx context.Context, root string, ctrl *controller.Controller, runner *agent.Runner, drill string, before []evidence.Record, diagnosis json.RawMessage) error {
	hashBefore := manifestHash(root)
	verifyBefore := ctrl.ManifestVerify(ctx)
	repRes, err := runner.RunPhase(ctx, "repair_proposal",
		"Propose the smallest repair for these findings. Prefer documentation/configuration. "+
			"Return repair_proposal JSON only; do not call submit_repair (the controller applies and reruns). "+
			"Diff paths must be prefixed with app/ (the sandbox copy root). Declare human-only actions as human_action_required.\nDiagnosis:\n"+string(diagnosis))
	if err != nil {
		return fmt.Errorf("repair proposal: %w", err)
	}
	fmt.Printf("--- repair_proposal (tokens total=%d) ---\n%s\n", repRes.Usage.TotalTokens, repRes.JSON)
	var rep struct {
		Repairs []struct {
			ID    string  `json:"repair_id"`
			Type  string  `json:"type"`
			Diff  *string `json:"diff"`
			Human *string `json:"human_action"`
		} `json:"repairs"`
	}
	if err := json.Unmarshal(repRes.JSON, &rep); err != nil {
		return fmt.Errorf("parse repair proposal: %w", err)
	}
	rerunPassed := false
	rerunNote := "no executable diff proposed"
	// Prefer documentation/configuration repairs over code (hp-repair-and-rerun).
	prio := map[string]int{"documentation": 0, "configuration": 1, "backup_procedure": 2, "code": 3, "human_action_required": 4}
	ordered := append([]struct {
		ID    string  `json:"repair_id"`
		Type  string  `json:"type"`
		Diff  *string `json:"diff"`
		Human *string `json:"human_action"`
	}{}, rep.Repairs...)
	sort.SliceStable(ordered, func(i, j int) bool { return prio[ordered[i].Type] < prio[ordered[j].Type] })
	for _, r := range ordered {
		if r.Diff == nil || strings.TrimSpace(*r.Diff) == "" {
			continue
		}
		appDir, err := ctrl.ApplyRepairToCopy(ctx, *r.Diff)
		if err != nil {
			fmt.Printf("repair %s rejected/unappliable: %v\n", r.ID, err)
			continue
		}
		passed, out, err := ctrl.RerunAcceptance(ctx, appDir, drill)
		if err != nil {
			return fmt.Errorf("rerun %s: %w", r.ID, err)
		}
		rerunPassed = passed
		rerunNote = out
		fmt.Println("applied repair:", r.ID, "("+r.Type+")")
		break
	}
	fmt.Println("--- controller rerun passed:", rerunPassed, "---")
	beforeRaw, _ := json.Marshal(before)
	hashAfter := manifestHash(root)
	verifyAfter := ctrl.ManifestVerify(ctx)
	unchanged := verifyBefore == nil && verifyAfter == nil && hashBefore == hashAfter
	assRes, err := runner.RunPhase(ctx, "assessment",
		"Write the final assessment. Before evidence:\n"+string(beforeRaw)+
			"\nDiagnosis:\n"+string(diagnosis)+
			"\nRepair rerun passed="+fmt.Sprint(rerunPassed)+". Rerun output tail:\n"+rerunNote+
			"\nAcceptance manifest hash before="+hashBefore+" after="+hashAfter+" unchanged="+fmt.Sprint(unchanged)+
			" (controller-verified). "+
			"Use after_evidence_id null where the rerun produced no new evidence record; say so in the note.")
	if err != nil {
		return fmt.Errorf("assessment: %w", err)
	}
	out := filepath.Join(root, "evidence", "assessment-"+drill+".json")
	if err := os.WriteFile(out, assRes.JSON, 0o644); err != nil {
		return err
	}
	fmt.Printf("--- assessment (tokens total=%d) ---\n%s\n", assRes.Usage.TotalTokens, assRes.JSON)
	fmt.Fprintln(os.Stderr, "assessment:", out)
	return nil
}

// cmdVerify runs the adversarial verifier: deterministic citation checks,
// then (when configured) a model verification pass that must anchor every
// root cause to a real file:line.
func cmdVerify(ctx context.Context, root string, args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	diagPath := fs.String("diagnosis", "", "diagnosis JSON file")
	repo := fs.String("repo", filepath.Join(root, "demo-app"), "app under test")
	model := fs.Bool("model", true, "also run the model verification pass")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *diagPath == "" {
		return fmt.Errorf("--diagnosis is required")
	}
	raw, err := os.ReadFile(*diagPath)
	if err != nil {
		return err
	}
	store := loadEvidenceStore(root)
	validEvidence := map[string]bool{}
	for _, rec := range store.All() {
		validEvidence[rec.ID] = true
	}
	checks, err := agent.CheckCitations(*repo, raw, validEvidence)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{"deterministic_citations": checks})
	if !*model {
		return nil
	}
	runner, err := agentRunner(root, *repo, loadEvidenceStore(root), controller.New(root), "verify")
	if err != nil {
		fmt.Fprintln(os.Stderr, "model verification skipped:", err)
		return nil
	}
	res, err := runner.RunPhase(ctx, "verification",
		"Adversarially verify this diagnosis. For each finding use read_file/search_code to check the root cause against real code. "+
			"citation_ok is true only when a real file:line supports the root cause; propose corrected_source_references otherwise. "+
			"evidence_ok is true only when observed findings cite evidence_ids.\nDiagnosis:\n"+string(raw))
	if err != nil {
		return fmt.Errorf("verification: %w", err)
	}
	out := filepath.Join(root, "evidence", "verification.json")
	if err := os.WriteFile(out, res.JSON, 0o644); err != nil {
		return err
	}
	fmt.Printf("--- verification (tokens total=%d) ---\n%s\n", res.Usage.TotalTokens, res.JSON)
	fmt.Fprintln(os.Stderr, "verification:", out)
	return nil
}

func cmdRepair(ctx context.Context, root string, args []string) error {
	fs := flag.NewFlagSet("repair", flag.ContinueOnError)
	diffPath := fs.String("diff", "", "unified diff file (paths prefixed with app/)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *diffPath == "" {
		return fmt.Errorf("--diff is required")
	}
	raw, err := os.ReadFile(*diffPath)
	if err != nil {
		return err
	}
	ctrl := controller.New(root)
	appDir, err := ctrl.ApplyRepairToCopy(ctx, string(raw))
	if err != nil {
		return err
	}
	passed, out, err := ctrl.RerunAcceptance(ctx, appDir, "repair")
	if err != nil {
		return err
	}
	fmt.Println(out)
	fmt.Println("rerun passed:", passed)
	if !passed {
		return fmt.Errorf("acceptance rerun failed after repair")
	}
	return nil
}

func manifestHash(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, ".manifest.sha256"))
	if err != nil {
		return "unavailable"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type diagnosisFile struct {
	Findings []struct {
		ID          string   `json:"finding_id"`
		Class       string   `json:"classification"`
		Workflow    string   `json:"workflow"`
		EvidenceIDs []string `json:"evidence_ids"`
		FailedStep  string   `json:"failed_step"`
		RootCause   string   `json:"root_cause"`
		Impact      string   `json:"business_impact"`
		Confidence  float64  `json:"confidence"`
		Logs        []string `json:"log_excerpts"`
		Sources     []string `json:"source_references"`
	} `json:"findings"`
}

func cmdReport(root string, args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	evPath := fs.String("evidence", "", "evidence JSON file")
	diagPath := fs.String("diagnosis", "", "diagnosis JSON file (optional)")
	outPath := fs.String("out", filepath.Join(root, "evidence", "report.html"), "output HTML")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *evPath == "" {
		return fmt.Errorf("--evidence is required")
	}
	recs, err := controller.ReadEvidence(*evPath)
	if err != nil {
		return err
	}
	integrityOK := controller.New(root).ManifestVerify(context.Background()) == nil
	d := report.Data{Evidence: recs, HashBefore: manifestHash(root), HashAfter: manifestHash(root), HashMatch: integrityOK}
	seen := map[string]*report.Check{}
	for _, r := range recs {
		c, ok := seen[r.Workflow]
		if !ok {
			c = &report.Check{Name: r.Workflow}
			seen[r.Workflow] = c
		}
		p := r.Passed
		if r.Passed {
			c.Class = evidence.ObservedPass
		} else {
			c.Class = evidence.ObservedFailure
		}
		c.BeforeID = r.ID
		c.BeforePassed = &p
	}
	for _, c := range seen {
		d.Checks = append(d.Checks, *c)
	}
	if *diagPath != "" {
		raw, err := os.ReadFile(*diagPath)
		if err != nil {
			return err
		}
		var df diagnosisFile
		if err := json.Unmarshal(raw, &df); err != nil {
			return fmt.Errorf("parse diagnosis: %w", err)
		}
		for _, f := range df.Findings {
			v := report.Finding{
				ID: f.ID, Class: evidence.Classification(f.Class), Workflow: f.Workflow,
				EvidenceIDs: f.EvidenceIDs, FailedStep: f.FailedStep,
				RootCause: f.RootCause, BusinessHit: f.Impact, Confidence: f.Confidence,
			}
			if len(f.Logs) > 0 {
				v.LogExcerpt = f.Logs[0]
			}
			if len(f.Sources) > 0 {
				v.SourceRef = f.Sources[0]
				v.SourceRefs = append([]string(nil), f.Sources...)
			}
			switch v.Class {
			case evidence.InferredRisk:
				d.Inferred = append(d.Inferred, v)
			case evidence.UntestedDependency:
				d.Untested = append(d.Untested, v)
			default:
				d.Findings = append(d.Findings, v)
			}
		}
	}
	f, err := os.Create(*outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := report.Render(f, d); err != nil {
		return err
	}
	fmt.Println("report:", *outPath)
	return nil
}
