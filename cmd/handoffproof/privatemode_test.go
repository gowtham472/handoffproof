package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gowtham472/handoffproof/internal/agent"
	"github.com/gowtham472/handoffproof/internal/privatemode"
)

func TestPrivateModeStatus(t *testing.T) {
	for _, tc := range []struct {
		mode       string
		wantCode   int
		wantMode   string
		wantPhases int
	}{
		{"", 200, "off", 0},
		{"local", 200, "local", len(agent.Phases)},
		{"hybrid", 200, "hybrid", len(privatemode.HybridLocalPhases)},
		{"cloud", 500, "", 0},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv(privatemode.EnvMode, tc.mode)
			t.Setenv("HANDOFF_GLIMMER_BASE_URL", "")
			t.Setenv("HANDOFF_GLIMMER_API_KEY", "glimmer-secret-value")
			w := httptest.NewRecorder()
			privateModeStatus(w, httptest.NewRequest(http.MethodGet, "/api/private-mode", nil))
			if w.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d: %s", w.Code, tc.wantCode, w.Body)
			}
			if strings.Contains(w.Body.String(), "glimmer-secret-value") {
				t.Error("status leaks the local API key")
			}
			if tc.wantCode != 200 {
				return
			}
			var got struct {
				Mode         string   `json:"mode"`
				LocalPhases  []string `json:"local_phases"`
				EndpointHost string   `json:"endpoint_host"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Mode != tc.wantMode || len(got.LocalPhases) != tc.wantPhases {
				t.Errorf("status = %+v", got)
			}
			if tc.wantMode != "off" && got.EndpointHost != "127.0.0.1:11434" {
				t.Errorf("endpoint_host = %q", got.EndpointHost)
			}
		})
	}
}

func TestLogPhaseFormatRecordsMode(t *testing.T) {
	for _, tc := range []struct{ mode, want string }{{"", "mode=off"}, {"local", "mode=local"}, {"hybrid", "mode=hybrid"}} {
		line := logPhaseFormat("intake_summary", &agent.PhaseResult{ModelID: "m", Provider: "p", Mode: tc.mode, Latency: time.Second,
			Usage: agent.UsageTotal{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}})
		if !strings.Contains(line, tc.want) {
			t.Errorf("line %q lacks %s", line, tc.want)
		}
		if m := phaseHeaderRe.FindStringSubmatch(line); m == nil || m[1] != "intake_summary" || m[4] != "3" {
			t.Errorf("eval header regex does not parse %q: %v", line, m)
		}
	}
}

func TestRepairScopedRunnerKeepsPrivateMode(t *testing.T) {
	guard := privatemode.Guard(nil, nil)
	r := agent.NewRunner(agent.Config{HTTP: &http.Client{Transport: guard}}, &agent.Spec{}, &agent.Executor{})
	r.Mode = "hybrid"
	r.SourceLocalOnly = true
	out := repairScopedRunner(r)
	if out.Cfg.HTTP.Transport != guard {
		t.Error("repair runner dropped the private-mode egress guard")
	}
	if out.Mode != "hybrid" || !out.SourceLocalOnly {
		t.Errorf("repair runner mode=%q sourceLocalOnly=%v", out.Mode, out.SourceLocalOnly)
	}
}

func TestInterviewLocalModeRefusesCloudTranscription(t *testing.T) {
	t.Setenv(privatemode.EnvMode, "local")
	t.Setenv("HANDOFF_GLIMMER_BASE_URL", "")
	ws := writeInterviewWorkspace(t)
	err := cmdInterview(context.Background(), t.TempDir(), []string{
		"--app-dir", ws, "--evidence-dir", t.TempDir(),
		"--audio", filepath.Join("..", "..", "eval", "fixtures", "interview-sample.wav"),
	})
	if err == nil || !strings.Contains(err.Error(), "private mode local") {
		t.Errorf("err = %v, want local-mode refusal before any cloud call", err)
	}
}
