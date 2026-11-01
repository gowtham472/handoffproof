package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gowtham472/handoffproof/internal/controller"
	"github.com/gowtham472/handoffproof/internal/drills"
	"github.com/gowtham472/handoffproof/internal/evidence"
	"github.com/gowtham472/handoffproof/internal/scan"
	"github.com/gowtham472/handoffproof/internal/score"

	_ "embed"
)

//go:embed web/index.html
var consoleHTML []byte

type job struct {
	ID      string            `json:"id"`
	Drill   string            `json:"drill"`
	Status  string            `json:"status"` // running|done|error
	Error   string            `json:"error,omitempty"`
	Records []evidence.Record `json:"records,omitempty"`
}

type server struct {
	root      string
	tokenHash string // sha256 hex of the console token; the token itself is never stored
	mu        sync.Mutex
	jobs      map[string]*job
	jobDone   map[string]time.Time
	seq       int
	rlMu      sync.Mutex
	rlHits    map[string][]time.Time
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write(consoleHTML)
	})
	mux.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(s.root, "evidence", "report.html"))
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
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
		if r.Method == http.MethodPost && r.Header.Get("X-HP-Request") != "1" {
			writeJSON(w, 403, map[string]string{"error": "missing X-HP-Request header"})
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

// workflowDeps maps each workflow to the dependencies it needs, and each
// dependency to the drill that would confirm the breakage (F3 what-if).
var workflowDeps = map[string][]string{
	"registration":       {"DATABASE_URL"},
	"booking":            {"DATABASE_URL"},
	"confirmation_email": {"DATABASE_URL", "MAIL_API_TOKEN", "MAIL_SINK_URL"},
	"documents":          {"DATABASE_URL", "/data/uploads"},
	"reminder_job":       {"SCHEDULER_TOKEN"},
}

var confirmDrill = map[string]string{
	"MAIL_API_TOKEN":  "withdrawn_test_token",
	"SCHEDULER_TOKEN": "missing_configuration",
	"/data/uploads":   "incomplete_backup",
	"DATABASE_URL":    "fresh_deployment_baseline",
	"MAIL_SINK_URL":   "missing_configuration",
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
		for wf, deps := range workflowDeps {
			for _, d := range deps {
				if d == remove {
					affected = append(affected, map[string]string{
						"workflow": wf, "via": d, "confirm_with": confirmDrill[d],
					})
				}
			}
		}
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
		fmt.Println("WARNING: binding beyond loopback; the console can launch Docker drills — keep the token secret.")
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
