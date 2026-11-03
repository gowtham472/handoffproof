package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rewriteHost sends every request to target while the route keeps its
// configured (possibly non-loopback) base URL.
type rewriteHost struct{ target *url.URL }

func (rt rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	c := req.Clone(req.Context())
	c.URL.Scheme = rt.target.Scheme
	c.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(c)
}

func TestIsLoopbackURL(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want bool
	}{
		{"http://127.0.0.1:11434/v1", true},
		{"http://localhost:8000/v1", true},
		{"http://[::1]:8000/v1", true},
		{"https://api.meta.ai/v1", false},
		{"http://10.0.0.5:11434/v1", false},
		{"http://localhost.evil.example/v1", false},
		{"::not a url", false},
	} {
		if got := IsLoopbackURL(tc.url); got != tc.want {
			t.Errorf("IsLoopbackURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestSourceLocalOnlyDeniesSourceOnRemoteRoute(t *testing.T) {
	clearRoutingEnv(t)
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "app.js"), []byte("secret-source"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		base     string
		wantDeny bool
	}{
		{"loopback route keeps source tools", "", false},
		{"remote route is denied source", "https://cloud.example/v1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var toolResult string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var m map[string]any
				if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
					t.Errorf("decode: %v", err)
				}
				w.Header().Set("content-type", "application/json")
				msgs, _ := m["messages"].([]any)
				if len(msgs) <= 2 {
					_, _ = w.Write([]byte(`{"id":"c1","model":"g","choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"t1","type":"function","function":{"name":"list_files","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
					return
				}
				last, _ := msgs[len(msgs)-1].(map[string]any)
				toolResult, _ = last["content"].(string)
				_, _ = w.Write([]byte(`{"id":"c2","model":"g","choices":[{"message":{"role":"assistant","content":"{\"dependencies\":[{\"id\":\"D-1\"}]}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			}))
			defer srv.Close()
			base := tc.base
			if base == "" {
				base = srv.URL
			}
			target, _ := url.Parse(srv.URL)
			cfg := Config{APIKey: "k", ModelID: "g", BaseURL: base, HTTP: &http.Client{Transport: rewriteHost{target}}}
			runner := NewRunner(cfg, testSpec(), &Executor{Workspace: ws})
			runner.Router = chatRouteFor(base)
			runner.SourceLocalOnly = true
			runner.Mode = "hybrid"
			res, err := runner.RunPhase(context.Background(), "dependency_map", "map it")
			if err != nil {
				t.Fatal(err)
			}
			denied := strings.Contains(toolResult, "source access denied")
			if denied != tc.wantDeny {
				t.Errorf("tool result = %q, want denied=%v", toolResult, tc.wantDeny)
			}
			if tc.wantDeny && strings.Contains(toolResult, "app.js") {
				t.Errorf("remote phase saw workspace listing: %q", toolResult)
			}
			if res.Mode != "hybrid" {
				t.Errorf("result mode = %q, want hybrid", res.Mode)
			}
			if runner.Exec.DenySource {
				t.Error("runner executor mutated; denial must apply to a per-phase copy")
			}
		})
	}
}
