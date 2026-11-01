package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Sandbox is one isolated compose stack for a drill.
type Sandbox struct {
	Dir          string
	Project      string
	AppPort      int
	SinkPort     int
	DBPort       int
	MailToken    string
	AdminToken   string
	SchedToken   string
	Publish      bool // rerun mode: loopback-published ports for Playwright
	lastUploadID string
	http         *http.Client
}

// genToken returns a random per-run credential.
func genToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return "run-" + hex.EncodeToString(b)
}

// composeEnvBase is the allowlisted host environment for compose processes.
// The full host environment is never passed (it may hold API keys).
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

var composeDeny = []string{
	"privileged", "docker.sock", "/var/run/docker", "network_mode", "devices:", "cap_add", "pid: host", "ipc: host",
}

var hostMountRe = regexp.MustCompile(`(?m)^\s*-\s*(/|[A-Za-z]:\\)`)

// validateComposeFile fails closed on host-escape constructs in the copied
// application's compose file. The controller never runs it verbatim without
// this check plus the generated override below.
func validateComposeFile(appDir string) error {
	raw, err := os.ReadFile(filepath.Join(appDir, "docker-compose.yml"))
	if err != nil {
		return fmt.Errorf("read compose: %w", err)
	}
	low := strings.ToLower(string(raw))
	for _, bad := range composeDeny {
		if strings.Contains(low, bad) {
			return fmt.Errorf("compose rejected: forbidden construct %q", bad)
		}
	}
	if hostMountRe.MatchString(string(raw)) {
		return fmt.Errorf("compose rejected: host bind mount")
	}
	return nil
}

func composeServices(appDir string) []string {
	raw, err := os.ReadFile(filepath.Join(appDir, "docker-compose.yml"))
	if err != nil {
		return nil
	}
	var out []string
	inSvc := false
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "services:") {
			inSvc = true
			continue
		}
		if inSvc {
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
				break
			}
			if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(strings.TrimSpace(line), ":") {
				out = append(out, strings.TrimSuffix(strings.TrimSpace(line), ":"))
			}
		}
	}
	return out
}

// writeOverride generates the controller-owned override: resource limits
// for every service. Drill mode also forces internal-only networking and
// removes published ports (the controller talks via docker exec instead).
// Rerun mode keeps loopback-only publishing for the Playwright suite.
func writeOverride(sb *Sandbox) error {
	appDir := sb.appDir()
	var b strings.Builder
	b.WriteString("services:\n")
	containerPort := map[string]int{"app": 3000, "mail-sink": 4100, "db": 5432}
	hostPort := map[string]int{"app": sb.AppPort, "mail-sink": sb.SinkPort, "db": sb.DBPort}
	for _, s := range composeServices(appDir) {
		fmt.Fprintf(&b, "  %s:\n    mem_limit: 512m\n    cpus: \"1.0\"\n    pids_limit: 128\n", s)
		if sb.Publish {
			if cp, ok := containerPort[s]; ok {
				fmt.Fprintf(&b, "    ports: !override\n      - \"127.0.0.1:%d:%d\"\n", hostPort[s], cp)
			}
		} else {
			b.WriteString("    ports: !override []\n")
		}
	}
	if sb.Publish {
		b.WriteString("networks:\n  default:\n    internal: false\n  handover:\n    internal: false\n")
	} else {
		b.WriteString("networks:\n  default:\n    internal: true\n  handover:\n    internal: true\n")
	}
	return os.WriteFile(filepath.Join(appDir, "hp-override.yml"), []byte(b.String()), 0o644)
}

// stageMocks copies mocks/mail-sink next to the sandbox app copy so the
// compose build context ../mocks/mail-sink resolves.
func stageMocks(root, dir string) error {
	src := filepath.Join(root, "mocks", "mail-sink")
	dst := filepath.Join(dir, "mocks", "mail-sink")
	if _, err := os.Stat(src); err != nil {
		return nil // no mock to stage
	}
	if err := copyApp(src, dst); err != nil {
		return fmt.Errorf("copy mail-sink: %w", err)
	}
	return nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func copyApp(src, dst string) error {
	skip := map[string]bool{"node_modules": true, "dist": true, "backups": true, "data": true, ".git": true}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in app tree is not allowed in a sandbox copy: %s", rel)
		}
		if d.IsDir() {
			if skip[d.Name()] && rel != "." {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), raw, 0o644)
	})
}

// startSandbox copies the app under test (plus the mail-sink build context
// its compose file references) and brings its stack up.
func startSandbox(ctx context.Context, root, srcApp, runID string, omitScheduler bool) (*Sandbox, error) {
	dir, err := os.MkdirTemp("", "hp-sandbox-*")
	if err != nil {
		return nil, err
	}
	appDir := filepath.Join(dir, "app")
	if err := copyApp(srcApp, appDir); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("copy app: %w", err)
	}
	if err := stageMocks(root, dir); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	if err := validateComposeFile(appDir); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	ap, err := freePort()
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	sp, err := freePort()
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	dp, err := freePort()
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	sb := &Sandbox{
		Dir: dir, Project: "hp" + strings.NewReplacer("-", "", "_", "").Replace(runID),
		AppPort: ap, SinkPort: sp, DBPort: dp,
		MailToken: genToken(), AdminToken: genToken(), SchedToken: genToken(),
		http: httpClient(),
	}
	if err := sb.up(ctx, omitScheduler); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return sb, nil
}

func httpClient() *http.Client { return &http.Client{Timeout: 15 * time.Second} }

// up brings the sandbox stack up from Dir/app and waits for health.
func (sb *Sandbox) up(ctx context.Context, omitScheduler bool) error {
	if err := writeOverride(sb); err != nil {
		return err
	}
	scheduler := sb.SchedToken
	if omitScheduler {
		scheduler = ""
	}
	env := composeEnvBase(
		"HANDOFF_APP_PORT="+strconv.Itoa(sb.AppPort),
		"HANDOFF_SINK_PORT="+strconv.Itoa(sb.SinkPort),
		"HANDOFF_DB_PORT="+strconv.Itoa(sb.DBPort),
		"HANDOFF_MAIL_TOKEN="+sb.MailToken,
		"HANDOFF_SINK_ADMIN_TOKEN="+sb.AdminToken,
		"HANDOFF_SINK_REQUIRE_AUTH=1",
		"HANDOFF_SCHEDULER_TOKEN="+scheduler,
	)
	if out, err := sb.compose(ctx, env, "up", "-d", "--build"); err != nil {
		_, _ = sb.compose(ctx, env, "down", "-v")
		return fmt.Errorf("compose up: %w: %s", err, tail(out, 2000))
	}
	if err := sb.waitHealthy(ctx); err != nil {
		sb.Close(ctx)
		return err
	}
	return nil
}

func (sb *Sandbox) compose(ctx context.Context, env []string, args ...string) (string, error) {
	full := []string{"compose"}
	if _, err := os.Stat(filepath.Join(sb.Dir, "app", "hp-override.yml")); err == nil {
		full = append(full, "-f", "docker-compose.yml", "-f", "hp-override.yml")
	}
	full = append(full, "-p", sb.Project)
	full = append(full, args...)
	cmd := execCmd(ctx, "docker", full...)
	cmd.Dir = filepath.Join(sb.Dir, "app")
	if env != nil {
		cmd.Env = env
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func (sb *Sandbox) composeEnv(ctx context.Context, args ...string) (string, error) {
	env := composeEnvBase(
		"HANDOFF_APP_PORT="+strconv.Itoa(sb.AppPort),
		"HANDOFF_SINK_PORT="+strconv.Itoa(sb.SinkPort),
		"HANDOFF_DB_PORT="+strconv.Itoa(sb.DBPort),
		"HANDOFF_MAIL_TOKEN="+sb.MailToken,
		"HANDOFF_SINK_ADMIN_TOKEN="+sb.AdminToken,
		"HANDOFF_SCHEDULER_TOKEN="+sb.SchedToken,
	)
	return sb.compose(ctx, env, args...)
}

// Close tears the stack down and removes the sandbox copy. Always called.
func (sb *Sandbox) Close(ctx context.Context) {
	if _, err := sb.composeEnv(ctx, "down", "-v"); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox teardown %s: %v\n", sb.Project, err)
	}
	if err := os.RemoveAll(sb.Dir); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox cleanup %s: %v\n", sb.Project, err)
	}
}

func (sb *Sandbox) waitHealthy(ctx context.Context) error {
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		_, _, appErr := sb.execFetch(ctx, "app", 3000, "GET", "health", "", nil)
		_, _, sinkErr := sb.execFetch(ctx, "mail-sink", 4100, "GET", "health", "", nil)
		if appErr == nil && sinkErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	logs, _ := sb.composeEnv(ctx, "logs", "--tail", "80", "app")
	return fmt.Errorf("sandbox did not become healthy (app log): %s", tail(logs, 3000))
}

func (sb *Sandbox) appBase() string  { return fmt.Sprintf("http://127.0.0.1:%d", sb.AppPort) }
func (sb *Sandbox) sinkBase() string { return fmt.Sprintf("http://127.0.0.1:%d", sb.SinkPort) }

func (sb *Sandbox) getOK(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	res, err := sb.http.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}

func (sb *Sandbox) postJSON(ctx context.Context, url, bearer string, body any) (int, []byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("content-type", "application/json")
	if bearer != "" {
		req.Header.Set("authorization", "Bearer "+bearer)
	}
	res, err := sb.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	out, err := io.ReadAll(res.Body)
	return res.StatusCode, out, err
}

func (sb *Sandbox) getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := sb.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", url, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
