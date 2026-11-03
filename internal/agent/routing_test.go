package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// clearRoutingEnv neutralizes every per-phase routing variable (and the
// test-only key vars) for the duration of a test.
func clearRoutingEnv(t *testing.T) {
	t.Helper()
	for _, phase := range Phases {
		short := phaseShort[phase]
		for _, prefix := range []string{"HANDOFF_MODEL_", "HANDOFF_PROVIDER_", "HANDOFF_BASE_URL_", "HANDOFF_KEY_ENV_"} {
			t.Setenv(prefix+short, "")
		}
	}
	t.Setenv("HANDOFF_TEST_KEY", "")
}

func testDefConfig() Config {
	return Config{APIKey: "def-key", ModelID: "muse-spark-1.3", BaseURL: "https://api.meta.ai/v1"}
}

func TestRouterDefaults(t *testing.T) {
	clearRoutingEnv(t)
	r, err := RouterFromEnv(testDefConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range Phases {
		route, err := r.ForPhase(phase)
		if err != nil {
			t.Fatalf("phase %s: %v", phase, err)
		}
		if route.ModelID != "muse-spark-1.3" || route.Provider != ProviderMetaResponses ||
			route.BaseURL != "https://api.meta.ai/v1" || route.APIKey != "def-key" {
			t.Errorf("phase %s default route = %+v, want process defaults", phase, route)
		}
	}
}

func TestRouterPerPhaseOverride(t *testing.T) {
	clearRoutingEnv(t)
	t.Setenv("HANDOFF_TEST_KEY", "together-key")
	t.Setenv("HANDOFF_MODEL_REPAIR", "muse-wonder-1.0")
	t.Setenv("HANDOFF_PROVIDER_REPAIR", ProviderOpenAIChat)
	t.Setenv("HANDOFF_BASE_URL_REPAIR", "https://api.together.xyz/v1/")
	t.Setenv("HANDOFF_KEY_ENV_REPAIR", "HANDOFF_TEST_KEY")
	r, err := RouterFromEnv(testDefConfig())
	if err != nil {
		t.Fatal(err)
	}
	route, err := r.ForPhase("repair_proposal")
	if err != nil {
		t.Fatal(err)
	}
	if route.ModelID != "muse-wonder-1.0" || route.Provider != ProviderOpenAIChat ||
		route.BaseURL != "https://api.together.xyz/v1" || route.APIKey != "together-key" {
		t.Errorf("repair route = %+v, want overrides (trailing slash trimmed)", route)
	}
	for _, phase := range Phases {
		if phase == "repair_proposal" {
			continue
		}
		route, err := r.ForPhase(phase)
		if err != nil {
			t.Fatalf("phase %s: %v", phase, err)
		}
		if route.ModelID != "muse-spark-1.3" || route.Provider != ProviderMetaResponses {
			t.Errorf("phase %s leaked override: %+v", phase, route)
		}
	}
}

func TestRouterUnknownProviderKind(t *testing.T) {
	clearRoutingEnv(t)
	for _, tc := range []struct {
		phase string
		short string
	}{
		{"diagnosis", "DIAGNOSIS"},
		{"repair_proposal", "REPAIR"},
	} {
		t.Run(tc.phase, func(t *testing.T) {
			t.Setenv("HANDOFF_PROVIDER_"+tc.short, "smoke-signals")
			if _, err := RouterFromEnv(testDefConfig()); err == nil ||
				!strings.Contains(err.Error(), "unknown provider kind") {
				t.Errorf("err = %v, want unknown provider kind failure", err)
			}
			t.Setenv("HANDOFF_PROVIDER_"+tc.short, "")
		})
	}
}

func TestRouterMissingKeyFailsClosed(t *testing.T) {
	clearRoutingEnv(t)
	t.Setenv("HANDOFF_KEY_ENV_INTAKE", "HANDOFF_DEFINITELY_UNSET_KEY")
	if _, err := RouterFromEnv(testDefConfig()); err == nil ||
		!strings.Contains(err.Error(), "API key is not set") {
		t.Errorf("err = %v, want fail-closed on unresolvable key", err)
	}
}

func TestRouterUnknownPhase(t *testing.T) {
	clearRoutingEnv(t)
	r, err := RouterFromEnv(testDefConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ForPhase("no_such_phase"); err == nil ||
		!strings.Contains(err.Error(), "unknown phase") {
		t.Errorf("err = %v, want unknown phase failure", err)
	}
}

func TestLegacyRunnerRecordsDefaults(t *testing.T) {
	clearRoutingEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("path = %s, want /responses", r.URL.Path)
		}
		body := writeReq(t, r)
		if _, has := body["response_format"]; has {
			t.Error("Responses request must not use response_format")
		}
		if body["text"] == nil {
			t.Error("Responses request must set text.format")
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"r","model":"m","output":[{"type":"message","content":[{"type":"output_text","text":"{\"status\":\"ok\"}"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()
	runner := NewRunner(Config{APIKey: "k", ModelID: "muse-spark-1.3", BaseURL: srv.URL}, testSpec(), nil)
	res, err := runner.RunPhase(context.Background(), "intake_summary", "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.ModelID != "muse-spark-1.3" || res.Provider != ProviderMetaResponses {
		t.Errorf("result records model=%q provider=%q", res.ModelID, res.Provider)
	}
}
