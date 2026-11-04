package byoa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	req            RunRequest
	dir            string
	project        string
	seq            int
	sourceSHA      string
	commandContext func(context.Context, string, ...string) *exec.Cmd
	stepTimeout    time.Duration // optional shorter limit for command regression tests
}

func (r *runner) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	var cmd *exec.Cmd
	if r.commandContext != nil {
		cmd = r.commandContext(ctx, name, args...)
	} else {
		cmd = exec.CommandContext(ctx, name, args...)
	}
	// A Compose plugin can inherit the CLI's output pipes and outlive it on Windows.
	// Context cancellation alone does not bound Cmd.Wait in that case.
	cmd.WaitDelay = 2 * time.Second
	return cmd
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
			// Skip build/runtime directories only at the application root:
			// a nested source directory named data or dist is application code.
			if skipDirs[d.Name()] && rel != "." && filepath.Dir(rel) == "." {
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
	limit := time.Minute
	if len(args) > 0 && args[0] == "up" {
		limit = 6 * time.Minute
	}
	if r.stepTimeout > 0 {
		limit = r.stepTimeout
	}
	stepCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	full := []string{"compose", "-f", r.req.Profile.ComposeFile, "-f", "hp-override.yml", "-p", r.project}
	full = append(full, args...)
	cmd := r.command(stepCtx, "docker", full...)
	cmd.Dir = r.appDir()
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if err != nil || stepCtx.Err() != nil {
		lines := strings.Split(strings.TrimRight(buf.String(), "\r\n"), "\n")
		if len(lines) > 60 {
			lines = lines[len(lines)-60:]
		}
		detail := strings.Join(lines, "\n")
		for _, declared := range r.req.Profile.Env {
			if value := r.req.Env[declared.Name]; declared.Secret && value != "" {
				detail = strings.ReplaceAll(detail, value, "[REDACTED:secret]")
			}
		}
		operation := "command"
		if len(args) > 0 {
			operation = args[0]
		}
		return buf.String(), fmt.Errorf("compose %s: %w; last 60 lines:\n%s", operation, errors.Join(err, stepCtx.Err()), agent.Redact(detail))
	}
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
	parseCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := r.command(parseCtx, "docker", "compose", "-f", r.req.Profile.ComposeFile, "config", "--format", "json")
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
	upCtx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	env := r.envList(overrides, omit)
	if _, err := r.compose(upCtx, env, "up", "-d", "--build"); err != nil {
		return err
	}
	return r.waitHealthy(upCtx, env)
}

func (r *runner) down(ctx context.Context) error {
	// Cleanup must still run after the caller's deadline expires.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	_, err := r.compose(cleanupCtx, r.envList(nil, nil), "down", "-v")
	return err
}

func (r *runner) waitHealthy(ctx context.Context, env []string) error {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		ok := true
		for _, s := range r.req.Profile.Services {
			if s.HealthPath == "" {
				continue
			}
			status, _, err := r.http(ctx, env, s.Name, "GET", s.HealthPath, nil, nil)
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
		script := `(async()=>{const h={'content-type':'application/json'};try{Object.assign(h,JSON.parse(process.env.HP_HEADERS||'{}'))}catch(e){}const o={method:process.env.HP_METHOD||'GET',headers:h};const b=process.env.HP_BODY||'';if(b&&o.method!=='GET')o.body=b;const r=await fetch(process.env.HP_URL,o);console.log(r.status);console.log(await r.text())})().catch(e=>{console.error(e.message);process.exit(1)})`
		return []string{"node", "-e", script}
	case "curl":
		return []string{"curl", "-sS", "-X", method, "-H", "content-type: application/json", "-d", "@-", "-w", "\n%{http_code}", "http://127.0.0.1:__PORT____PATH__"}
	case "python3":
		script := `import os,urllib.request,sys,json; d=(os.environ.get('HP_BODY') or '').encode() or None; h={'content-type':'application/json'}; h.update(json.loads(os.environ.get('HP_HEADERS') or '{}')); req=urllib.request.Request(os.environ['HP_URL'],data=d,method=os.environ.get('HP_METHOD','GET'),headers=h); 
try:
 r=urllib.request.urlopen(req,timeout=10); print(r.status); print(r.read().decode('utf-8','replace'))
except urllib.error.HTTPError as e:
 print(e.code); print(e.read().decode('utf-8','replace'))`
		return []string{"python3", "-c", script}
	case "wget":
		return []string{"wget", "-S", "-qO-", "http://127.0.0.1:__PORT____PATH__"}
	}
	return nil
}

// substEnv expands ${VAR} references from the run environment so header
// and body values can use configured secrets without storing them in the
// profile.
func (r *runner) substEnv(s string) string {
	return envRefRe.ReplaceAllStringFunc(s, func(m string) string {
		name := envRefRe.FindStringSubmatch(m)[1]
		return r.req.Env[name]
	})
}

var envRefRe = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)\}`)

func (r *runner) http(ctx context.Context, env []string, service, method, path string, body map[string]string, headers map[string]string) (int, string, error) {
	svc, ok := r.req.Profile.Service(service)
	if !ok {
		return 0, "", fmt.Errorf("unknown service %q", service)
	}
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)
	url := fmt.Sprintf("http://127.0.0.1:%d%s", svc.Port, path)
	subBody := map[string]string{}
	for k, v := range body {
		subBody[k] = r.substEnv(v)
	}
	var raw []byte
	if len(subBody) > 0 {
		raw, _ = json.Marshal(subBody)
	}
	subHeaders := map[string]string{}
	for k, v := range headers {
		subHeaders[k] = r.substEnv(v)
	}
	headersJSON, _ := json.Marshal(subHeaders)
	args := probeCommand(r.req.Profile.ProbeRuntime, method)
	if args == nil {
		return 0, "", fmt.Errorf("unsupported probe runtime")
	}
	for i, a := range args {
		args[i] = strings.ReplaceAll(strings.ReplaceAll(a, "__PORT__", strconv.Itoa(svc.Port)), "__PATH__", path)
	}
	// curl and wget take headers as arguments, inserted before the URL.
	if rt := r.req.Profile.ProbeRuntime; (rt == "curl" || rt == "wget") && len(subHeaders) > 0 {
		var extra []string
		for name, value := range subHeaders {
			if rt == "curl" {
				extra = append(extra, "-H", name+": "+value)
			} else {
				extra = append(extra, "--header", name+": "+value)
			}
		}
		args = append(args[:len(args)-1], append(extra, args[len(args)-1])...)
	}
	execEnv := []string{"HP_URL=" + url, "HP_METHOD=" + method, "HP_BODY=" + string(raw), "HP_HEADERS=" + string(headersJSON)}
	full := []string{"compose", "-f", r.req.Profile.ComposeFile, "-f", "hp-override.yml", "-p", r.project, "exec", "-T"}
	for _, e := range execEnv {
		full = append(full, "-e", e)
	}
	full = append(full, service)
	full = append(full, args...)
	cmd := r.command(ctx, "docker", full...)
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
		// wget -S prints response headers (including the status line)
		// to stderr, which is merged into out. Parse the real status.
		status := 0
		for _, line := range strings.Split(out, "\n") {
			if fields := strings.Fields(strings.TrimSpace(line)); len(fields) >= 2 && strings.HasPrefix(fields[0], "HTTP/") {
				if n, cerr := strconv.Atoi(fields[1]); cerr == nil {
					status = n
				}
			}
		}
		if status == 0 {
			return 0, out, fmt.Errorf("wget probe returned no HTTP status: %w", err)
		}
		return status, out, nil
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

// emit records one verdict. The artifact hash binds the measured probe
// output (status/body, count, log text) when available, falling back to
// the assertion identity for summary records.
func (r *runner) emit(recs *[]evidence.Record, drill, workflow, assertion string, passed bool, faultTarget string, artifact ...string) {
	r.seq++
	bind := assertion + "|" + r.sourceSHA
	if len(artifact) > 0 && artifact[0] != "" {
		bind = artifact[0] + "|" + r.sourceSHA
	}
	sum := sha256.Sum256([]byte(bind))
	*recs = append(*recs, evidence.Record{
		ID: evidence.NewID(r.req.RunID, drill, workflow, r.seq), RunID: r.req.RunID, Drill: drill,
		Workflow: workflow, Assertion: assertion, Passed: passed, ArtifactSHA256: hex.EncodeToString(sum[:]),
		CreatedAt: time.Now().UTC(), AppID: r.req.Profile.ID, ProfileVersion: r.req.Profile.Version,
		SourceSHA256: r.sourceSHA, FaultTarget: faultTarget,
	})
}

func (r *runner) runStep(ctx context.Context, env []string, st Step) (bool, string) {
	deadline := 0
	if st.PollSeconds > 0 {
		deadline = st.PollSeconds
	}
	for {
		passed := false
		artifact := ""
		switch st.Type {
		case "http":
			status, body, err := r.http(ctx, env, st.Service, st.Method, st.Path, st.Body, st.Headers)
			want := st.ExpectedStatus
			if want == 0 {
				want = 200
			}
			passed = err == nil && status == want && (st.Contains == "" || strings.Contains(body, st.Contains))
			artifact = fmt.Sprintf("http %d %s", status, body)
		case "sql_count":
			n := r.sqlCount(ctx, env, st)
			passed = n >= st.MinCount
			artifact = fmt.Sprintf("sql_count %d (%s)", n, st.Query)
		case "log_contains":
			out, err := r.compose(ctx, env, "logs", "--tail", "200", st.Service)
			passed = err == nil && strings.Contains(out, st.Contains)
			artifact = "logs: " + tail(out, 500)
		case "file_exists":
			_, err := r.exec(ctx, env, st.Service, []string{"test", "-f", st.FilePath})
			passed = err == nil
			artifact = "file_exists " + st.FilePath
		}
		if passed || deadline <= 0 {
			return passed, artifact
		}
		deadline -= 2
		select {
		case <-ctx.Done():
			return false, artifact
		case <-time.After(2 * time.Second):
		}
	}
}

func (r *runner) exec(ctx context.Context, env []string, service string, argv []string) (string, error) {
	full := []string{"compose", "-f", r.req.Profile.ComposeFile, "-f", "hp-override.yml", "-p", r.project, "exec", "-T", service}
	full = append(full, argv...)
	cmd := r.command(ctx, "docker", full...)
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
			passed, artifact := r.runStep(ctx, env, st)
			r.emit(&recs, drill, w.Name, st.Name, passed, faultTarget, artifact)
			if !passed {
				all = false
			}
		}
		results[w.Name] = all
	}
	return recs, results
}

func Run(ctx context.Context, req RunRequest) (records []evidence.Record, runErr error) {
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
	defer func() {
		if err := r.down(ctx); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}()
	if err := r.up(ctx, nil, nil); err != nil {
		return nil, err
	}

	switch req.Kind {
	case "fresh_deployment_baseline":
		recs, _ := r.runWorkflows(ctx, env, req.Kind, "")
		return recs, nil
	case "withdrawn_test_token", "missing_configuration":
		base, _ := r.runWorkflows(ctx, env, req.Kind, "")
		if err := r.down(ctx); err != nil {
			return base, err
		}
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
		if err := r.down(ctx); err != nil {
			return base, err
		}
		if err := r.up(ctx, nil, nil); err != nil {
			return base, err
		}
		if _, err := r.exec(ctx, r.envList(nil, nil), b.Service, b.RestoreArgv); err != nil {
			return base, fmt.Errorf("backup restore: %w", err)
		}
		recs, _ := r.runWorkflows(ctx, r.envList(nil, nil), req.Kind, "")
		// Fault phase: start fresh and skip the restore entirely, as if
		// the backup were incomplete. Detection is proven only when a
		// workflow that passed after the real restore now fails; a
		// profile whose workflows never read restored data honestly
		// reports "not detected" instead of a free pass.
		if err := r.down(ctx); err != nil {
			return append(base, recs...), err
		}
		if err := r.up(ctx, nil, nil); err != nil {
			return append(base, recs...), err
		}
		faultRecs, faultResults := r.runWorkflows(ctx, r.envList(nil, nil), req.Kind, "backup_not_restored")
		detected := false
		for _, ok := range faultResults {
			if !ok {
				detected = true
			}
		}
		r.emit(&faultRecs, req.Kind, "incomplete_backup", "missing restore is detected by a workflow", detected, "backup_not_restored")
		return append(append(base, recs...), faultRecs...), nil
	default:
		return nil, fmt.Errorf("unsupported BYOA run kind %q", req.Kind)
	}
}
