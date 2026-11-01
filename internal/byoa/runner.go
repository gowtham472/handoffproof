package byoa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/agent"
	"github.com/gowtham472/handoffproof/internal/evidence"
	"github.com/gowtham472/handoffproof/internal/secretpath"
)

type RunRequest struct {
	SourceDir string
	Profile   *Profile
	Env       map[string]string
	Kind      string // fresh_deployment_baseline, withdrawn_test_token, missing_configuration, incomplete_backup
	Target    string // credential name or env var, by kind
	RunID     string
}

type runner struct {
	req       RunRequest
	dir       string
	project   string
	seq       int
	sourceSHA string
}

func composeEnvBase(extra ...string) []string {
	keys := []string{"PATH", "HOME", "USERPROFILE", "SystemRoot", "WINDIR", "TEMP", "TMP", "DOCKER_CONFIG", "DOCKER_CONTEXT", "COMPOSE_PROJECT_NAME"}
	var env []string
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return append(env, extra...)
}

func SourceSHA256(root string) (string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, rel := range files {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(raw)
		fmt.Fprintf(h, "%s %x\n", rel, sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func CopySource(src, dst string) error {
	const maxBytes = 200 << 20
	var total int64
	count := 0
	skipDirs := map[string]bool{"node_modules": true, "dist": true, "backups": true, "data": true, ".git": true}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel != "." && secretpath.Denied(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in application tree is not allowed: %s", rel)
		}
		if d.IsDir() {
			if skipDirs[d.Name()] && rel != "." {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		count++
		if count > 10000 {
			return fmt.Errorf("application has too many files")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		total += int64(len(raw))
		if total > maxBytes {
			return fmt.Errorf("application source exceeds 200 MB")
		}
		return os.WriteFile(filepath.Join(dst, rel), raw, 0o644)
	})
}

var composeDeny = []string{"privileged", "docker.sock", "/var/run/docker", "network_mode: host", "devices:", "cap_add: [\"sys_admin\"]", "pid: host", "ipc: host"}

func ValidateCompose(dir, file string) error {
	raw, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return err
	}
	low := strings.ToLower(string(raw))
	for _, bad := range composeDeny {
		if strings.Contains(low, bad) {
			return fmt.Errorf("compose rejected: forbidden construct %q", bad)
		}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- /") || strings.HasPrefix(t, "- \"") && strings.Contains(t, ":/") {
			return fmt.Errorf("compose rejected: host bind mount")
		}
	}
	return nil
}

func (r *runner) appDir() string { return filepath.Join(r.dir, "app") }

func (r *runner) compose(ctx context.Context, env []string, args ...string) (string, error) {
	full := []string{"compose", "-f", r.req.Profile.ComposeFile, "-f", "hp-override.yml", "-p", r.project}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	cmd.Dir = r.appDir()
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func (r *runner) envList(overrides map[string]string, omit map[string]bool) []string {
	vals := map[string]string{}
	for k, v := range r.req.Env {
		vals[k] = v
	}
	for k, v := range overrides {
		vals[k] = v
	}
	for k := range omit {
		delete(vals, k)
	}
	var extra []string
	for k, v := range vals {
		extra = append(extra, k+"="+v)
	}
	sort.Strings(extra)
	return composeEnvBase(extra...)
}

func (r *runner) writeOverride(ctx context.Context) error {
	// Parse the real compose model; fail closed if Docker cannot.
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", r.req.Profile.ComposeFile, "config", "--format", "json")
	cmd.Dir = r.appDir()
	cmd.Env = r.envList(nil, nil)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("parse compose: %w", err)
	}
	var model struct {
		Services map[string]json.RawMessage `json:"services"`
	}
	if err := json.Unmarshal(out, &model); err != nil {
		return fmt.Errorf("parse compose: %w", err)
	}
	for _, s := range r.req.Profile.Services {
		if _, ok := model.Services[s.Name]; !ok {
			return fmt.Errorf("profile service %q not found in compose file", s.Name)
		}
	}
	var names []string
	for n := range model.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("services:\n")
	for _, n := range names {
		fmt.Fprintf(&b, "  %s:\n    mem_limit: 512m\n    cpus: \"1.0\"\n    pids_limit: 128\n    ports: !override []\n", n)
	}
	b.WriteString("networks:\n  default:\n    internal: true\n")
	return os.WriteFile(filepath.Join(r.appDir(), "hp-override.yml"), []byte(b.String()), 0o644)
}

func (r *runner) up(ctx context.Context, overrides map[string]string, omit map[string]bool) error {
	env := r.envList(overrides, omit)
	if out, err := r.compose(ctx, env, "up", "-d", "--build"); err != nil {
		return fmt.Errorf("compose up: %w: %s", err, agent.Redact(tail(out, 2000)))
	}
	return r.waitHealthy(ctx, env)
}

func (r *runner) down(ctx context.Context) {
	_, _ = r.compose(ctx, r.envList(nil, nil), "down", "-v")
}

func (r *runner) waitHealthy(ctx context.Context, env []string) error {
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		ok := true
		for _, s := range r.req.Profile.Services {
			if s.HealthPath == "" {
				continue
			}
			status, _, err := r.http(ctx, env, s.Name, "GET", s.HealthPath, nil)
			if err != nil || status != 200 {
				ok = false
				break
			}
		}
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	logs, _ := r.compose(ctx, env, "logs", "--tail", "80", r.req.Profile.AppService)
	return fmt.Errorf("application did not become healthy: %s", agent.Redact(tail(logs, 2000)))
}

func probeCommand(runtime, method string) []string {
	switch runtime {
	case "node":
		script := `(async()=>{const o={method:process.env.HP_METHOD||'GET',headers:{'content-type':'application/json'}};const b=process.env.HP_BODY||'';if(b&&o.method!=='GET')o.body=b;const r=await fetch(process.env.HP_URL,o);console.log(r.status);console.log(await r.text())})().catch(e=>{console.error(e.message);process.exit(1)})`
		return []string{"node", "-e", script}
	case "curl":
		return []string{"curl", "-sS", "-X", method, "-H", "content-type: application/json", "-d", "@-", "-w", "\n%{http_code}", "http://127.0.0.1:__PORT____PATH__"}
	case "python3":
		script := `import os,urllib.request,sys; d=(os.environ.get('HP_BODY') or '').encode() or None; req=urllib.request.Request(os.environ['HP_URL'],data=d,method=os.environ.get('HP_METHOD','GET'),headers={'content-type':'application/json'}); 
try:
 r=urllib.request.urlopen(req,timeout=10); print(r.status); print(r.read().decode('utf-8','replace'))
except urllib.error.HTTPError as e:
 print(e.code); print(e.read().decode('utf-8','replace'))`
		return []string{"python3", "-c", script}
	case "wget":
		return []string{"wget", "-qO-", "http://127.0.0.1:__PORT____PATH__"}
	}
	return nil
}

func (r *runner) http(ctx context.Context, env []string, service, method, path string, body map[string]string) (int, string, error) {
	svc, ok := r.req.Profile.Service(service)
	if !ok {
		return 0, "", fmt.Errorf("unknown service %q", service)
	}
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)
	url := fmt.Sprintf("http://127.0.0.1:%d%s", svc.Port, path)
	var raw []byte
	if len(body) > 0 {
		raw, _ = json.Marshal(body)
	}
	args := probeCommand(r.req.Profile.ProbeRuntime, method)
	if args == nil {
		return 0, "", fmt.Errorf("unsupported probe runtime")
	}
	for i, a := range args {
		args[i] = strings.ReplaceAll(strings.ReplaceAll(a, "__PORT__", strconv.Itoa(svc.Port)), "__PATH__", path)
	}
	execEnv := []string{"HP_URL=" + url, "HP_METHOD=" + method, "HP_BODY=" + string(raw)}
	full := []string{"compose", "-f", r.req.Profile.ComposeFile, "-f", "hp-override.yml", "-p", r.project, "exec", "-T"}
	for _, e := range execEnv {
		full = append(full, "-e", e)
	}
	full = append(full, service)
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	cmd.Dir = r.appDir()
	cmd.Env = env
	if r.req.Profile.ProbeRuntime == "curl" {
		cmd.Stdin = bytes.NewReader(raw)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if r.req.Profile.ProbeRuntime == "wget" {
		if err != nil {
			return 0, out, err
		}
		return 200, out, nil
	}
	if r.req.Profile.ProbeRuntime == "curl" {
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) == 0 {
			return 0, out, err
		}
		status, cerr := strconv.Atoi(strings.TrimSpace(lines[len(lines)-1]))
		if cerr != nil {
			return 0, out, fmt.Errorf("probe output: %w", err)
		}
		return status, strings.Join(lines[:len(lines)-1], "\n"), nil
	}
	lines := strings.SplitN(strings.TrimSpace(out), "\n", 2)
	if len(lines) == 0 {
		return 0, out, err
	}
	status, cerr := strconv.Atoi(strings.TrimSpace(lines[0]))
	if cerr != nil {
		return 0, out, fmt.Errorf("probe output: %s", agent.Redact(tail(out, 500)))
	}
	bodyOut := ""
	if len(lines) > 1 {
		bodyOut = lines[1]
	}
	return status, bodyOut, err
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func (r *runner) emit(recs *[]evidence.Record, drill, workflow, assertion string, passed bool, faultTarget string) {
	r.seq++
	sum := sha256.Sum256([]byte(assertion + "|" + r.sourceSHA))
	*recs = append(*recs, evidence.Record{
		ID: evidence.NewID(r.req.RunID, drill, workflow, r.seq), RunID: r.req.RunID, Drill: drill,
		Workflow: workflow, Assertion: assertion, Passed: passed, ArtifactSHA256: hex.EncodeToString(sum[:]),
		CreatedAt: time.Now().UTC(), AppID: r.req.Profile.ID, ProfileVersion: r.req.Profile.Version,
		SourceSHA256: r.sourceSHA, FaultTarget: faultTarget,
	})
}

func (r *runner) runStep(ctx context.Context, env []string, st Step) bool {
	deadline := 0
	if st.PollSeconds > 0 {
		deadline = st.PollSeconds
	}
	for {
		passed := false
		switch st.Type {
		case "http":
			status, body, err := r.http(ctx, env, st.Service, st.Method, st.Path, st.Body)
			want := st.ExpectedStatus
			if want == 0 {
				want = 200
			}
			passed = err == nil && status == want && (st.Contains == "" || strings.Contains(body, st.Contains))
		case "sql_count":
			passed = r.sqlCount(ctx, env, st) >= st.MinCount && st.MinCount > 0 || (st.MinCount == 0 && r.sqlCount(ctx, env, st) >= 0)
			// Recompute cleanly: a zero minimum means at least zero rows.
			if st.MinCount == 0 {
				passed = r.sqlCount(ctx, env, st) >= 0
			}
		case "log_contains":
			out, err := r.compose(ctx, env, "logs", "--tail", "200", st.Service)
			passed = err == nil && strings.Contains(out, st.Contains)
		case "file_exists":
			_, err := r.exec(ctx, env, st.Service, []string{"test", "-f", st.FilePath})
			passed = err == nil
		}
		if passed || deadline <= 0 {
			return passed
		}
		deadline -= 2
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
	}
}

func (r *runner) exec(ctx context.Context, env []string, service string, argv []string) (string, error) {
	full := []string{"compose", "-f", r.req.Profile.ComposeFile, "-f", "hp-override.yml", "-p", r.project, "exec", "-T", service}
	full = append(full, argv...)
	cmd := exec.CommandContext(ctx, "docker", full...)
	cmd.Dir = r.appDir()
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func (r *runner) sqlCount(ctx context.Context, env []string, st Step) int {
	var ds *DataStore
	for i := range r.req.Profile.DataStores {
		if r.req.Profile.DataStores[i].Service == st.Service && r.req.Profile.DataStores[i].Kind == "postgres" {
			ds = &r.req.Profile.DataStores[i]
			break
		}
	}
	if ds == nil {
		return -1
	}
	user := ds.User
	if user == "" {
		user = "postgres"
	}
	db := ds.Database
	if db == "" {
		db = "postgres"
	}
	out, err := r.exec(ctx, env, st.Service, []string{"psql", "-U", user, "-d", db, "-tAc", st.Query})
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return -1
	}
	return n
}

func (r *runner) runWorkflows(ctx context.Context, env []string, drill string, faultTarget string) ([]evidence.Record, map[string]bool) {
	var recs []evidence.Record
	results := map[string]bool{}
	for _, w := range r.req.Profile.Workflows {
		all := true
		for _, st := range w.Steps {
			passed := r.runStep(ctx, env, st)
			r.emit(&recs, drill, w.Name, st.Name, passed, faultTarget)
			if !passed {
				all = false
			}
		}
		results[w.Name] = all
	}
	return recs, results
}

func Run(ctx context.Context, req RunRequest) ([]evidence.Record, error) {
	if req.Profile == nil {
		return nil, fmt.Errorf("profile is required")
	}
	if err := req.Profile.Validate(req.SourceDir); err != nil {
		return nil, err
	}
	if err := ValidateCompose(req.SourceDir, req.Profile.ComposeFile); err != nil {
		return nil, err
	}
	for _, e := range req.Profile.Env {
		if e.Required {
			if _, ok := req.Env[e.Name]; !ok {
				return nil, fmt.Errorf("required environment value %s is not configured", e.Name)
			}
		}
	}
	sha, err := SourceSHA256(req.SourceDir)
	if err != nil {
		return nil, err
	}
	r := &runner{req: req, sourceSHA: sha, project: "hpbyoa" + strings.NewReplacer("-", "", "_", "").Replace(req.RunID)}
	dir, err := os.MkdirTemp("", "hp-byoa-*")
	if err != nil {
		return nil, err
	}
	r.dir = dir
	defer os.RemoveAll(dir)
	if err := CopySource(req.SourceDir, r.appDir()); err != nil {
		return nil, fmt.Errorf("copy application: %w", err)
	}
	if err := r.writeOverride(ctx); err != nil {
		return nil, err
	}
	env := r.envList(nil, nil)
	if err := r.up(ctx, nil, nil); err != nil {
		return nil, err
	}
	defer r.down(ctx)

	switch req.Kind {
	case "fresh_deployment_baseline":
		recs, _ := r.runWorkflows(ctx, env, req.Kind, "")
		return recs, nil
	case "withdrawn_test_token", "missing_configuration":
		base, _ := r.runWorkflows(ctx, env, req.Kind, "")
		r.down(ctx)
		var overrides map[string]string
		var omit map[string]bool
		targetWorkflow := ""
		if req.Kind == "withdrawn_test_token" {
			cred, ok := req.Profile.Credential(req.Target)
			if !ok {
				return base, fmt.Errorf("unknown credential %q", req.Target)
			}
			overrides = map[string]string{cred.EnvVar: "handoffproof-invalid-credential"}
			// The workflow expected to break is the first workflow depending on it.
			for _, w := range req.Profile.Workflows {
				for _, d := range w.DependsOn {
					if d == cred.EnvVar {
						targetWorkflow = w.Name
					}
				}
			}
		} else {
			omit = map[string]bool{req.Target: true}
			for _, w := range req.Profile.Workflows {
				for _, d := range w.DependsOn {
					if d == req.Target {
						targetWorkflow = w.Name
					}
				}
			}
		}
		if targetWorkflow == "" {
			return base, fmt.Errorf("no workflow depends on target %q", req.Target)
		}
		if err := r.up(ctx, overrides, omit); err != nil {
			return base, err
		}
		recs, results := r.runWorkflows(ctx, r.envList(overrides, omit), req.Kind, req.Target)
		// Convert fault-phase step records into one verdict record per workflow
		// is unnecessary: step records already show the target failing. Add a
		// summary record that makes the expected break explicit.
		summaryPassed := !results[targetWorkflow]
		r.emit(&recs, req.Kind, targetWorkflow, "expected workflow fails after fault injection", summaryPassed, req.Target)
		return append(base, recs...), nil
	case "incomplete_backup":
		if req.Profile.Backup == nil {
			return nil, fmt.Errorf("profile does not configure backup")
		}
		base, _ := r.runWorkflows(ctx, env, req.Kind, "")
		b := req.Profile.Backup
		if _, err := r.exec(ctx, env, b.Service, b.DumpArgv); err != nil {
			return base, fmt.Errorf("backup dump: %w", err)
		}
		r.down(ctx)
		if err := r.up(ctx, nil, nil); err != nil {
			return base, err
		}
		if _, err := r.exec(ctx, r.envList(nil, nil), b.Service, b.RestoreArgv); err != nil {
			return base, fmt.Errorf("backup restore: %w", err)
		}
		recs, _ := r.runWorkflows(ctx, r.envList(nil, nil), req.Kind, "")
		return append(base, recs...), nil
	default:
		return nil, fmt.Errorf("unsupported BYOA run kind %q", req.Kind)
	}
}
