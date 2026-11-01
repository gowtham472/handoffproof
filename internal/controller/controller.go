// Package controller runs drills, writes evidence, and issues verdicts.
// Only this package (with internal/drills) decides pass or fail.
package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/drills"
	"github.com/gowtham472/handoffproof/internal/evidence"
)

// Controller owns drill execution and evidence writes.
type Controller struct {
	Root string // repository root (absolute)
}

// New returns a controller rooted at the repository root.
func New(root string) *Controller { return &Controller{Root: root} }

func (c *Controller) demoApp() string { return filepath.Join(c.Root, "demo-app") }

// manifestRoots are the controller-verified trees (kept in sync with
// scripts/manifest.sh PROTECTED).
var manifestRoots = []string{
	"acceptance", "internal/controller", "internal/drills", "internal/evidence",
	"internal/agent", "internal/scan", "internal/report", "internal/score",
	"cmd", "prompts",
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func manifestSkip(rel string, d os.DirEntry) bool {
	slash := filepath.ToSlash(rel)
	if d.IsDir() {
		switch d.Name() {
		case "node_modules", "dist":
			return true
		}
		return false
	}
	if d.Name() == ".gitkeep" {
		return true
	}
	return strings.Contains(slash, "/test-results/") || strings.Contains(slash, "/playwright-report/")
}

// ManifestVerify checks protected files against .manifest.sha256 in Go.
// It never executes the script it is verifying.
func (c *Controller) ManifestVerify(ctx context.Context) error {
	raw, err := os.ReadFile(filepath.Join(c.Root, ".manifest.sha256"))
	if err != nil {
		return fmt.Errorf("manifest verify: read manifest: %w", err)
	}
	expected := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		hash, path, ok := strings.Cut(line, "  ")
		if !ok {
			// tolerate "hash *path" dialect
			parts := strings.Fields(line)
			if len(parts) != 2 {
				return fmt.Errorf("manifest verify: bad line %q", line)
			}
			hash, path = parts[0], strings.TrimPrefix(parts[1], "*")
		}
		expected[path] = hash
	}
	actual := map[string]string{}
	add := func(rel string) error {
		h, err := hashFile(filepath.Join(c.Root, rel))
		if err != nil {
			return err
		}
		actual[filepath.ToSlash(rel)] = h
		return nil
	}
	for _, root := range manifestRoots {
		base := filepath.Join(c.Root, root)
		err := filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(c.Root, p)
			if err != nil {
				return err
			}
			if manifestSkip(rel, d) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			return add(rel)
		})
		if err != nil {
			return fmt.Errorf("manifest verify: walk %s: %w", root, err)
		}
	}
	if err := add("scripts/manifest.sh"); err != nil {
		return fmt.Errorf("manifest verify: %w", err)
	}
	var diffs []string
	for p, h := range expected {
		if actual[p] != h {
			diffs = append(diffs, "changed/missing: "+p)
		}
	}
	for p := range actual {
		if _, ok := expected[p]; !ok {
			diffs = append(diffs, "unlisted: "+p)
		}
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		if len(diffs) > 10 {
			diffs = diffs[:10]
		}
		return fmt.Errorf("manifest verify: PROTECTED PATH MISMATCH: %s", strings.Join(diffs, "; "))
	}
	return nil
}

func newRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}

// RunDrill executes one drill in a fresh sandbox and persists its evidence.
// The manifest is verified before and after; any mismatch aborts the run.
func (c *Controller) RunDrill(ctx context.Context, name string, params map[string]string) (string, []evidence.Record, error) {
	d, ok := drills.Registry()[name]
	if !ok {
		return "", nil, fmt.Errorf("unknown drill %q", name)
	}
	if err := d.ValidateParams(params); err != nil {
		return "", nil, fmt.Errorf("validate params: %w", err)
	}
	if err := c.ManifestVerify(ctx); err != nil {
		return "", nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	runID := newRunID()
	sb, err := startSandbox(ctx, c.Root, c.demoApp(), runID, name == "missing_configuration")
	if err != nil {
		return runID, nil, err
	}
	defer sb.Close(context.WithoutCancel(ctx))
	seq := 0
	recs, err := d.Run(ctx, sb, runID, &seq)
	if err != nil {
		return runID, recs, fmt.Errorf("drill %s: %w", name, err)
	}
	path, werr := c.WriteEvidence(runID, name, recs)
	if werr != nil {
		return runID, recs, werr
	}
	if err := c.ManifestVerify(ctx); err != nil {
		return runID, recs, err
	}
	return path, recs, nil
}

// WriteEvidence persists records under evidence/ and returns the file path.
func (c *Controller) WriteEvidence(runID, drill string, recs []evidence.Record) (string, error) {
	dir := filepath.Join(c.Root, "evidence")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, runID+"-"+drill+".json")
	raw, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ReadEvidence loads persisted records by file path.
func ReadEvidence(path string) ([]evidence.Record, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var recs []evidence.Record
	if err := json.Unmarshal(raw, &recs); err != nil {
		return nil, err
	}
	return recs, nil
}

var protectedPrefixes = []string{
	"acceptance/", "internal/", "evidence/", ".manifest.sha256", "scripts/manifest.sh",
}

func unquotePath(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "\"") {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return strings.Trim(s, "\"")
	}
	return s
}

func normalizeDiffPath(p string) string {
	p = unquotePath(p)
	p = strings.ReplaceAll(p, "\\", "/")
	for strings.HasPrefix(p, "./") {
		p = strings.TrimPrefix(p, "./")
	}
	// Clean dot segments without resolving leading ../ away.
	parts := strings.Split(p, "/")
	var out []string
	for _, part := range parts {
		if part == "." || part == "" {
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, "/")
}

// ChangedPaths extracts every path a diff touches: ---/+++ headers,
// diff --git lines, and rename/copy from/to lines, in any quoting or case.
func ChangedPaths(diff string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = normalizeDiffPath(p)
		if p == "" || p == "/dev/null" {
			return
		}
		key := strings.ToLower(p)
		if !seen[key] {
			seen[key] = true
			out = append(out, p)
		}
	}
	for _, line := range strings.Split(diff, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "diff --git "):
			rest := strings.TrimPrefix(line, "diff --git ")
			// a/path b/path; take both sides (last token is the b side).
			fields := strings.Fields(rest)
			if len(fields) >= 2 {
				add(strings.TrimPrefix(fields[0], "a/"))
				add(strings.TrimPrefix(fields[len(fields)-1], "b/"))
			}
		case strings.HasPrefix(line, "rename from "), strings.HasPrefix(line, "copy from "):
			add(strings.SplitN(line, " ", 3)[2])
		case strings.HasPrefix(line, "rename to "), strings.HasPrefix(line, "copy to "):
			add(strings.SplitN(line, " ", 3)[2])
		case strings.HasPrefix(line, "--- "):
			add(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(line, "--- ")), "a/"))
		case strings.HasPrefix(line, "+++ "):
			add(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")), "b/"))
		}
	}
	return out
}

// DiffTouchesProtected reports whether a unified diff touches protected paths.
func DiffTouchesProtected(diff string) (bool, string) {
	for _, p := range ChangedPaths(diff) {
		lp := strings.ToLower(p)
		if strings.HasPrefix(lp, "/") || lp == ".." || strings.HasPrefix(lp, "../") {
			return true, p
		}
		for _, pre := range protectedPrefixes {
			if lp == strings.TrimSuffix(pre, "/") || strings.HasPrefix(lp, pre) {
				return true, p
			}
		}
	}
	return false, ""
}

// EnsureDiffHeaders adds missing "diff --git" headers. Model-generated
// multi-file diffs often omit them, and git apply then rejects the patch.
func EnsureDiffHeaders(diff string) string {
	lines := strings.Split(diff, "\n")
	var out []string
	for i, line := range lines {
		if strings.HasPrefix(line, "--- a/") {
			prev := ""
			if len(out) > 0 {
				prev = out[len(out)-1]
			}
			if !strings.HasPrefix(prev, "diff --git ") {
				next := ""
				if i+1 < len(lines) {
					next = lines[i+1]
				}
				a := strings.TrimSpace(strings.TrimPrefix(line, "--- "))
				b := a
				if strings.HasPrefix(next, "+++ ") {
					b = strings.TrimSpace(strings.TrimPrefix(next, "+++ "))
				}
				out = append(out, "diff --git "+a+" "+b)
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// ApplyRepairToCopy applies a repair diff to a fresh sandbox copy of the app
// under test. Protected-path diffs are rejected. Returns the copy directory.
func (c *Controller) ApplyRepairToCopy(ctx context.Context, diff string) (string, error) {
	if bad, p := DiffTouchesProtected(diff); bad {
		return "", fmt.Errorf("repair touches protected path %q", p)
	}
	for _, p := range ChangedPaths(diff) {
		if !strings.HasPrefix(strings.ToLower(p), "app/") {
			return "", fmt.Errorf("repair path %q is outside the app copy", p)
		}
	}
	dir, err := os.MkdirTemp("", "hp-repair-*")
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "app")
	if err := copyApp(c.demoApp(), appDir); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("copy app: %w", err)
	}
	diff = EnsureDiffHeaders(diff)
	if !strings.HasSuffix(diff, "\n") {
		diff += "\n" // model diffs often omit the final newline
	}
	diffPath := filepath.Join(dir, "repair.diff")
	if err := os.WriteFile(diffPath, []byte(diff), 0o644); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	cmd := execCmd(ctx, "git", "apply", "--whitespace=nowarn", "--recount", diffPath)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("git apply: %w: %s", err, tail(buf.String(), 1000))
	}
	// The original tree must be untouched: manifest still verifies.
	if err := c.ManifestVerify(ctx); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return appDir, nil
}

// RerunAcceptance starts the given app copy and runs the identical Playwright
// suite against it. Returns whether the suite passed, plus output tail.
func (c *Controller) RerunAcceptance(ctx context.Context, appDir, runID string) (bool, string, error) {
	if err := c.ManifestVerify(ctx); err != nil {
		return false, "", err
	}
	defer func() { _ = c.ManifestVerify(ctx) }()
	ap, err := freePort()
	if err != nil {
		return false, "", err
	}
	sp, err := freePort()
	if err != nil {
		return false, "", err
	}
	dp, err := freePort()
	if err != nil {
		return false, "", err
	}
	if err := stageMocks(c.Root, filepath.Dir(appDir)); err != nil {
		return false, "", err
	}
	if err := validateComposeFile(appDir); err != nil {
		return false, "", err
	}
	sb := &Sandbox{
		Dir: filepath.Dir(appDir), Project: "hp" + strings.NewReplacer("-", "", "_", "").Replace(runID+"-rerun"),
		AppPort: ap, SinkPort: sp, DBPort: dp,
		MailToken: genToken(), AdminToken: genToken(), SchedToken: genToken(),
		Publish: true,
		http:    httpClient(),
	}
	if err := sb.up(ctx, false); err != nil {
		return false, "", err
	}
	defer sb.Close(context.WithoutCancel(ctx))
	name := "npx"
	if runtime.GOOS == "windows" {
		name = "npx.cmd"
	}
	cmd := execCmd(ctx, name, "playwright", "test")
	cmd.Dir = filepath.Join(c.Root, "acceptance")
	cmd.Env = append(os.Environ(),
		"HANDOFFPROOF_BASE_URL="+sb.appBase(),
		"HANDOFFPROOF_MAIL_SINK_URL="+sb.sinkBase(),
		fmt.Sprintf("HANDOFFPROOF_DATABASE_URL=postgres://handoff:handoff_pw@127.0.0.1:%d/handoffproof", sb.DBPort),
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err = cmd.Run()
	return err == nil, tail(buf.String(), 3000), nil
}
