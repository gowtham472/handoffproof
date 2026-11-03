package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gowtham472/handoffproof/internal/agent"
	"github.com/gowtham472/handoffproof/internal/controller"
	"github.com/gowtham472/handoffproof/internal/drills"
	"github.com/gowtham472/handoffproof/internal/evidence"
	"github.com/gowtham472/handoffproof/internal/privatemode"
	"github.com/gowtham472/handoffproof/internal/scan"
	"github.com/gowtham472/handoffproof/internal/score"

	_ "embed"
)

//go:embed web/index.html
var consoleHTML []byte

//go:embed web/landing/*
var landingFS embed.FS

//go:embed web/assets/illustrations/*.svg
var illoFS embed.FS

// Illustrations by Storyset - https://storyset.com - recolored to the IBM palette.
var illoFiles = map[string]bool{
	"handover.svg": true, "rehearse.svg": true, "evidence.svg": true,
	"insights.svg": true, "about.svg": true,
}

type job struct {
	ID      string            `json:"id"`
	Drill   string            `json:"drill"`
	Status  string            `json:"status"` // running|done|error
	Error   string            `json:"error,omitempty"`
	Records []evidence.Record `json:"records,omitempty"`
}

type server struct {
	root          string
	tokenHash     string // sha256 hex of the console token; the token itself is never stored
	mu            sync.Mutex
	jobs          map[string]*job
	jobDone       map[string]time.Time
	seq           int
	rlMu          sync.Mutex
	rlHits        map[string][]time.Time
	shareMu       sync.Mutex
	shares        map[string]time.Time // sha256(share id) -> expiry; read-only, GET only
	sharesLoaded  bool
	slackMu       sync.Mutex
	slackStates   map[string]slackOAuthState
	slackDelivery slackDelivery
	githubMu      sync.Mutex
	githubStates  map[string]githubOAuthState
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

func (s *server) authorized(r *http.Request) bool {
	if c, err := r.Cookie("hp_session"); err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.tokenHash)) == 1 {
		return true
	}
	if q := r.URL.Query().Get("token"); q != "" && subtle.ConstantTimeCompare([]byte(hashToken(q)), []byte(s.tokenHash)) == 1 {
		return true
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") &&
		subtle.ConstantTimeCompare([]byte(hashToken(strings.TrimPrefix(h, "Bearer "))), []byte(s.tokenHash)) == 1 {
		return true
	}
	return false
}

// shareFile is the persistent share store. Only SHA-256 hashes of share
// ids are kept, in a 0600 file outside the repository by default, so
// links survive server restarts without a credential list on disk.
func (s *server) shareFile() string {
	if p := os.Getenv("HANDOFF_SHARE_FILE"); p != "" {
		return p
	}
	return filepath.Join(os.TempDir(), "handoffproof-shares.json")
}

func (s *server) loadSharesLocked() {
	if s.sharesLoaded {
		return
	}
	s.sharesLoaded = true
	if s.shares == nil {
		s.shares = map[string]time.Time{}
	}
	if raw, err := os.ReadFile(s.shareFile()); err == nil {
		var stored map[string]time.Time
		if json.Unmarshal(raw, &stored) == nil {
			for id, exp := range stored {
				if time.Now().Before(exp) {
					s.shares[id] = exp
				}
			}
		}
	}
}

func (s *server) saveSharesLocked() {
	raw, err := json.Marshal(s.shares)
	if err != nil {
		return
	}
	tmp := s.shareFile() + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) == nil {
		_ = os.Rename(tmp, s.shareFile())
	}
}

// shareAuthorized reports whether r carries a live read-only share id.
// Share links grant GET only: writers (drills, Slack posts, new shares) stay
// behind the console token.
func (s *server) shareAuthorized(r *http.Request) bool {
	id := r.URL.Query().Get("s")
	if id == "" {
		return false
	}
	s.shareMu.Lock()
	defer s.shareMu.Unlock()
	s.loadSharesLocked()
	exp, ok := s.shares[hashToken(id)]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.shares, hashToken(id))
		s.saveSharesLocked()
		return false
	}
	return true
}

func (s *server) pruneShares() {
	s.shareMu.Lock()
	defer s.shareMu.Unlock()
	s.loadSharesLocked()
	changed := false
	defer func() {
		if changed {
			s.saveSharesLocked()
		}
	}()
	for id, exp := range s.shares {
		if time.Since(exp) > 0 {
			delete(s.shares, id)
			changed = true
		}
	}
	if len(s.shares) > 50 {
		for id := range s.shares {
			delete(s.shares, id)
			changed = true
			if len(s.shares) <= 50 {
				break
			}
		}
	}
}

// shareLink mints a read-only share id (POST) - the only share operation,
// so only console-token holders can create links.
func (s *server) shareLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	s.pruneShares()
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not mint share link"})
		return
	}
	id := hex.EncodeToString(buf)
	s.shareMu.Lock()
	s.loadSharesLocked()
	s.shares[hashToken(id)] = time.Now().Add(24 * time.Hour)
	s.saveSharesLocked()
	s.shareMu.Unlock()
	writeJSON(w, 200, map[string]string{"share": id, "expires": "24h", "scope": "read-only"})
}

// allow enforces a per-client sliding-window rate limit.
func (s *server) allow(r *http.Request, limit int, window time.Duration) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	now := time.Now()
	s.rlMu.Lock()
	defer s.rlMu.Unlock()
	hits := s.rlHits[host]
	kept := hits[:0]
	for _, t := range hits {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		s.rlHits[host] = kept
		return false
	}
	s.rlHits[host] = append(kept, now)
	return true
}

func (s *server) pruneJobs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, done := range s.jobDone {
		if time.Since(done) > 30*time.Minute {
			delete(s.jobs, id)
			delete(s.jobDone, id)
		}
	}
}

func (s *server) evidenceFiles() []string {
	matches, _ := filepath.Glob(filepath.Join(s.root, "evidence", "run-*.json"))
	sort.Strings(matches)
	return matches
}

// interviewLatest serves the newest redacted exit-interview record, or a
// none marker when no interview has run. Stored records are redacted at
// write time; nothing here decrypts or rehydrates secrets.
// privateModeStatus reports the private mode for the Control Room badge:
// mode, local endpoint host, model and the phases kept local. Keys never
// leave the server.
func privateModeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	mode, local, err := privateModeFromEnv()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out := map[string]any{"mode": mode, "local_phases": []string{}}
	if mode != privatemode.Off {
		phases := privatemode.HybridLocalPhases
		if mode == privatemode.Local {
			phases = agent.Phases
		}
		host := local.BaseURL
		if u, err := url.Parse(local.BaseURL); err == nil {
			host = u.Host
		}
		out["local_phases"] = phases
		out["endpoint_host"] = host
		out["model"] = local.ModelID
	}
	writeJSON(w, 200, out)
}

func (s *server) interviewLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	matches, _ := filepath.Glob(filepath.Join(s.root, "evidence", "interview-*.json"))
	if len(matches) == 0 {
		writeJSON(w, 200, map[string]bool{"none": true})
		return
	}
	sort.Strings(matches)
	raw, err := os.ReadFile(matches[len(matches)-1])
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		writeJSON(w, 500, map[string]string{"error": "unreadable interview record"})
		return
	}
	doc["redacted"] = true
	writeJSON(w, 200, doc)
}

func (s *server) allRecords() []evidence.Record {
	var out []evidence.Record
	for _, f := range s.evidenceFiles() {
		recs, err := controller.ReadEvidence(f)
		if err == nil {
			out = append(out, recs...)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// refreshReport regenerates the served report when the newest evidence
// or diagnosis file is newer than the generated HTML (or it is missing),
// so /report never silently serves stale results.
func (s *server) refreshReport(reportPath string) {
	reportInfo, reportErr := os.Stat(reportPath)
	newest := time.Time{}
	latestRun := ""
	for _, f := range s.evidenceFiles() {
		if info, err := os.Stat(f); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
			latestRun = f
		}
	}
	diagPath := ""
	if matches, _ := filepath.Glob(filepath.Join(s.root, "evidence", "diagnosis-*.json")); len(matches) > 0 {
		sort.Strings(matches)
		diagPath = matches[len(matches)-1]
		if info, err := os.Stat(diagPath); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if latestRun == "" {
		return
	}
	if reportErr == nil && !reportInfo.ModTime().Before(newest) {
		return
	}
	_ = generateReport(s.root, latestRun, diagPath, "", reportPath, false)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	landingAssets, err := fs.Sub(landingFS, "web/landing")
	if err != nil {
		panic(fmt.Errorf("open embedded landing assets: %w", err))
	}
	mux.Handle("/landing/", http.StripPrefix("/landing/", http.FileServer(http.FS(landingAssets))))
	mux.HandleFunc("/landing", func(w http.ResponseWriter, r *http.Request) {
		page, err := landingFS.ReadFile("web/landing/index.html")
		if err != nil {
			http.Error(w, "Landing page unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write(consoleHTML)
	})
	mux.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		reportPath := filepath.Join(s.root, "evidence", "report.html")
		s.refreshReport(reportPath)
		http.ServeFile(w, r, reportPath)
	})
	mux.HandleFunc("/api/integrations/slack", s.slackStatus)
	mux.HandleFunc("/api/integrations/slack/connect", s.slackConnect)
	mux.HandleFunc("/api/integrations/slack/callback", s.slackCallback)
	mux.HandleFunc("/api/integrations/slack/notifications", s.slackNotifications)
	mux.HandleFunc("/api/integrations/slack/", s.slackStatus)
	mux.HandleFunc("/api/integrations/github", s.githubStatus)
	mux.HandleFunc("/api/integrations/github/connect", s.githubConnect)
	mux.HandleFunc("/api/integrations/github/callback", s.githubCallback)
	mux.HandleFunc("/api/integrations/github/repositories", s.githubRepositories)
	mux.HandleFunc("/api/integrations/github/repository", s.githubRepository)
	mux.HandleFunc("/api/integrations/github/installations", s.githubInstallations)
	mux.HandleFunc("/api/share", s.shareLink)
	mux.HandleFunc("/assets/illustrations/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/assets/illustrations/")
		if !illoFiles[name] {
			http.NotFound(w, r)
			return
		}
		raw, err := illoFS.ReadFile("web/assets/illustrations/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("/api/summary", func(w http.ResponseWriter, r *http.Request) {
		recs := s.allRecords()
		passed, failed := 0, 0
		runs := map[string]bool{}
		for _, rec := range recs {
			runs[rec.RunID] = true
			if rec.Passed {
				passed++
			} else {
				failed++
			}
		}
		writeJSON(w, 200, map[string]any{
			"records": len(recs), "passed": passed, "failed": failed, "runs": len(runs),
			"manifest": manifestHash(s.root),
			"model":    os.Getenv("META_MODEL_ID"),
		})
	})
	mux.HandleFunc("/api/evidence", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, s.allRecords())
	})
	mux.HandleFunc("/api/scan", func(w http.ResponseWriter, r *http.Request) {
		rep, err := scan.Scan(filepath.Join(s.root, "demo-app"))
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, rep)
	})
	mux.HandleFunc("/api/scoreboard", s.scoreboard)
	mux.HandleFunc("/api/diagnosis", func(w http.ResponseWriter, r *http.Request) {
		s.serveJSONFile(w, "diagnosis-*.json")
	})
	mux.HandleFunc("/api/assessment", func(w http.ResponseWriter, r *http.Request) {
		s.serveJSONFile(w, "assessment-*.json")
	})
	mux.HandleFunc("/api/score", func(w http.ResponseWriter, r *http.Request) {
		res := score.Compute(s.root)
		var history []map[string]any
		if raw, err := os.ReadFile(filepath.Join(s.root, "docs", "score-history.jsonl")); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				var e map[string]any
				if json.Unmarshal([]byte(line), &e) == nil {
					history = append(history, e)
				}
			}
		}
		writeJSON(w, 200, map[string]any{"current": res, "history": history})
	})
	mux.HandleFunc("/api/whatif", s.whatif)
	mux.HandleFunc("/api/drills/run", s.runDrill)
	mux.HandleFunc("/api/interview/latest", s.interviewLatest)
	mux.HandleFunc("/api/private-mode", privateModeStatus)
	mux.HandleFunc("/api/drafts/latest", s.draftLatest)
	mux.HandleFunc("/api/applications", s.applications)
	mux.HandleFunc("/api/applications/", s.applicationByID)
	mux.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
		s.mu.Lock()
		j := s.jobs[id]
		s.mu.Unlock()
		if j == nil {
			writeJSON(w, 404, map[string]string{"error": "unknown job"})
			return
		}
		writeJSON(w, 200, j)
	})
	return s.protect(mux)
}

// protect adds authentication, CSRF, rate limiting and security headers.
// The drill endpoint can launch Docker containers, so the console is never
// served unauthenticated, even on loopback.
func (s *server) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			if !s.allow(r, 240, time.Minute) {
				writeJSON(w, 429, map[string]string{"error": "rate limited"})
				return
			}
		}
		if (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete) && r.Header.Get("X-HP-Request") != "1" {
			writeJSON(w, 403, map[string]string{"error": "missing X-HP-Request header"})
			return
		}
		if (r.URL.Path == "/api/integrations/slack/callback" || r.URL.Path == "/api/integrations/github/callback") && r.Method == http.MethodGet {
			// Slack returns the browser here without a console token. The
			// handler only proceeds with the one-time OAuth state cookie and
			// server-side state record created by an operator.
			next.ServeHTTP(w, r)
			return
		}
		if s.shareAuthorized(r) {
			if r.Method != http.MethodGet {
				writeJSON(w, 403, map[string]string{"error": "share links are read-only"})
				return
			}
			s.pruneShares()
			next.ServeHTTP(w, r)
			return
		}
		if !s.authorized(r) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeJSON(w, 401, map[string]string{"error": "unauthorized: open the console via the token URL printed at startup"})
				return
			}
			w.Header().Set("content-type", "text/plain; charset=utf-8")
			w.WriteHeader(401)
			_, _ = w.Write([]byte("HandoffProof console: add ?token=<console token> (printed at startup) to this URL.\n"))
			return
		}
		if q := r.URL.Query().Get("token"); q != "" {
			http.SetCookie(w, &http.Cookie{
				Name: "hp_session", Value: hashToken(q), Path: "/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 12 * 3600,
			})
		}
		s.pruneJobs()
		next.ServeHTTP(w, r)
	})
}

func (s *server) scoreboard(w http.ResponseWriter, r *http.Request) {
	type fault struct {
		ID     string `json:"id"`
		Kind   string `json:"kind"`
		Status string `json:"status"`
		Basis  string `json:"basis"`
	}
	raw, err := os.ReadFile(filepath.Join(s.root, "eval", "ground_truth.json"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	var gt struct {
		Planted []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"planted_faults"`
	}
	if err := json.Unmarshal(raw, &gt); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	recs := s.allRecords()
	hasFail := func(drill, workflow, assertionPart string) (string, bool) {
		for _, rec := range recs {
			if rec.Drill == drill && rec.Workflow == workflow && !rec.Passed && strings.Contains(rec.Assertion, assertionPart) {
				return rec.ID, true
			}
		}
		return "", false
	}
	rep, _ := scan.Scan(filepath.Join(s.root, "demo-app"))
	askRavi := false
	for _, f := range rep.AskPhrases {
		if f.Value == "ask Ravi" {
			askRavi = true
		}
	}
	injected := false
	for _, pat := range []string{"evidence/*.err", "evidence/*.log"} {
		matches, _ := filepath.Glob(filepath.Join(s.root, pat))
		for _, m := range matches {
			b, err := os.ReadFile(m)
			// PowerShell redirection writes UTF-16; strip NULs before matching.
			if err == nil && strings.Contains(strings.ReplaceAll(string(b), "\x00", ""), "prompt_injection_attempt") {
				injected = true
			}
		}
	}
	var out []fault
	for _, f := range gt.Planted {
		row := fault{ID: f.ID, Kind: f.Kind, Status: "missed", Basis: "no matching evidence yet"}
		switch f.Kind {
		case "missing_configuration":
			if id, ok := hasFail("missing_configuration", "reminder_job", "reminder job"); ok {
				row.Status, row.Basis = "caught", id
			}
		case "silent_failure":
			if id, ok := hasFail("withdrawn_test_token", "confirmation_email", "confirmation email"); ok {
				row.Status, row.Basis = "caught", id
			}
		case "incomplete_backup":
			if id, ok := hasFail("incomplete_backup", "documents", "restored"); ok {
				row.Status, row.Basis = "caught", id
			}
		case "documentation_gap":
			if askRavi {
				row.Status, row.Basis = "caught", "static scan ask-phrase"
			}
		case "prompt_injection":
			if injected {
				row.Status, row.Basis = "caught", "live scan security event"
			}
		}
		out = append(out, row)
	}
	writeJSON(w, 200, out)
}

// derivedWorkflowEdges computes what-if edges from the static scan and
// controller evidence, never from a hand-written dependency map:
//   - a scheduled job (scan) is affected when one of its env vars goes;
//   - a workflow with failing records under missing_configuration is
//     affected when an undocumented env var goes;
//   - a workflow with records under incomplete_backup is affected when
//     a storage path goes.
func (s *server) derivedWorkflowEdges(rep scan.Report, remove string) []map[string]string {
	var affected []map[string]string
	add := func(workflow, via, confirm string) {
		for _, a := range affected {
			if a["workflow"] == workflow && a["via"] == via {
				return
			}
		}
		affected = append(affected, map[string]string{"workflow": workflow, "via": via, "confirm_with": confirm})
	}
	isStorage := false
	for _, sp := range rep.StoragePaths {
		if sp.Value == remove {
			isStorage = true
		}
	}
	if isStorage {
		for _, rec := range s.allRecords() {
			if rec.Drill == "incomplete_backup" {
				add(rec.Workflow, remove, "incomplete_backup")
			}
		}
		return affected
	}
	for _, job := range rep.Jobs {
		for _, env := range job.EnvVars {
			if env == remove {
				add("scheduled job ("+job.Kind+")", remove, "missing_configuration")
			}
		}
	}
	undocumented := false
	for _, name := range rep.UndocumentedEnvVars {
		if name == remove {
			undocumented = true
		}
	}
	if undocumented {
		for _, rec := range s.allRecords() {
			if rec.Drill == "missing_configuration" {
				add(rec.Workflow, remove, "missing_configuration")
			}
		}
	}
	return affected
}

func (s *server) whatif(w http.ResponseWriter, r *http.Request) {
	rep, err := scan.Scan(filepath.Join(s.root, "demo-app"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	type node struct {
		ID         string `json:"id"`
		Kind       string `json:"kind"`
		Documented bool   `json:"documented"`
	}
	nodes := []node{}
	seen := map[string]bool{}
	for _, ev := range rep.EnvVars {
		nodes = append(nodes, node{ev.Name, "environment_variable", ev.Documented})
		seen[ev.Name] = true
	}
	for _, sp := range rep.StoragePaths {
		nodes = append(nodes, node{sp.Value, "file_storage", true})
		seen[sp.Value] = true
	}
	remove := r.URL.Query().Get("remove")
	var affected []map[string]string
	if remove != "" {
		if !seen[remove] {
			writeJSON(w, 400, map[string]string{"error": "unknown dependency " + remove})
			return
		}
		affected = s.derivedWorkflowEdges(rep, remove)
		sort.Slice(affected, func(i, j int) bool { return affected[i]["workflow"] < affected[j]["workflow"] })
	}
	writeJSON(w, 200, map[string]any{"nodes": nodes, "remove": remove, "affected": affected})
}

func (s *server) runDrill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	var body struct {
		Drill string `json:"drill"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad json"})
		return
	}
	if _, ok := drills.Registry()[body.Drill]; !ok {
		writeJSON(w, 400, map[string]string{"error": "unknown drill"})
		return
	}
	s.mu.Lock()
	for _, j := range s.jobs {
		if j.Status == "running" {
			s.mu.Unlock()
			writeJSON(w, 409, map[string]string{"error": "a drill is already running"})
			return
		}
	}
	s.seq++
	j := &job{ID: fmt.Sprintf("job-%d", s.seq), Drill: body.Drill, Status: "running"}
	s.jobs[j.ID] = j
	s.mu.Unlock()
	go func() {
		ctrl := controller.New(s.root)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		_, recs, err := ctrl.RunDrill(ctx, body.Drill, map[string]string{})
		defer s.notifySlack(recs, err)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.jobDone[j.ID] = time.Now()
		if err != nil {
			j.Status = "error"
			j.Error = err.Error()
			return
		}
		j.Status = "done"
		j.Records = recs
	}()
	writeJSON(w, 202, j)
}

func cmdServe(root string, args []string) error {
	addr := "127.0.0.1:8787"
	for i := 0; i < len(args); i++ {
		if args[i] == "--addr" && i+1 < len(args) {
			addr = args[i+1]
		}
	}
	token := os.Getenv("HANDOFF_CONSOLE_TOKEN")
	generated := false
	if len(token) < 16 {
		buf := make([]byte, 24)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		token = hex.EncodeToString(buf)
		generated = true
	}
	s := &server{root: root, tokenHash: hashToken(token), jobs: map[string]*job{}, jobDone: map[string]time.Time{}, rlHits: map[string][]time.Time{}}
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	fmt.Printf("HandoffProof console at http://%s/?token=%s\n", host, token)
	if generated {
		fmt.Println("(random console token generated; set HANDOFF_CONSOLE_TOKEN to choose your own)")
	}
	if !strings.HasPrefix(addr, "127.0.0.1") && !strings.HasPrefix(addr, "localhost") && !strings.HasPrefix(addr, "[::1]") {
		fmt.Println("WARNING: binding beyond loopback; the console can launch Docker drills - keep the token secret.")
	}
	srv := &http.Server{
		Addr: addr, Handler: s.routes(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	return srv.ListenAndServe()
}

func (s *server) serveJSONFile(w http.ResponseWriter, pattern string) {
	matches, _ := filepath.Glob(filepath.Join(s.root, "evidence", pattern))
	if len(matches) == 0 {
		writeJSON(w, 404, map[string]string{"error": "not available yet"})
		return
	}
	sort.Strings(matches)
	raw, err := os.ReadFile(matches[len(matches)-1])
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("content-type", "application/json")
	_, _ = w.Write(raw)
}
