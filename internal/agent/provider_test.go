package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// chatRouteFor builds a Router sending every phase to one chat endpoint.
func chatRouteFor(url string) *Router {
	route := PhaseRoute{ModelID: "wonder-test", BaseURL: url, APIKey: "chat-key", Provider: ProviderOpenAIChat}
	r := &Router{Default: route, PerPhase: map[string]PhaseRoute{}}
	for _, phase := range Phases {
		r.PerPhase[phase] = route
	}
	return r
}

func TestChatAdapterRequestResponseMapping(t *testing.T) {
	clearRoutingEnv(t)
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Errorf("authorization = %q, want Bearer scheme", Redact(got))
		}
		var m map[string]any
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Errorf("decode request: %v", err)
		}
		bodies = append(bodies, m)
		w.Header().Set("content-type", "application/json")
		if _, hasText := m["text"]; hasText {
			t.Error("chat request must not use Responses text.format")
		}
		if len(bodies) == 1 {
			msgs, _ := m["messages"].([]any)
			if len(msgs) != 2 {
				t.Errorf("first turn has %d messages, want system+user", len(msgs))
			}
			_, _ = w.Write([]byte(`{"id":"c1","model":"wonder-test","choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"list_files","arguments":"{\"path\":\".\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
			return
		}
		// Second turn must replay the assistant tool_calls and pair each
		// tool result with its tool_call_id.
		raw, _ := json.Marshal(m["messages"])
		if !strings.Contains(string(raw), "tool_calls") || !strings.Contains(string(raw), "c1") {
			t.Errorf("second turn does not replay tool pairing: %s", raw)
		}
		_, _ = w.Write([]byte(`{"id":"c2","model":"wonder-test","choices":[{"message":{"role":"assistant","content":"{\"dependencies\":[{\"id\":\"D-1\"}]}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`))
	}))
	defer srv.Close()

	ws := t.TempDir()
	if err := os.WriteFile(ws+"/a.txt", []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(Config{APIKey: "unused", ModelID: "unused", BaseURL: srv.URL}, testSpec(), &Executor{Workspace: ws})
	runner.Router = chatRouteFor(srv.URL)
	res, err := runner.RunPhase(context.Background(), "dependency_map", "map it")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.JSON), "D-1") {
		t.Errorf("result = %s", res.JSON)
	}
	if res.ModelID != "wonder-test" || res.Provider != ProviderOpenAIChat {
		t.Errorf("result records model=%q provider=%q", res.ModelID, res.Provider)
	}
	if res.Usage.Calls != 2 || res.Usage.TotalTokens != 17 {
		t.Errorf("usage = %+v, want 2 calls / 17 tokens", res.Usage)
	}
	if res.Latency <= 0 {
		t.Error("latency was not recorded")
	}
	if len(bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(bodies))
	}
	first := bodies[0]
	if first["model"] != "wonder-test" {
		t.Errorf("model = %v", first["model"])
	}
	rf, _ := first["response_format"].(map[string]any)
	inner, _ := rf["json_schema"].(map[string]any)
	if rf["type"] != "json_schema" || inner["name"] != "dependency_map" {
		t.Errorf("response_format = %v, want json_schema for the phase", first["response_format"])
	}
	tools, _ := first["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v, want the flat spec tool", first["tools"])
	}
	fn, _ := tools[0].(map[string]any)["function"].(map[string]any)
	if tools[0].(map[string]any)["type"] != "function" || fn["name"] != "list_files" {
		t.Errorf("tool mapping = %v", tools[0])
	}
	params, _ := fn["parameters"].(map[string]any)
	props, _ := params["properties"].(map[string]any)
	if path, _ := props["path"].(map[string]any); path["type"] != "string" {
		t.Errorf("tool parameters = %v", params)
	}
}

func TestChatToolsStripInvalidRequired(t *testing.T) {
	s := &Spec{Tools: []ToolDef{{
		Name:        "list_files",
		Description: "list",
		Parameters: map[string]any{
			"path":   map[string]any{"type": "string", "required": false},
			"filter": map[string]any{"type": "string", "required": []any{"path"}},
		},
	}}}
	tools := s.ChatTools()
	fn := tools[0]["function"].(map[string]any)
	props := fn["parameters"].(map[string]any)["properties"].(map[string]any)
	if _, has := props["path"].(map[string]any)["required"]; has {
		t.Errorf("boolean required flag leaked to the wire: %v", props)
	}
	resp := s.ResponsesTools()
	rprops := resp[0]["parameters"].(map[string]any)["properties"].(map[string]any)
	if _, has := rprops["path"].(map[string]any)["required"]; has {
		t.Errorf("boolean required flag leaked to the Responses wire: %v", rprops)
	}
}

func TestChatSchemaFailureFailsClosed(t *testing.T) {
	clearRoutingEnv(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","model":"wonder-test","choices":[{"message":{"role":"assistant","content":"{\"status\":\"nope\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()
	runner := NewRunner(Config{APIKey: "k", ModelID: "m", BaseURL: srv.URL}, testSpec(), &Executor{Workspace: t.TempDir()})
	runner.Router = chatRouteFor(srv.URL)
	_, err := runner.RunPhase(context.Background(), "intake_summary", "go")
	if err == nil || !strings.Contains(err.Error(), "schema validation failed") {
		t.Fatalf("err = %v, want schema failure", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 1 + 2 retries", calls)
	}
}

func TestChatAPIErrorNoRetryOn401(t *testing.T) {
	clearRoutingEnv(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","code":"invalid_api_key","message":"Unauthorized"}}`))
	}))
	defer srv.Close()
	runner := NewRunner(Config{APIKey: "bad", ModelID: "m", BaseURL: srv.URL}, testSpec(), nil)
	runner.Router = chatRouteFor(srv.URL)
	_, err := runner.RunPhase(context.Background(), "intake_summary", "go")
	if err == nil || !strings.Contains(err.Error(), "invalid_api_key") {
		t.Fatalf("err = %v, want auth failure", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want no retry on 401", calls)
	}
}
