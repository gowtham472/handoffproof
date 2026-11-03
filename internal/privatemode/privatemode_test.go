package privatemode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gowtham472/handoffproof/internal/agent"
)

// recorder sees every request that reaches the network layer.
type recorder struct {
	mu    sync.Mutex
	hosts []string
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.hosts = append(r.hosts, req.URL.Hostname())
	r.mu.Unlock()
	return http.DefaultTransport.RoundTrip(req)
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvMode, "HANDOFF_GLIMMER_BASE_URL", "HANDOFF_GLIMMER_MODEL", "HANDOFF_GLIMMER_API_KEY",
		"META_API_KEY", "MODEL_API_KEY", "META_MODEL_ID", "META_API_BASE_URL"} {
		t.Setenv(k, "")
	}
	for _, short := range []string{"INTAKE", "DEPENDENCY", "DIAGNOSIS", "REPAIR", "ASSESSMENT", "VERIFICATION"} {
		for _, p := range []string{"HANDOFF_MODEL_", "HANDOFF_PROVIDER_", "HANDOFF_BASE_URL_", "HANDOFF_KEY_ENV_"} {
			t.Setenv(p+short, "")
		}
	}
}

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    Mode
		wantErr bool
	}{
		{"", Off, false},
		{"off", Off, false},
		{"LOCAL", Local, false},
		{" hybrid ", Hybrid, false},
		{"cloud", Off, true},
	} {
		got, err := Parse(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("Parse(%q) = %q, %v; want %q, err=%v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestLocalRoute(t *testing.T) {
	clearEnv(t)
	r, err := LocalRoute()
	if err != nil {
		t.Fatal(err)
	}
	if r.BaseURL != DefaultBaseURL || r.ModelID != DefaultModelID || r.Provider != agent.ProviderOpenAIChat || r.APIKey == "" {
		t.Errorf("default route = %+v", r)
	}
	t.Setenv("HANDOFF_GLIMMER_BASE_URL", "https://api.meta.ai/v1")
	if _, err := LocalRoute(); err == nil || !strings.Contains(err.Error(), "not a loopback") {
		t.Errorf("remote glimmer URL accepted: %v", err)
	}
}

func TestApply(t *testing.T) {
	cloud := agent.PhaseRoute{ModelID: "muse-spark-1.3", BaseURL: "https://api.meta.ai/v1", APIKey: "k", Provider: agent.ProviderMetaResponses}
	local := agent.PhaseRoute{ModelID: DefaultModelID, BaseURL: DefaultBaseURL, APIKey: "local", Provider: agent.ProviderOpenAIChat}
	newRouter := func() *agent.Router {
		r := &agent.Router{Default: cloud, PerPhase: map[string]agent.PhaseRoute{}}
		for _, p := range agent.Phases {
			r.PerPhase[p] = cloud
		}
		return r
	}
	isLocal := map[string]bool{}
	for _, p := range HybridLocalPhases {
		isLocal[p] = true
	}
	for _, tc := range []struct {
		mode      Mode
		wantLocal func(phase string) bool
	}{
		{Off, func(string) bool { return false }},
		{Local, func(string) bool { return true }},
		{Hybrid, func(p string) bool { return isLocal[p] }},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			r := newRouter()
			if err := Apply(r, tc.mode, local); err != nil {
				t.Fatal(err)
			}
			for _, p := range agent.Phases {
				got := r.PerPhase[p].BaseURL == local.BaseURL
				if got != tc.wantLocal(p) {
					t.Errorf("phase %s local=%v, want %v", p, got, tc.wantLocal(p))
				}
			}
		})
	}
	if err := Apply(newRouter(), Local, cloud); err == nil {
		t.Error("local mode with a remote 'local' route must fail closed")
	}
}

func TestGuardRefusesNonLoopback(t *testing.T) {
	rec := &recorder{}
	var blocked []string
	hc := &http.Client{Transport: Guard(rec, func(h string) { blocked = append(blocked, h) })}
	_, err := hc.Get("https://api.meta.ai/v1/responses")
	if err == nil || !strings.Contains(err.Error(), "egress to api.meta.ai refused") {
		t.Fatalf("err = %v, want refusal", err)
	}
	if len(rec.hosts) != 0 {
		t.Errorf("refused request reached the network layer: %v", rec.hosts)
	}
	if len(blocked) != 1 || blocked[0] != "api.meta.ai" {
		t.Errorf("blocked = %v", blocked)
	}
}

// TestLocalModeNoEgressToMetaAPI is the F5 network-egress acceptance test:
// with the cloud configured everywhere it can be (process default and a
// per-phase repair override), local mode runs all six phases against the
// local endpoint and no request ever reaches api.meta.ai.
func TestLocalModeNoEgressToMetaAPI(t *testing.T) {
	clearEnv(t)
	var paths []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		var m map[string]any
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Errorf("decode: %v", err)
		}
		if m["model"] != "muse-glimmer-test" {
			t.Errorf("model = %v, want the local Glimmer model", m["model"])
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","model":"muse-glimmer-test","choices":[{"message":{"role":"assistant","content":"{\"ok\":true}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	t.Setenv(EnvMode, "local")
	t.Setenv("META_API_BASE_URL", "https://api.meta.ai/v1")
	t.Setenv("META_MODEL_ID", "muse-spark-1.3")
	t.Setenv("HANDOFF_MODEL_REPAIR", "muse-wonder-1.0")
	t.Setenv("HANDOFF_BASE_URL_REPAIR", "https://api.meta.ai/v1")
	t.Setenv("HANDOFF_GLIMMER_BASE_URL", srv.URL)
	t.Setenv("HANDOFF_GLIMMER_MODEL", "muse-glimmer-test")

	mode, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	local, err := LocalRoute()
	if err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	var blocked []string
	cfg := agent.ConfigFromEnv()
	cfg.HTTP = &http.Client{Transport: rec}
	cfg = Configure(cfg, mode, local, func(h string) { blocked = append(blocked, h) })
	if err := cfg.Validate(); err != nil {
		t.Fatalf("local mode must not need a Meta API key: %v", err)
	}
	router, err := agent.RouterFromEnv(cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema := json.RawMessage(`{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}`)
	spec := &agent.Spec{Name: "t", SystemPrompt: "SYS", MaxOutputTokens: 256, Schemas: map[string]json.RawMessage{}}
	for _, p := range agent.Phases {
		spec.Schemas[p] = schema
	}
	runner := agent.NewRunner(cfg, spec, &agent.Executor{Workspace: t.TempDir()})
	if err := Wire(runner, &router, mode, local); err != nil {
		t.Fatal(err)
	}
	for _, p := range agent.Phases {
		res, err := runner.RunPhase(context.Background(), p, "go")
		if err != nil {
			t.Fatalf("phase %s: %v", p, err)
		}
		if res.Mode != "local" || res.Provider != agent.ProviderOpenAIChat {
			t.Errorf("phase %s result mode=%q provider=%q", p, res.Mode, res.Provider)
		}
	}
	if len(paths) != len(agent.Phases) {
		t.Errorf("local endpoint got %d requests, want %d", len(paths), len(agent.Phases))
	}
	// A stray cloud call through the same client is refused before dialing.
	if _, err := cfg.HTTP.Get("https://api.meta.ai/v1/responses"); err == nil {
		t.Error("stray api.meta.ai request was not refused")
	}
	for _, h := range rec.hosts {
		if h == "api.meta.ai" || !agent.IsLoopbackHost(h) {
			t.Errorf("request reached non-loopback host %s", h)
		}
	}
	if len(blocked) != 1 || blocked[0] != "api.meta.ai" {
		t.Errorf("blocked = %v, want only the stray probe", blocked)
	}
}
