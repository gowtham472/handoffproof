package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/agent"
	"github.com/gowtham472/handoffproof/internal/controller"
	"github.com/gowtham472/handoffproof/internal/evidence"
	"github.com/gowtham472/handoffproof/internal/scan"
)

// modelProfile is one named --models entry. Prices are USD per 1M tokens
// from the official pricing page (2026-11-04); negative means unpriced.
type modelProfile struct {
	Name        string
	ModelID     string
	Provider    string
	KeyEnv      string // required unless OptionalKey; never logged
	BaseURLEnv  string
	BaseURL     string
	OptionalKey bool
	DialCheck   bool // TCP-probe the endpoint before starting (local only)
	PriceIn     float64
	PriceOut    float64
	PriceNote   string
}

func modelProfiles() []modelProfile {
	return []modelProfile{
		{
			Name: "spark13", ModelID: "", Provider: agent.ProviderMetaResponses,
			KeyEnv: "META_API_KEY", PriceIn: 1.25, PriceOut: 4.25,
			PriceNote: "Standard mode",
		},
		{
			Name: "wonder", ModelID: "muse-wonder-1.0", Provider: agent.ProviderMetaResponses,
			KeyEnv: "META_API_KEY", PriceIn: 4.00, PriceOut: 20.00,
			PriceNote: "Standard mode",
		},
		{
			Name: "spark-open", ModelID: "meta-models/muse-spark-open", Provider: agent.ProviderOpenAIChat,
			KeyEnv: "TOGETHER_API_KEY", BaseURL: "https://api.together.xyz/v1",
			BaseURLEnv: "HANDOFF_SPARKOPEN_BASE_URL",
			PriceIn:    -1, PriceOut: -1, PriceNote: "provider-priced (Together AI)",
		},
		{
			Name: "glimmer", ModelID: "muse-glimmer", Provider: agent.ProviderOpenAIChat,
			KeyEnv: "HANDOFF_GLIMMER_API_KEY", OptionalKey: true,
			BaseURL: "http://127.0.0.1:11434/v1", BaseURLEnv: "HANDOFF_GLIMMER_BASE_URL",
			DialCheck: true,
			PriceIn:   -1, PriceOut: -1, PriceNote: "local inference, no API cost",
		},
	}
}

// profileRoute resolves a profile to a concrete route without network use.
// A non-nil error means the profile cannot start; the caller records a
// labelled "not run" row instead of running cells.
func profileRoute(p modelProfile) (model, base, key string, err error) {
	model = p.ModelID
	if p.Name == "spark13" && model == "" {
		model = os.Getenv("META_MODEL_ID")
		if model == "" {
			model = "muse-spark-1.3"
		}
	}
	base = p.BaseURL
	if p.BaseURLEnv != "" {
		if v := os.Getenv(p.BaseURLEnv); v != "" {
			base = v
		}
	}
	if base == "" && p.Provider == agent.ProviderMetaResponses {
		base = os.Getenv("META_API_BASE_URL")
		if base == "" {
			base = "https://api.meta.ai/v1"
		}
	}
	if base == "" {
		return "", "", "", fmt.Errorf("profile %s: no endpoint (set %s)", p.Name, p.BaseURLEnv)
	}
	key = os.Getenv(p.KeyEnv)
	if key == "" && p.Name == "spark13" {
		key = os.Getenv("MODEL_API_KEY")
	}
	if key == "" && !p.OptionalKey {
		return "", "", "", fmt.Errorf("profile %s: %s is not set", p.Name, p.KeyEnv)
	}
	if p.DialCheck {
		hostport := strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
		if i := strings.Index(hostport, "/"); i >= 0 {
			hostport = hostport[:i]
		}
		if !strings.Contains(hostport, ":") {
			hostport += ":80"
		}
		conn, derr := net.DialTimeout("tcp", hostport, 5*time.Second)
		if derr != nil {
			return "", "", "", fmt.Errorf("profile %s: endpoint %s unreachable: %v", p.Name, base, derr)
		}
		conn.Close()
	}
	return model, strings.TrimSuffix(base, "/"), key, nil
}

// modelCell is one recorded profile/app/rep run. It is the only input the
// comparison table reads: the table is generated from cell.json files.
type modelCell struct {
	Profile         string   `json:"profile"`
	App             string   `json:"app"`
	Rep             int      `json:"rep"`
	Commit          string   `json:"commit"`
	Started         string   `json:"started_utc"`
	Model           string   `json:"model"`
	Provider        string   `json:"provider"`
	OK              bool     `json:"ok"`
	Error           string   `json:"error,omitempty"`
	Caught          []string `json:"caught_ids"`
	Missed          []string `json:"missed_ids"`
	FalseAlarms     int      `json:"false_alarms"`
	CitVerified     int      `json:"citations_verified"`
	CitInvalid      int      `json:"citations_invalid"`
	CitMissing      int      `json:"citations_missing_issues"`
	CitUnknown      int      `json:"citations_unknown_ids"`
	SchemaFailTurns int      `json:"schema_fail_turns"`
	ToolTurns       int      `json:"tool_turns"`
	InTokens        int      `json:"input_tokens"`
	OutTokens       int      `json:"output_tokens"`
	TotalTokens     int      `json:"total_tokens"`
	LatencyMs       int64    `json:"latency_ms"`
	EstCostUSD      float64  `json:"est_cost_usd"`
	Unpriced        bool     `json:"unpriced"`
	PriceNote       string   `json:"price_note,omitempty"`
	RepairProposed  bool     `json:"repair_proposed"`
	FirstTryApplied bool     `json:"first_try_applied"`
	RerunGreen      string   `json:"rerun_green"`
}

func cellPath(evDir string, profile, app string, rep int) string {
	return filepath.Join(evDir, "eval", profile, app, fmt.Sprintf("rep%d", rep))
}

// runModelCell executes one profile/app/rep cell and records cell.json.
// Drill evidence is the shared recorded controller evidence (model
// independent); the six agent phases run live behind the profile router.
// Repair apply/rerun execute for real when the proposal carries a diff.
func runModelCell(ctx context.Context, root, evDir string, p modelProfile, model, base, key, app, appDir string, rep int, truth []truthFault) modelCell {
	started := time.Now().UTC()
	cell := modelCell{
		Profile: p.Name, App: app, Rep: rep,
		Commit: gitShort(root), Started: started.Format(time.RFC3339),
		Model: model, Provider: p.Provider, RerunGreen: "n/a",
	}
	if p.PriceIn < 0 || p.PriceOut < 0 {
		cell.Unpriced = true
		cell.PriceNote = p.PriceNote
	}
	dir := cellPath(evDir, p.Name, app, rep)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		cell.Error = err.Error()
		return cell
	}
	logPath := filepath.Join(dir, "phases.log")
	logf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		cell.Error = err.Error()
		return cell
	}
	defer logf.Close()
	plog := func(format string, args ...any) {
		fmt.Fprintf(logf, format+"\n", args...)
	}
	plog("cell profile=%s app=%s rep=%d model=%s provider=%s", p.Name, app, rep, model, p.Provider)

	rep2, err := scan.Scan(appDir)
	if err != nil {
		cell.Error = fmt.Sprintf("scan: %v", err)
		plog("cell error: %s", cell.Error)
		writeCell(dir, &cell)
		return cell
	}
	recs := readEvidenceRecords(evDir)
	ev := newMemEvidence()
	ev.add(recs)

	cfg := agent.Config{APIKey: key, ModelID: model, BaseURL: base}
	spec, err := agent.LoadSpec(specPath(root))
	if err != nil {
		cell.Error = fmt.Sprintf("spec: %v", err)
		plog("cell error: %s", cell.Error)
		writeCell(dir, &cell)
		return cell
	}
	exec := &agent.Executor{
		Workspace: appDir,
		Evidence:  ev,
		OnSecurity: func(kind, detail string) {
			plog("security event: %s at %s (cell run, ignored, task unchanged)", kind, detail)
		},
	}
	runner := agent.NewRunner(cfg, spec, exec)
	route := agent.PhaseRoute{ModelID: model, BaseURL: base, APIKey: key, Provider: p.Provider}
	router := &agent.Router{Default: route, PerPhase: map[string]agent.PhaseRoute{}}
	for _, phase := range agent.Phases {
		router.PerPhase[phase] = route
	}
	runner.Router = router

	var results []*agent.PhaseResult
	run := func(runCtx context.Context, rr *agent.Runner, phase, task string) (*agent.PhaseResult, error) {
		res, err := rr.RunPhase(runCtx, phase, task)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
		plog("%s", logPhaseFormat(phase, res))
		plog("%s", res.JSON)
		if err := os.WriteFile(filepath.Join(dir, phase+".json"), res.JSON, 0o644); err != nil {
			return nil, err
		}
		return res, nil
	}
	fail := func(format string, args ...any) modelCell {
		cell.Error = fmt.Sprintf(format, args...)
		plog("cell error: %s", cell.Error)
		writeCell(dir, &cell)
		return cell
	}

	if _, err := run(ctx, runner, "intake_summary", "Analyse the app under test in the workspace. Start with static_scan_report, then list_files/read_file as needed. Return the intake_summary JSON."); err != nil {
		return fail("intake_summary: %v", err)
	}
	if _, err := run(ctx, runner, "dependency_map", "Map the app under test. Start with static_scan_report, then list_files/read_file as needed. Return the dependency_map JSON."); err != nil {
		return fail("dependency_map: %v", err)
	}
	rawRecs, _ := json.Marshal(recs)
	diagRes, err := run(ctx, runner, "diagnosis", "Diagnose from this controller evidence only. Cite evidence_id for every observed claim.\n"+string(rawRecs))
	if err != nil {
		return fail("diagnosis: %v", err)
	}
	diagJSON := diagRes.JSON

	// The repair phase runs scoped (submit_repair unwired) and wall-clock
	// capped; the controller applies the proposal diff itself.
	repCtx, repCancel := context.WithTimeout(ctx, repairPhaseTimeout)
	defer repCancel()
	repRes, err := run(repCtx, repairScopedRunner(runner), "repair_proposal",
		"Propose the smallest repair for these findings. Prefer documentation/configuration. "+
			"Return repair_proposal JSON only; do not call submit_repair (the controller applies and reruns). "+
			"Diff paths must be prefixed with app/ (the sandbox copy root). Declare human-only actions as human_action_required.\nDiagnosis:\n"+string(diagJSON))
	if err != nil {
		return fail("repair_proposal: %v", err)
	}
	cell.RepairProposed = hasRepairDiff(repRes.JSON)
	if cell.RepairProposed {
		cell.FirstTryApplied, cell.RerunGreen = cellRepairAttempt(ctx, root, repRes.JSON, p, app, rep, plog)
	}

	beforeRaw, _ := json.Marshal(recs)
	ctrl := controller.New(root)
	verifyBefore := ctrl.ManifestVerify(ctx)
	if _, err := run(ctx, runner, "assessment",
		"Write the final assessment. Before evidence:\n"+string(beforeRaw)+
			"\nDiagnosis:\n"+string(diagJSON)+
			"\nRepair rerun passed="+cell.RerunGreen+". Rerun output tail: see phases.log."+
			"\nAcceptance manifest verified before="+fmt.Sprint(verifyBefore == nil)+
			" (controller-verified). "+
			"Use after_evidence_id null where the rerun produced no new evidence record; say so in the note."); err != nil {
		return fail("assessment: %v", err)
	}

	validIDs := map[string]bool{}
	for _, r := range recs {
		validIDs[r.ID] = true
	}
	if verRes, err := run(ctx, runner, "verification",
		"Adversarially verify this diagnosis. For each finding use read_file/search_code to check the root cause against real code. "+
			"citation_ok is true only when a real file:line supports the root cause; propose corrected_source_references otherwise. "+
			"evidence_ok is true only when observed findings cite evidence_ids.\nDiagnosis:\n"+string(diagJSON)); err != nil {
		plog("verification model pass failed (deterministic checks still apply): %v", err)
	} else if _, derr := agent.CheckCitations(appDir, verRes.JSON, validIDs); derr != nil {
		plog("deterministic citation check: %v", derr)
	}

	events := parseSecurityEvents(dir)
	caught, missed := scoreCell(app, rep2, recs, events, truth)
	falseAl := 0
	if app == "demo-app" {
		falseAl = countFalseAlarms(rep2, map[string]bool{"SCHEDULER_TOKEN": true, "ask Ravi": true})
	} else {
		falseAl = countFalseAlarms(rep2, map[string]bool{"API_KEY": true, "ask Sam": true})
	}
	cc := countCitations(dir)
	cell.Caught, cell.Missed = caught, missed
	cell.FalseAlarms = falseAl
	cell.CitVerified, cell.CitInvalid, cell.CitMissing, cell.CitUnknown =
		cc.Verified, cc.Invalid, cc.Missing, cc.UnknownIDs
	finalizeCell(&cell, runner, results, p)
	cell.OK = true
	writeCell(dir, &cell)
	return cell
}

// cellRepairAttempt applies the first ordered repair diff and reruns
// acceptance when applied. It mirrors the repair loop's preference order.
func cellRepairAttempt(ctx context.Context, root string, repairJSON json.RawMessage, p modelProfile, app string, rep int, plog func(string, ...any)) (applied bool, green string) {
	var rep2 struct {
		Repairs []struct {
			ID    string  `json:"repair_id"`
			Type  string  `json:"type"`
			Diff  *string `json:"diff"`
			Human *string `json:"human_action"`
		} `json:"repairs"`
	}
	if err := json.Unmarshal(repairJSON, &rep2); err != nil {
		plog("repair parse: %v", err)
		return false, "n/a"
	}
	prio := map[string]int{"documentation": 0, "configuration": 1, "backup_procedure": 2, "code": 3, "human_action_required": 4}
	ordered := append([]struct {
		ID    string  `json:"repair_id"`
		Type  string  `json:"type"`
		Diff  *string `json:"diff"`
		Human *string `json:"human_action"`
	}{}, rep2.Repairs...)
	sort.SliceStable(ordered, func(i, j int) bool { return prio[ordered[i].Type] < prio[ordered[j].Type] })
	ctrl := controller.New(root)
	for _, r := range ordered {
		if r.Diff == nil || strings.TrimSpace(*r.Diff) == "" {
			continue
		}
		appDir, err := ctrl.ApplyRepairToCopy(ctx, *r.Diff)
		if err != nil {
			plog("repair %s rejected/unappliable: %v", r.ID, err)
			return false, "n/a"
		}
		passed, out, err := ctrl.RerunAcceptance(ctx, appDir, fmt.Sprintf("%s-%s-rep%d", p.Name, app, rep))
		if err != nil {
			plog("rerun %s: %v", r.ID, err)
			return true, "n/a"
		}
		plog("repair %s applied; rerun passed=%v\n%s", r.ID, passed, out)
		if passed {
			return true, "true"
		}
		return true, "false"
	}
	return false, "n/a"
}

func hasRepairDiff(raw json.RawMessage) bool {
	var doc struct {
		Repairs []struct {
			Diff *string `json:"diff"`
		} `json:"repairs"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return false
	}
	for _, r := range doc.Repairs {
		if r.Diff != nil && strings.TrimSpace(*r.Diff) != "" {
			return true
		}
	}
	return false
}

// readEvidenceRecords loads full controller records from run-*.json files.
func readEvidenceRecords(evDir string) []evidence.Record {
	matches, _ := filepath.Glob(filepath.Join(evDir, "run-*.json"))
	var out []evidence.Record
	for _, m := range matches {
		if recs, err := controller.ReadEvidence(m); err == nil {
			out = append(out, recs...)
		}
	}
	return out
}

// scoreCell reuses the deterministic arm scoring: detection is
// controller-driven and identical across profiles; profiles differ on the
// model metrics recorded in the cell.
func scoreCell(app string, rep scan.Report, recs []evidence.Record, events []securityEvent, truth []truthFault) (caught, missed []string) {
	stripped := make([]drillRec, 0, len(recs))
	for _, r := range recs {
		stripped = append(stripped, drillRec{Drill: r.Drill, Workflow: r.Workflow, Passed: r.Passed})
	}
	var judge func(truthFault) bool
	if app == "demo-app" {
		judge = func(f truthFault) bool {
			if staticSignal(rep, f.Kind) || drillSignal(stripped, f.Kind) {
				return true
			}
			return f.Kind == "prompt_injection" && hasSecurityEvent(events, "prompt_injection_attempt")
		}
	} else {
		judge = func(f truthFault) bool { return expectedStaticSignal(rep, f.ExpectedStatic) }
	}
	for _, f := range truth {
		if judge(f) {
			caught = append(caught, f.ID)
		} else {
			missed = append(missed, f.ID)
		}
	}
	sort.Strings(caught)
	sort.Strings(missed)
	return caught, missed
}

func writeCell(dir string, cell *modelCell) {
	raw, _ := json.MarshalIndent(cell, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "cell.json"), append(raw, '\n'), 0o644)
}

// finalizeCell fills token, latency, retry and cost totals from the recorded
// phase results. Call after the last phase of a successful run.
func finalizeCell(cell *modelCell, runner *agent.Runner, results []*agent.PhaseResult, p modelProfile) {
	for _, res := range results {
		cell.InTokens += res.Usage.InputTokens
		cell.OutTokens += res.Usage.OutputTokens
		cell.TotalTokens += res.Usage.TotalTokens
		cell.LatencyMs += res.Latency.Milliseconds()
	}
	cell.SchemaFailTurns = runner.SchemaRetries
	cell.ToolTurns = runner.ToolTurns
	if !cell.Unpriced {
		cell.EstCostUSD = float64(cell.InTokens)/1e6*p.PriceIn + float64(cell.OutTokens)/1e6*p.PriceOut
	}
}

// evalApps are the two benchmark fixtures.
var evalApps = []struct {
	Name string
	Dir  string
}{
	{"demo-app", "demo-app"},
	{"heldout-app", filepath.Join("eval", "fixtures", "heldout-app")},
}

// writeNotRun records why a profile never started, so the table can label
// the row instead of silently omitting it.
func writeNotRun(evDir, profile, reason, commit string) {
	dir := filepath.Join(evDir, "eval", profile)
	_ = os.MkdirAll(dir, 0o755)
	raw, _ := json.Marshal(map[string]string{
		"profile": profile, "reason": reason, "commit": commit,
		"date": time.Now().Format("2006-01-02"),
	})
	_ = os.WriteFile(filepath.Join(dir, "NOT_RUN"), append(raw, '\n'), 0o644)
}

// runModelBenchmark executes every profile/app/rep cell and returns the
// comparison section for docs/RESULTS.md. It never fails the eval command:
// unstartable profiles and failed cells become labelled rows.
// With renderOnly the live cells are not executed; the table is rendered
// from the cell.json files already under evidence/eval.
func runModelBenchmark(ctx context.Context, root, evDir string, names []string, reps int, renderOnly bool) string {
	byName := map[string]modelProfile{}
	for _, p := range modelProfiles() {
		byName[p.Name] = p
	}
	var wanted []modelProfile
	for _, n := range names {
		p, ok := byName[n]
		if !ok {
			return fmt.Sprintf("\n## Model comparison\n\nUnknown profile %q (want one of spark13, wonder, spark-open, glimmer).\n", n)
		}
		wanted = append(wanted, p)
	}
	demoTruth, derr := loadTruth(filepath.Join(root, "eval", "ground_truth.json"))
	heldTruth, herr := loadTruth(filepath.Join(root, "eval", "heldout_truth.json"))
	if derr != nil || herr != nil {
		return fmt.Sprintf("\n## Model comparison\n\nCannot load ground truth: %v %v.\n", derr, herr)
	}
	for _, p := range wanted {
		if renderOnly {
			continue
		}
		model, base, key, err := profileRoute(p)
		if err != nil {
			writeNotRun(evDir, p.Name, err.Error(), gitShort(root))
			continue
		}
		for _, app := range evalApps {
			var truth []truthFault
			if app.Name == "demo-app" {
				truth = demoTruth
			} else {
				truth = heldTruth
			}
			for rep := 1; rep <= reps; rep++ {
				runModelCell(ctx, root, evDir, p, model, base, key, app.Name,
					filepath.Join(root, app.Dir), rep, truth)
			}
		}
	}
	return renderModelTable(evDir, wanted, reps)
}

// modelAggregate summarizes the successful cells of one profile/app pair.
type modelAggregate struct {
	Profile, App       string
	Runs, Failed       int
	Caught, Missed     []string
	FalseAlarms        int
	CitV, CitI         int
	SchemaFails        int
	Tokens             int
	LatencyMs          int64
	Cost               float64
	Unpriced           bool
	PriceNote          string
	ApplyNum, ApplyDen int
	GreenNum, GreenDen int
	NotRun             string
}

func aggregateCells(evDir string, p modelProfile, app string, reps int) modelAggregate {
	agg := modelAggregate{Profile: p.Name, App: app, Unpriced: true, PriceNote: p.PriceNote}
	profileDir := filepath.Join(evDir, "eval", p.Name)
	if raw, err := os.ReadFile(filepath.Join(profileDir, "NOT_RUN")); err == nil {
		var doc struct {
			Reason string `json:"reason"`
		}
		reason := strings.TrimSpace(string(raw))
		if json.Unmarshal(raw, &doc) == nil && doc.Reason != "" {
			reason = doc.Reason
		}
		agg.NotRun = reason
		return agg
	}
	seenCaught := map[string]bool{}
	for rep := 1; rep <= reps; rep++ {
		raw, err := os.ReadFile(filepath.Join(cellPath(evDir, p.Name, app, rep), "cell.json"))
		if err != nil {
			continue
		}
		var c modelCell
		if json.Unmarshal(raw, &c) != nil || !c.OK {
			agg.Failed++
			continue
		}
		agg.Runs++
		for _, id := range c.Caught {
			if !seenCaught[id] {
				seenCaught[id] = true
				agg.Caught = append(agg.Caught, id)
			}
		}
		agg.Missed = c.Missed
		agg.FalseAlarms = c.FalseAlarms
		agg.CitV += c.CitVerified
		agg.CitI += c.CitInvalid
		agg.SchemaFails += c.SchemaFailTurns
		agg.Tokens += c.TotalTokens
		agg.LatencyMs += c.LatencyMs
		if !c.Unpriced {
			agg.Unpriced = false
			agg.Cost += c.EstCostUSD
		}
		if c.RepairProposed {
			agg.ApplyDen++
			if c.FirstTryApplied {
				agg.ApplyNum++
				agg.GreenDen++
				if c.RerunGreen == "true" {
					agg.GreenNum++
				}
			}
		}
	}
	sort.Strings(agg.Caught)
	sort.Strings(agg.Missed)
	return agg
}

func joinOrEmpty(xs []string) string {
	if len(xs) == 0 {
		return "-"
	}
	return strings.Join(xs, ", ")
}

// renderModelTable builds the comparison section from cell.json files only.
// A profile with no runnable cells yields a labelled "not run" row.
func renderModelTable(evDir string, profiles []modelProfile, reps int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Model comparison (profiles x apps, %d runs per cell)\n\n", reps)
	fmt.Fprintf(&b, "Each cell runs the six agent phases live behind the profile router on the same recorded controller drill evidence; detection arms are controller-driven and identical across profiles, while citation, retry, token, latency, cost and repair columns vary by model. Sample size: %d runs per cell on author-written fixtures (`demo-app`, `eval/fixtures/heldout-app`). Rows labelled `not run` never started; their reason is recorded, not omitted. Denominators: apply = cells whose proposal carried a diff; green = applied cells.\n\n", reps)
	b.WriteString("| Profile | App | Runs OK | Caught | Missed | False alarms | Cit V/I | Schema-fail turns | Tokens/run | Latency/run | Est. $/run | Apply 1st-try | Rerun green |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, p := range profiles {
		for _, app := range evalApps {
			agg := aggregateCells(evDir, p, app.Name, reps)
			if agg.NotRun != "" {
				fmt.Fprintf(&b, "| %s | %s | not run | - | - | - | - | - | - | - | - | - | %s |\n", p.Name, app.Name, agg.NotRun)
				continue
			}
			if agg.Runs == 0 {
				fmt.Fprintf(&b, "| %s | %s | 0/%d (%d failed) | - | - | - | - | - | - | - | - | - | - |\n", p.Name, app.Name, reps, agg.Failed)
				continue
			}
			runs := fmt.Sprintf("%d/%d", agg.Runs, reps)
			if agg.Failed > 0 {
				runs += fmt.Sprintf(" (%d failed)", agg.Failed)
			}
			tokens := fmt.Sprintf("%d", agg.Tokens/agg.Runs)
			lat := fmt.Sprintf("%.1fs", float64(agg.LatencyMs)/float64(agg.Runs)/1000.0)
			cost := "n/a (" + agg.PriceNote + ")"
			if !agg.Unpriced {
				cost = fmt.Sprintf("$%.4f", agg.Cost/float64(agg.Runs))
			}
			apply := fmt.Sprintf("%d/%d", agg.ApplyNum, agg.ApplyDen)
			green := fmt.Sprintf("%d/%d", agg.GreenNum, agg.GreenDen)
			if agg.ApplyDen == 0 {
				apply, green = "no diff proposed", "n/a"
			} else if agg.GreenDen == 0 {
				green = "n/a"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %d (%s) | %d (%s) | %d | %d/%d | %d | %s | %s | %s | %s | %s |\n",
				p.Name, app.Name, runs,
				len(agg.Caught), joinOrEmpty(agg.Caught), len(agg.Missed), joinOrEmpty(agg.Missed),
				agg.FalseAlarms, agg.CitV, agg.CitI, agg.SchemaFails,
				tokens, lat, cost, apply, green)
		}
	}
	b.WriteString("\nCell artifacts: `evidence/eval/<profile>/<app>/rep<n>/` (`cell.json`, phase JSONs, `phases.log`).\n")
	return b.String()
}
