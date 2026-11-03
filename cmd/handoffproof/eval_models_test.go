package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gowtham472/handoffproof/internal/agent"
)

type stubRepairSubmitter struct{}

func (stubRepairSubmitter) SubmitRepair(ctx context.Context, diff, rationale string, addresses []string) (string, error) {
	return "ok", nil
}

func TestRepairScopedRunnerIsReadOnly(t *testing.T) {
	exec := &agent.Executor{Workspace: t.TempDir(), Repairs: stubRepairSubmitter{}}
	base := agent.NewRunner(agent.Config{APIKey: "k", ModelID: "m", BaseURL: "http://x"}, phaseTestSpec(), exec)
	router := &agent.Router{Default: agent.PhaseRoute{ModelID: "m", Provider: agent.ProviderMetaResponses}}
	base.Router = router
	scoped := repairScopedRunner(base)
	if scoped.Exec.Repairs != nil {
		t.Error("scoped repair runner must unwire submit_repair")
	}
	if exec.Repairs == nil {
		t.Error("scoping must not mutate the shared executor")
	}
	if scoped.Router != router || scoped.Cfg.ModelID != "m" || scoped.Spec != base.Spec {
		t.Error("scoped runner must preserve config, spec and router")
	}
	if repairPhaseTimeout != 10*time.Minute {
		t.Errorf("repair wall-clock cap = %v, want 10m", repairPhaseTimeout)
	}
	if scoped.Cfg.HTTP == nil || scoped.Cfg.HTTP.Timeout != 5*time.Minute {
		t.Errorf("scoped repair transport timeout = %+v, want 5m client", scoped.Cfg.HTTP)
	}
}

func TestCellRepairProtectedPathRefused(t *testing.T) {
	var log []string
	plog := func(format string, args ...any) {
		log = append(log, fmt.Sprintf(format, args...))
	}
	diff := "diff --git a/acceptance/booking.spec.ts b/acceptance/booking.spec.ts\n" +
		"--- a/acceptance/booking.spec.ts\n+++ b/acceptance/booking.spec.ts\n@@ -1 +1 @@\n-x\n+y\n"
	raw, _ := json.Marshal(map[string]any{"repairs": []map[string]any{{
		"repair_id": "R-1", "addresses_findings": []string{"F-1"}, "type": "code",
		"diff": diff, "rationale": "t", "reruns_required": []string{},
		"touches_protected_paths": false,
	}}})
	applied, green := cellRepairAttempt(context.Background(), "../..", raw,
		modelProfile{Name: "spark13"}, "demo-app", 1, plog)
	if applied || green != "n/a" {
		t.Errorf("applied=%v green=%q, want refusal without sandbox", applied, green)
	}
	if strings.Join(log, "\n") == "" || !strings.Contains(strings.Join(log, "\n"), "protected") {
		t.Errorf("log = %q, want protected-path refusal note", log)
	}
}

// phaseSchemas returns a permissive test spec covering all six phases.
func phaseSchemas() map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for _, p := range agent.Phases {
		out[p] = json.RawMessage(`{"type":"object"}`)
	}
	return out
}

func phaseTestSpec() *agent.Spec {
	return &agent.Spec{
		Name:            "test",
		SystemPrompt:    "SYS",
		MaxOutputTokens: 4096,
		Tools:           []agent.ToolDef{{Name: "list_files", Description: "list", Parameters: map[string]any{"path": map[string]any{"type": "string"}}}},
		Schemas:         phaseSchemas(),
	}
}

func clearModelEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"META_API_KEY", "MODEL_API_KEY", "META_MODEL_ID", "META_API_BASE_URL",
		"TOGETHER_API_KEY", "HANDOFF_SPARKOPEN_BASE_URL", "HANDOFF_GLIMMER_BASE_URL",
		"HANDOFF_GLIMMER_API_KEY", "HANDOFF_TEST_ROUTE_KEY"} {
		t.Setenv(k, "")
	}
}

func TestProfileRouteValidation(t *testing.T) {
	clearModelEnv(t)
	byName := map[string]modelProfile{}
	for _, p := range modelProfiles() {
		byName[p.Name] = p
	}
	t.Run("spark13 defaults", func(t *testing.T) {
		t.Setenv("META_API_KEY", "k")
		model, base, key, err := profileRoute(byName["spark13"])
		if err != nil {
			t.Fatal(err)
		}
		if model != "muse-spark-1.3" || base != "https://api.meta.ai/v1" || key != "k" {
			t.Errorf("route = %s %s, want spark 1.3 defaults", model, base)
		}
	})
	t.Run("spark13 missing key fails closed", func(t *testing.T) {
		if _, _, _, err := profileRoute(byName["spark13"]); err == nil ||
			!strings.Contains(err.Error(), "META_API_KEY is not set") {
			t.Errorf("err = %v, want missing-key failure", err)
		}
	})
	t.Run("wonder model id", func(t *testing.T) {
		t.Setenv("META_API_KEY", "k")
		model, _, _, err := profileRoute(byName["wonder"])
		if err != nil {
			t.Fatal(err)
		}
		if model != "muse-wonder-1.0" {
			t.Errorf("model = %s", model)
		}
	})
	t.Run("glimmer unreachable endpoint", func(t *testing.T) {
		t.Setenv("HANDOFF_GLIMMER_BASE_URL", "http://127.0.0.1:1/v1")
		if _, _, _, err := profileRoute(byName["glimmer"]); err == nil ||
			!strings.Contains(err.Error(), "unreachable") {
			t.Errorf("err = %v, want unreachable-endpoint failure", err)
		}
	})
	t.Run("spark-open missing key", func(t *testing.T) {
		if _, _, _, err := profileRoute(byName["spark-open"]); err == nil ||
			!strings.Contains(err.Error(), "TOGETHER_API_KEY is not set") {
			t.Errorf("err = %v, want missing-key failure", err)
		}
	})
}

func TestUnknownProfileSection(t *testing.T) {
	sec := runModelBenchmark(context.Background(), t.TempDir(), t.TempDir(), []string{"nope"}, 3, false)
	if !strings.Contains(sec, "Unknown profile") {
		t.Errorf("section = %q", sec)
	}
}

func writeTempTruth(t *testing.T, root string) {
	t.Helper()
	truth := `{"planted_faults":[{"id":"T-1","kind":"missing_configuration","description":"uses MAIL_TOKEN","expected_static":"undocumented_env:MAIL_TOKEN"}]}`
	if err := os.MkdirAll(filepath.Join(root, "eval"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ground_truth.json", "heldout_truth.json"} {
		if err := os.WriteFile(filepath.Join(root, "eval", name), []byte(truth), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBenchmarkNotRunRow(t *testing.T) {
	clearModelEnv(t)
	root := t.TempDir()
	writeTempTruth(t, root)
	evDir := t.TempDir()
	sec := runModelBenchmark(context.Background(), root, evDir, []string{"spark-open"}, 3, false)
	if !strings.Contains(sec, "not run") || !strings.Contains(sec, "TOGETHER_API_KEY is not set") {
		t.Errorf("section = %q, want labelled not-run row", sec)
	}
	raw, err := os.ReadFile(filepath.Join(evDir, "eval", "spark-open", "NOT_RUN"))
	if err != nil || !strings.Contains(string(raw), "TOGETHER_API_KEY") {
		t.Errorf("NOT_RUN = %q, err = %v", raw, err)
	}
}

// cannedPhaseServer serves one valid JSON per phase name found in the
// Responses text.format selector, with a tool turn first for dependency_map.
func cannedPhaseServer(t *testing.T, bodies *[]map[string]any) *httptest.Server {
	answers := map[string]string{
		"intake_summary":  `{"status":"ok","stack_detected":[],"supported":true,"areas_read":[],"areas_not_read":[],"unknowns":[]}`,
		"dependency_map":  `{"dependencies":[{"id":"D-1","name":"MAIL_TOKEN","kind":"credential","classification":"inferred_risk","ownership_status":"unknown","source_references":[],"rationale":"t","affected_workflows":[],"confidence":0.5,"proposed_check":"t"}]}`,
		"diagnosis":       `{"status":"ok","findings":[]}`,
		"repair_proposal": `{"repairs":[{"repair_id":"R-1","addresses_findings":[],"type":"human_action_required","diff":null,"human_action":"ask a human","rationale":"t","reruns_required":[],"touches_protected_paths":false}]}`,
		"assessment":      `{"summary":"s","checks":[],"limitations":[],"acceptance_integrity":{"hash_before":"a","hash_after":"a","unchanged":true}}`,
		"verification":    `{"verdicts":[{"finding_id":"F-1","citation_ok":true,"evidence_ok":true,"issues":[]}]}`,
	}
	toolFirst := map[string]bool{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Errorf("decode request: %v", err)
		}
		*bodies = append(*bodies, m)
		name := ""
		if tf, ok := m["text"].(map[string]any); ok {
			if fm, ok := tf["format"].(map[string]any); ok {
				name, _ = fm["name"].(string)
			}
		}
		w.Header().Set("content-type", "application/json")
		if name == "dependency_map" && !toolFirst[name] {
			toolFirst[name] = true
			_, _ = w.Write([]byte(`{"id":"r1","model":"m","output":[{"type":"function_call","name":"list_files","arguments":"{\"path\":\".\"}","call_id":"c1"}],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`))
			return
		}
		ans, ok := answers[name]
		if !ok {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"unknown phase"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"r2","model":"m","output":[{"type":"message","content":[{"type":"output_text","text":` + strconv.Quote(ans) + `}]}],"usage":{"input_tokens":5,"output_tokens":7,"total_tokens":12}}`))
	}))
}

func TestRunModelCellOffline(t *testing.T) {
	clearModelEnv(t)
	var bodies []map[string]any
	srv := cannedPhaseServer(t, &bodies)
	defer srv.Close()

	ws := t.TempDir()
	if err := os.WriteFile(ws+"/app.txt", []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	evDir := t.TempDir()
	root := t.TempDir()
	writeTempTruth(t, root)
	// The cell runner loads the real agent spec (read-only); copy it in.
	specRaw, err := os.ReadFile(filepath.Join("..", "..", "prompts", "handoffproof_agent_prompt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "prompts", "handoffproof_agent_prompt.json"), specRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	truth, err := loadTruth(filepath.Join(root, "eval", "heldout_truth.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := modelProfile{Name: "spark13", Provider: agent.ProviderMetaResponses, PriceIn: 1.25, PriceOut: 4.25}
	cell := runModelCell(context.Background(), root, evDir, p, "muse-spark-1.3", srv.URL, "k",
		"heldout-app", ws, 1, truth)
	if !cell.OK {
		t.Fatalf("cell error: %s", cell.Error)
	}
	if cell.Model != "muse-spark-1.3" || cell.Provider != agent.ProviderMetaResponses {
		t.Errorf("cell records model=%q provider=%q", cell.Model, cell.Provider)
	}
	if cell.TotalTokens == 0 || cell.InTokens == 0 || cell.OutTokens == 0 {
		t.Errorf("tokens not recorded: %+v", cell)
	}
	if cell.LatencyMs < 0 {
		t.Errorf("latency not recorded: %+v", cell)
	}
	if cell.ToolTurns != 1 {
		t.Errorf("tool turns = %d, want 1 (dependency_map list_files)", cell.ToolTurns)
	}
	if cell.EstCostUSD <= 0 || cell.Unpriced {
		t.Errorf("cost not computed: %+v", cell)
	}
	if len(cell.Missed) != 1 || cell.Missed[0] != "T-1" {
		t.Errorf("missed = %v, want [T-1] on an empty workspace", cell.Missed)
	}
	if cell.CitVerified != 1 || cell.CitInvalid != 0 {
		t.Errorf("citations = %d/%d, want 1/0", cell.CitVerified, cell.CitInvalid)
	}
	if cell.RepairProposed || cell.FirstTryApplied || cell.RerunGreen != "n/a" {
		t.Errorf("repair = proposed:%v applied:%v green:%s, want no diff path", cell.RepairProposed, cell.FirstTryApplied, cell.RerunGreen)
	}
	raw, err := os.ReadFile(filepath.Join(cellPath(evDir, "spark13", "heldout-app", 1), "cell.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reread modelCell
	if json.Unmarshal(raw, &reread) != nil || !reread.OK {
		t.Errorf("cell.json unreadable: %s", raw)
	}
	for _, phase := range agent.Phases {
		if _, err := os.Stat(filepath.Join(cellPath(evDir, "spark13", "heldout-app", 1), phase+".json")); err != nil {
			t.Errorf("missing %s.json: %v", phase, err)
		}
	}
}

func TestRenderModelTableFromFilesOnly(t *testing.T) {
	evDir := t.TempDir()
	p := modelProfile{Name: "spark13", Provider: agent.ProviderMetaResponses, PriceIn: 1.25, PriceOut: 4.25, PriceNote: "Standard mode"}
	write := func(app string, rep int, c modelCell) {
		t.Helper()
		dir := cellPath(evDir, p.Name, app, rep)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		c.Profile, c.App, c.Rep = p.Name, app, rep
		raw, _ := json.Marshal(c)
		if err := os.WriteFile(filepath.Join(dir, "cell.json"), append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := modelCell{OK: true, Model: "muse-spark-1.3", Provider: agent.ProviderMetaResponses,
		Caught: []string{"F-1"}, Missed: []string{"F-2"}, FalseAlarms: 1,
		CitVerified: 2, CitInvalid: 1, SchemaFailTurns: 1, ToolTurns: 2,
		RepairProposed: true, FirstTryApplied: true, RerunGreen: "true"}
	toks := []int{10, 20, 30}
	for rep, tok := range toks {
		c := base
		c.TotalTokens, c.InTokens, c.OutTokens = tok, tok/2, tok/2
		c.LatencyMs = int64((rep + 1) * 1000)
		c.EstCostUSD = float64(tok) / 1e6 * 1.25
		write("demo-app", rep+1, c)
	}
	failed := base
	failed.OK = false
	failed.Error = "boom"
	write("demo-app", 3, failed) // failed cells do not move the means
	held := base
	held.TotalTokens, held.InTokens, held.OutTokens = 40, 20, 20
	held.LatencyMs = 2000
	held.EstCostUSD = 40.0 / 1e6 * 1.25
	write("heldout-app", 1, held)

	sec := renderModelTable(evDir, []modelProfile{p}, 3)
	for _, want := range []string{"| spark13 | demo-app | 2/3 (1 failed) |",
		"1 (F-1)", "1 (F-2)", "2/1", "| 15 |", "1.5s", "$0.0000", "2/2", "2/2",
		"| spark13 | heldout-app | 1/3 |"} {
		if !strings.Contains(sec, want) {
			t.Errorf("table missing %q:\n%s", want, sec)
		}
	}

	// A profile that never started yields a labelled not-run row.
	writeNotRun(evDir, "wonder", "META_API_KEY is not set", "abc123")
	sec2 := renderModelTable(evDir, []modelProfile{
		p,
		{Name: "wonder", Provider: agent.ProviderMetaResponses},
	}, 3)
	if !strings.Contains(sec2, "not run") || !strings.Contains(sec2, "META_API_KEY is not set") {
		t.Errorf("table missing not-run row:\n%s", sec2)
	}
}

func TestRunModelBenchmarkRenderOnly(t *testing.T) {
	root := t.TempDir()
	writeTempTruth(t, root)
	evDir := filepath.Join(root, "evidence")
	dir := cellPath(evDir, "spark13", "demo-app", 1)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(modelCell{OK: true, Profile: "spark13", App: "demo-app", Rep: 1,
		Model: "muse-spark-1.3", Caught: []string{"T-1"}, TotalTokens: 10, LatencyMs: 1000})
	if err := os.WriteFile(filepath.Join(dir, "cell.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	writeNotRun(evDir, "wonder", "META_API_KEY is not set", "abc123")
	before := 0
	filepath.Walk(evDir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			before++
		}
		return nil
	})
	sec := runModelBenchmark(context.Background(), root, evDir,
		[]string{"spark13", "wonder"}, 3, true)
	for _, want := range []string{"| spark13 | demo-app | 1/3 |", "not run", "META_API_KEY is not set"} {
		if !strings.Contains(sec, want) {
			t.Errorf("render-only table missing %q:\n%s", want, sec)
		}
	}
	after := 0
	filepath.Walk(evDir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			after++
		}
		return nil
	})
	if after != before {
		t.Errorf("render-only wrote %d files, want 0", after-before)
	}
}

func TestPhaseHeaderReBothFormats(t *testing.T) {
	for _, tc := range []struct {
		line  string
		phase string
		total string
	}{
		{"--- diagnosis (tokens total=12) ---", "diagnosis", "12"},
		{"--- diagnosis (tokens in=3 out=4 total=12) ---", "diagnosis", "12"},
		{"--- repair_proposal (model=muse-wonder-1.0 provider=openai-chat tokens in=3 out=4 total=12 latency=1s) ---", "repair_proposal", "12"},
	} {
		m := phaseHeaderRe.FindStringSubmatch(tc.line)
		if m == nil || m[1] != tc.phase || m[4] != tc.total {
			t.Errorf("line %q parsed as %v", tc.line, m)
		}
	}
}
