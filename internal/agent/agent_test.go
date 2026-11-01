package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testSpec() *Spec {
	return &Spec{
		Name:            "test",
		SystemPrompt:    "SYS",
		MaxOutputTokens: 4096,
		Tools:           []ToolDef{{Name: "list_files", Description: "list", Parameters: map[string]any{"path": map[string]any{"type": "string"}}}},
		Schemas: map[string]json.RawMessage{
			"dependency_map": json.RawMessage(`{"type":"object","required":["dependencies"],"properties":{"dependencies":{"type":"array","items":{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}}}}`),
			"intake_summary": json.RawMessage(`{"type":"object","required":["status"],"properties":{"status":{"enum":["ok","blocked"]}}}`),
		},
	}
}

func writeReq(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		t.Errorf("decode request: %v", err)
	}
	return m
}

func TestLoopWithTools(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("authorization"))
		body := writeReq(t, r)
		if _, has := body["response_format"]; has {
			t.Error("Responses request must not use response_format")
		}
		if body["text"] == nil {
			t.Error("Responses request must set text.format")
		}
		raw, _ := json.Marshal(body)
		w.Header().Set("content-type", "application/json")
		if strings.Contains(string(raw), "function_call_output") {
			_, _ = w.Write([]byte(`{"id":"r2","model":"m","output":[{"type":"message","content":[{"type":"output_text","text":"{\"dependencies\":[{\"id\":\"D-1\"}]}"}]}],"usage":{"input_tokens":5,"output_tokens":7,"total_tokens":12}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"r1","model":"m","output":[{"type":"function_call","name":"list_files","arguments":"{\"path\":\".\"}","call_id":"c1"}],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`))
	}))
	defer srv.Close()

	cfg := Config{APIKey: "test-key", ModelID: "test-model", BaseURL: srv.URL}
	runner := NewRunner(cfg, testSpec(), &Executor{Workspace: ws})
	res, err := runner.RunPhase(context.Background(), "dependency_map", "map it")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.JSON), "D-1") {
		t.Errorf("result = %s", res.JSON)
	}
	if res.Usage.Calls != 2 || res.Usage.TotalTokens != 17 {
		t.Errorf("usage = %+v, want 2 calls / 17 tokens", res.Usage)
	}
	if len(auths) != 2 || auths[0] != "Bearer test-key" {
		t.Errorf("auth headers = %v", auths)
	}
}

func TestSchemaRetryThenFailClosed(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"r","model":"m","output":[{"type":"message","content":[{"type":"output_text","text":"{\"status\":\"nope\"}"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()
	cfg := Config{APIKey: "k", ModelID: "m", BaseURL: srv.URL}
	runner := NewRunner(cfg, testSpec(), &Executor{Workspace: t.TempDir()})
	_, err := runner.RunPhase(context.Background(), "intake_summary", "go")
	if err == nil || !strings.Contains(err.Error(), "schema validation failed") {
		t.Fatalf("err = %v, want schema failure", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 1 + 2 retries", calls)
	}
}

func TestAPIErrorNoRetryOn401(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","code":"invalid_api_key","message":"Unauthorized"}}`))
	}))
	defer srv.Close()
	cfg := Config{APIKey: "bad", ModelID: "m", BaseURL: srv.URL}
	runner := NewRunner(cfg, testSpec(), nil)
	_, err := runner.RunPhase(context.Background(), "intake_summary", "go")
	var apiErr *APIError
	if err == nil || !strings.Contains(err.Error(), "invalid_api_key") {
		t.Fatalf("err = %v", err)
	}
	_ = apiErr
	if calls != 1 {
		t.Errorf("calls = %d, want no retry on 401", calls)
	}
}

func TestRetryOn429(t *testing.T) {
	retryBackoff = time.Millisecond
	defer func() { retryBackoff = time.Second }()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"slow down"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"r","model":"m","output":[{"type":"message","content":[{"type":"output_text","text":"{\"status\":\"ok\"}"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()
	cfg := Config{APIKey: "k", ModelID: "m", BaseURL: srv.URL}
	runner := NewRunner(cfg, testSpec(), nil)
	res, err := runner.RunPhase(context.Background(), "intake_summary", "go")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(string(res.JSON), `"ok"`) {
		t.Errorf("calls=%d res=%s", calls, res.JSON)
	}
}

func TestValidatorAndRedact(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","required":["a"],"properties":{"a":{"type":"number","minimum":0,"maximum":0.9},"b":{"type":["string","null"]},"c":{"const":false}}}`)
	if probs := ValidateSchema(schema, map[string]any{"a": 0.5, "b": nil, "c": false}); len(probs) != 0 {
		t.Errorf("valid value flagged: %v", probs)
	}
	if probs := ValidateSchema(schema, map[string]any{"a": 1.5, "c": true}); len(probs) == 0 {
		t.Error("invalid value passed validation")
	}
	out := Redact("Authorization: Bearer abcdefgh12345678 and password=supersecret99")
	if strings.Contains(out, "abcdefgh") || strings.Contains(out, "supersecret") {
		t.Errorf("redact leaked: %s", out)
	}
	if !strings.Contains(out, "[REDACTED:") {
		t.Errorf("redact marker missing: %s", out)
	}
}
