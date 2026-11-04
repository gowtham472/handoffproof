package byoa

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This subprocess is a fake Compose command; it never starts Docker or a model.
func TestComposeCommandHelper(t *testing.T) {
	if os.Getenv("HP_COMPOSE_HELPER") != "1" {
		return
	}
	for i := 0; i < 70; i++ {
		fmt.Printf("line-%02d\n", i)
	}
	fmt.Println("MAIL_API_TOKEN=test-secret-value")
	if os.Args[len(os.Args)-1] == "stall" {
		time.Sleep(5 * time.Second)
	}
	os.Exit(23)
}

func fakeComposeRunner(t *testing.T, mode string) *runner {
	t.Helper()
	r := &runner{dir: t.TempDir(), project: "hpbyoafake", stepTimeout: 200 * time.Millisecond,
		req: RunRequest{Profile: &Profile{ComposeFile: "docker-compose.yml", Env: []EnvDecl{{Name: "MAIL_API_TOKEN", Secret: true}}},
			Env: map[string]string{"MAIL_API_TOKEN": "test-secret-value"}}}
	if err := os.MkdirAll(filepath.Join(r.dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestComposeCommandHelper$", "--", mode)
	}
	return r
}

func TestComposeStepDeadlineAndLogs(t *testing.T) {
	for _, operation := range []string{"up", "down"} {
		t.Run(operation, func(t *testing.T) {
			r := fakeComposeRunner(t, "stall")
			ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
			defer cancel()
			start := time.Now()
			_, err := r.compose(ctx, composeEnvBase("HP_COMPOSE_HELPER=1"), operation)
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("step deadline exceeded: %s", elapsed)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("want wrapped deadline exceeded, got %v", err)
			}
			checkComposeError(t, err)
		})
	}
}

func checkComposeError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected Compose failure")
	}
	message := err.Error()
	if !strings.Contains(message, "last 60 lines") || !strings.Contains(message, "line-69") || strings.Contains(message, "line-00") {
		t.Fatalf("missing bounded Compose diagnostics: %s", message)
	}
	if strings.Contains(message, "test-secret-value") {
		t.Fatal("Compose error leaked a synthetic secret")
	}
}

func TestComposeFailureIncludesBoundedRedactedLogs(t *testing.T) {
	r := fakeComposeRunner(t, "fail")
	_, err := r.compose(context.Background(), composeEnvBase("HP_COMPOSE_HELPER=1"), "up")
	checkComposeError(t, err)
}
