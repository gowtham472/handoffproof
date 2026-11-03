package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/agent"
	"github.com/gowtham472/handoffproof/internal/byoa"
	"github.com/gowtham472/handoffproof/internal/draft"
	"github.com/gowtham472/handoffproof/internal/privatemode"
)

// draftPhaseTimeout bounds one drafting run, like repair: the loop caps
// steps, tokens and schema retries; this caps wall-clock time.
const draftPhaseTimeout = 10 * time.Minute

// draftAttempts caps drafting rounds: the first draft plus two rounds
// of validator feedback, mirroring the schema-retry budget.
const draftAttempts = 3

// cmdDraft drafts handoffproof.json for an imported Compose app, then
// screens, validates and baselines it. Only a draft that passes every
// check is saved as a candidate for human review in the wizard.
func cmdDraft(ctx context.Context, root string, args []string) error {
	fs := flag.NewFlagSet("draft", flag.ContinueOnError)
	appDir := fs.String("app-dir", "", "Compose application to draft a profile for")
	envFile := fs.String("env-file", "", "KEY=value test environment for the sandbox baseline")
	reps := fs.Int("reps", 1, "independent drafts to make")
	noBaseline := fs.Bool("no-baseline", false, "skip the sandbox baseline (the draft is then never shown)")
	outDir := fs.String("out-dir", filepath.Join(root, "evidence", "drafts"), "where draft records are written")
	render := fs.Bool("render", false, "only regenerate the docs/RESULTS.md auto-drafter section from recorded drafts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *render {
		return replaceResultsSection(root, "f6", renderDraftSection(*outDir))
	}
	if *appDir == "" {
		return fmt.Errorf("--app-dir is required")
	}
	env := map[string]string{}
	if *envFile != "" {
		var err error
		if env, err = readEnvFile(*envFile); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	cfg, route, mode, err := draftRoute()
	if err != nil {
		return err
	}
	spec, err := agent.LoadSpec(filepath.Join(root, "prompts", "f6_profile_draft.json"))
	if err != nil {
		return err
	}
	app := filepath.Base(filepath.Clean(*appDir))
	for i := 1; i <= *reps; i++ {
		res := draftOnce(ctx, root, cfg, route, mode, spec, *appDir, env, !*noBaseline)
		stamp := time.Now().UTC().Format("20060102-150405")
		base := filepath.Join(*outDir, fmt.Sprintf("draft-%s-%s-%d", app, stamp, i))
		raw, _ := json.MarshalIndent(res, "", "  ")
		if err := os.WriteFile(base+".json", append(raw, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("draft %d/%d for %s: model=%s valid=%v baseline_passed=%v shown=%v -> %s.json\n",
			i, *reps, app, res.ModelID, res.ValidationError == "" && res.ModelError == "" && len(res.Rejections) == 0,
			res.BaselinePassed, res.Shown, base)
		for _, p := range append(append([]string{res.ModelError, res.ValidationError, res.BaselineError}, res.Rejections...), res.BaselineFailures...) {
			if p != "" {
				fmt.Println("  -", agent.Redact(p))
			}
		}
	}
	return nil
}

// draftRoute picks the drafting model: Wonder on the Meta API by default
// (HANDOFF_MODEL_DRAFT overrides), or the local Glimmer endpoint when
// private mode is on, since drafting reads the whole source tree.
func draftRoute() (agent.Config, agent.PhaseRoute, privatemode.Mode, error) {
	mode, local, err := privateModeFromEnv()
	if err != nil {
		return agent.Config{}, agent.PhaseRoute{}, mode, err
	}
	cfg := privatemode.Configure(agent.ConfigFromEnv(), mode, local, logBlockedEgress)
	route := local
	if mode == privatemode.Off {
		if cfg.ModelID == "" {
			cfg.ModelID = draft.DefaultModelID
		}
		if err := cfg.Validate(); err != nil {
			return cfg, route, mode, err
		}
		model := os.Getenv("HANDOFF_MODEL_DRAFT")
		if model == "" {
			model = draft.DefaultModelID
		}
		route = agent.PhaseRoute{ModelID: model, BaseURL: strings.TrimSuffix(cfg.BaseURL, "/"), APIKey: cfg.APIKey, Provider: agent.ProviderMetaResponses}
	}
	if err := draft.CheckModel(route.ModelID); err != nil {
		return cfg, route, mode, err
	}
	// Wonder explores many files per turn: allow five minutes per request
	// inside the phase cap, keeping any private-mode transport.
	var transport http.RoundTripper
	if cfg.HTTP != nil {
		transport = cfg.HTTP.Transport
	}
	cfg.HTTP = &http.Client{Timeout: 5 * time.Minute, Transport: transport}
	return cfg, route, mode, nil
}

func draftOnce(ctx context.Context, root string, cfg agent.Config, route agent.PhaseRoute, mode privatemode.Mode, spec *agent.Spec, appDir string, env map[string]string, baseline bool) draft.Result {
	res := draft.Result{
		StartedUTC: time.Now().UTC().Format(time.RFC3339), Commit: gitShort(root),
		App: filepath.Base(filepath.Clean(appDir)), ModelID: route.ModelID, Provider: route.Provider, Mode: string(mode),
	}
	if res.Mode == "" {
		res.Mode = string(privatemode.Off)
	}
	ws, cleanup, err := draft.Workspace(appDir)
	if err != nil {
		res.ModelError = err.Error()
		return res
	}
	defer cleanup()
	task := "Draft the handoffproof.json profile for the Docker Compose application in the workspace. " +
		"Read the code first; include only workflows whose expected results you can see. Return profile_draft JSON only."
	var p *byoa.Profile
	for attempt := 1; attempt <= draftAttempts; attempt++ {
		res.Attempts = attempt
		res.ValidationError, res.Rejections = "", nil
		// A fresh runner per attempt: the loop's per-phase tool budget
		// would otherwise be shared, starving later attempts.
		runner := agent.NewRunner(cfg, spec, &agent.Executor{Workspace: ws})
		runner.Router = &agent.Router{Default: route}
		runner.Mode = res.Mode
		phaseCtx, cancel := context.WithTimeout(ctx, draftPhaseTimeout)
		out, err := runner.RunPhase(phaseCtx, "profile_draft", task)
		cancel()
		if err != nil {
			res.ModelError = err.Error()
			return res
		}
		res.Tokens += out.Usage.TotalTokens
		res.LatencyMS += out.Latency.Milliseconds()
		res.Draft = redactedJSON(draft.Scrub(string(out.JSON), env))
		p, err = draft.Decode(out.JSON)
		if err != nil {
			res.ValidationError = err.Error()
		} else if res.Rejections = draft.Screen(p, env); len(res.Rejections) == 0 {
			if err := draft.Check(p, ws); err != nil {
				res.ValidationError = err.Error()
			}
		}
		if res.ValidationError == "" && len(res.Rejections) == 0 {
			res.FirstAttemptValid = attempt == 1
			break
		}
		if attempt == draftAttempts {
			return res
		}
		// Validator feedback, never values: problems name fields and env
		// names only, and configured values are scrubbed regardless.
		problems := draft.Scrub(agent.Redact(strings.Join(append([]string{res.ValidationError}, res.Rejections...), "; ")), env)
		task = "Your previous profile draft was rejected by the HandoffProof validator: " + problems +
			"\nRe-read the workspace as needed and return a corrected, complete profile_draft JSON only."
	}
	if !baseline {
		return res
	}
	res.BaselineRan = true
	recs, err := byoa.Run(ctx, byoa.RunRequest{SourceDir: ws, Profile: p, Env: env, Kind: "fresh_deployment_baseline",
		RunID: "draft" + time.Now().UTC().Format("150405")})
	if err != nil {
		res.BaselineError = err.Error()
	}
	res.BaselinePassed, res.BaselineFailures, res.EvidenceIDs = draft.Baseline(recs)
	res.BaselinePassed = res.BaselinePassed && err == nil
	res.Shown = res.BaselinePassed
	return res
}

// redactedJSON redacts every string value of the draft and keeps it as
// JSON. Redacting the raw text would hit keys like "secret": true and
// break the structure; text that is not JSON is stored as a redacted string.
func redactedJSON(raw string) json.RawMessage {
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		s, _ := json.Marshal(agent.Redact(raw))
		return s
	}
	out, err := json.Marshal(redactLeaves(doc))
	if err != nil {
		s, _ := json.Marshal(agent.Redact(raw))
		return s
	}
	return out
}

func redactLeaves(v any) any {
	switch t := v.(type) {
	case string:
		return agent.Redact(t)
	case []any:
		for i := range t {
			t[i] = redactLeaves(t[i])
		}
	case map[string]any:
		for k := range t {
			t[k] = redactLeaves(t[k])
		}
	}
	return v
}

// readEnvFile reads KEY=value lines; values are never printed.
func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	env := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s: line without '='", path)
		}
		env[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return env, sc.Err()
}

// loadDraftResults reads every recorded draft attempt, oldest first.
func loadDraftResults(dir string) []draft.Result {
	matches, _ := filepath.Glob(filepath.Join(dir, "draft-*.json"))
	sort.Strings(matches)
	var out []draft.Result
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var r draft.Result
		if json.Unmarshal(raw, &r) == nil {
			out = append(out, r)
		}
	}
	return out
}

// renderDraftSection builds the auto-drafter results from recorded files only.
func renderDraftSection(dir string) string {
	results := loadDraftResults(dir)
	var b strings.Builder
	b.WriteString("## Profile auto-drafter (F6)\n\n")
	b.WriteString("Generated from `evidence/drafts/draft-*.json` by `handoffproof draft --render`. A draft counts as passing only when it decodes strictly, carries no secret values, validates against the profile schema and Compose denylist, and passes a fresh sandbox baseline unedited. Apps are author-written fixtures.\n\n")
	if len(results) == 0 {
		b.WriteString("No drafts recorded yet.\n")
		return b.String()
	}
	type agg struct {
		n, modelOK, first, valid, passed int
		models                           map[string]bool
		reasons                          []string
	}
	byApp := map[string]*agg{}
	var apps []string
	for _, r := range results {
		a := byApp[r.App]
		if a == nil {
			a = &agg{models: map[string]bool{}}
			byApp[r.App] = a
			apps = append(apps, r.App)
		}
		a.n++
		a.models[r.ModelID] = true
		if r.ModelError == "" {
			a.modelOK++
		}
		if r.ModelError == "" && r.ValidationError == "" && len(r.Rejections) == 0 {
			a.valid++
		}
		if r.FirstAttemptValid {
			a.first++
		}
		if r.Shown {
			a.passed++
		}
		for _, p := range append(append([]string{r.ModelError, r.ValidationError, r.BaselineError}, r.Rejections...), r.BaselineFailures...) {
			if p != "" {
				a.reasons = append(a.reasons, agent.Redact(p))
			}
		}
	}
	fmt.Fprintf(&b, "Each draft gets up to %d attempts: validator errors (field names only, never values) are fed back to the model, like schema retries. No human edits anything.\n\n", draftAttempts)
	b.WriteString("| App | Model | Drafts | Model OK | Valid 1st try | Valid + clean | Baseline passed unedited |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, app := range apps {
		a := byApp[app]
		var models []string
		for m := range a.models {
			models = append(models, m)
		}
		sort.Strings(models)
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d | %d/%d (%.0f%%) |\n", app, strings.Join(models, ", "),
			a.n, a.modelOK, a.first, a.valid, a.passed, a.n, 100*float64(a.passed)/float64(a.n))
	}
	notRun, _ := filepath.Glob(filepath.Join(dir, "not-run-*.json"))
	sort.Strings(notRun)
	for _, m := range notRun {
		var nr struct{ App, Reason string }
		if raw, err := os.ReadFile(m); err == nil && json.Unmarshal(raw, &nr) == nil {
			fmt.Fprintf(&b, "| %s | - | not run | - | - | - | %s |\n", nr.App, strings.ReplaceAll(nr.Reason, "|", "/"))
		}
	}
	b.WriteString("\nFailure reasons (final attempt), as recorded:\n\n")
	for _, app := range apps {
		for _, r := range byApp[app].reasons {
			fmt.Fprintf(&b, "- %s: %s\n", app, strings.ReplaceAll(r, "\n", " "))
		}
	}
	return b.String()
}

// replaceResultsSection swaps the marked section of docs/RESULTS.md, or
// appends it when the markers are missing. Other sections are untouched.
func replaceResultsSection(root, key, body string) error {
	path := filepath.Join(root, "docs", "RESULTS.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	begin, end := "<!-- "+key+":begin -->", "<!-- "+key+":end -->"
	block := begin + "\n" + body + end
	text := string(raw)
	i, j := strings.Index(text, begin), strings.Index(text, end)
	if i >= 0 && j > i {
		text = text[:i] + block + text[j+len(end):]
	} else {
		text = strings.TrimRight(text, "\n") + "\n\n" + block + "\n"
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// draftAppRe limits the app query to a plain folder name.
var draftAppRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// draftLatest serves the newest draft for an app that passed every check
// and the sandbox baseline, for human review in the Add application wizard.
// Drafts that failed anything are never served.
func (s *server) draftLatest(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	app := r.URL.Query().Get("app")
	if !draftAppRe.MatchString(app) {
		writeJSON(w, 400, map[string]string{"error": "app must be a folder name"})
		return
	}
	results := loadDraftResults(filepath.Join(s.root, "evidence", "drafts"))
	for i := len(results) - 1; i >= 0; i-- {
		res := results[i]
		if res.App == app && res.Shown {
			writeJSON(w, 200, map[string]any{"found": true, "profile": res.Draft, "model": res.ModelID,
				"started_utc": res.StartedUTC, "evidence_ids": res.EvidenceIDs})
			return
		}
	}
	writeJSON(w, 200, map[string]any{"found": false})
}
