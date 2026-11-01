package byoa

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProfileValidation(t *testing.T) {
	p, err := Load("testdata/miniapp")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "miniapp" || len(p.Workflows) != 2 {
		t.Fatalf("profile = %+v", p)
	}
	bad := *p
	bad.Workflows = append([]Workflow(nil), p.Workflows...)
	bad.Workflows[0].Steps = append([]Step(nil), p.Workflows[0].Steps...)
	bad.Workflows[0].Steps[0].Service = "missing"
	if err := bad.Validate("testdata/miniapp"); err == nil || !strings.Contains(err.Error(), "unknown service") {
		t.Fatalf("dangling service error = %v", err)
	}
	bad2 := *p
	bad2.Workflows = append([]Workflow(nil), p.Workflows...)
	bad2.Workflows[1].Steps = []Step{{Name: "bad sql", Type: "sql_count", Service: "app", Query: "DELETE FROM users"}}
	if err := bad2.Validate("testdata/miniapp"); err == nil {
		t.Fatal("non-count SQL should be rejected")
	}
}

func TestComposeValidationRejectsPrivileged(t *testing.T) {
	dir := t.TempDir()
	compose := "services:\n  app:\n    image: alpine\n    privileged: true\n"
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCompose(dir, "docker-compose.yml"); err == nil {
		t.Fatal("privileged compose should be rejected")
	}
}

func TestCopySourceKeepsNestedDataDirs(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "app")
	write := func(rel, content string) {
		t.Helper()
		full := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("engine/src/data/places.json", `{"places":[]}`)
	write("data/runtime-state.txt", "runtime")
	write("node_modules/left-pad/index.js", "module.exports = 1")
	write("main.txt", "app")
	if err := CopySource(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "engine", "src", "data", "places.json")); err != nil {
		t.Fatalf("nested source data directory was dropped: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "data", "runtime-state.txt")); !os.IsNotExist(err) {
		t.Fatalf("root runtime data directory should be skipped, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("root node_modules should be skipped, stat err = %v", err)
	}
}

func TestRunnerPythonFixture(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Skipf("docker daemon not available: %v: %s", err, out)
	}
	p, err := Load("testdata/miniapp")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	recs, err := Run(ctx, RunRequest{
		SourceDir: "testdata/miniapp", Profile: p,
		Env:  map[string]string{"MAIL_API_TOKEN": "test-token"},
		Kind: "withdrawn_test_token", Target: "mail", RunID: "run-byoa-test",
	})
	if err != nil {
		t.Fatalf("run: %v records=%v", err, recs)
	}
	var sawPass, sawFail, sawExpected bool
	for _, r := range recs {
		if r.AppID != "miniapp" || r.SourceSHA256 == "" {
			t.Errorf("record missing BYOA provenance: %+v", r)
		}
		if r.Workflow == "mail" && r.Passed {
			sawPass = true
		}
		if r.Workflow == "mail" && !r.Passed {
			sawFail = true
		}
		if r.Assertion == "expected workflow fails after fault injection" && r.Passed {
			sawExpected = true
		}
	}
	if !sawPass || !sawFail || !sawExpected {
		t.Fatalf("records pass=%v fail=%v expected=%v: %+v", sawPass, sawFail, sawExpected, recs)
	}
}
