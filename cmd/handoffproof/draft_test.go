package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gowtham472/handoffproof/internal/agent"
	"github.com/gowtham472/handoffproof/internal/draft"
	"github.com/gowtham472/handoffproof/internal/privatemode"
)

const miniappDir = "../../internal/byoa/testdata/miniapp"

// fakeDraftServer answers every Responses turn with the given draft text and
// records the tool names offered to the model.
func fakeDraftServer(t *testing.T, draftText string, tools *[]string) *httptest.Server {
	return fakeDraftSeq(t, []string{draftText}, tools, nil)
}

// fakeDraftSeq replies with drafts in order (repeating the last), records
// tool names, and keeps each raw request body when bodies is non-nil.
func fakeDraftSeq(t *testing.T, drafts []string, tools *[]string, bodies *[]string) *httptest.Server {
	n := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		draftText := drafts[min(n, len(drafts)-1)]
		n++
		raw, _ := io.ReadAll(r.Body)
		if bodies != nil {
			*bodies = append(*bodies, string(raw))
		}
		var m struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Errorf("decode: %v", err)
		}
		for _, tl := range m.Tools {
			*tools = append(*tools, tl.Name)
		}
		text, _ := json.Marshal(draftText)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"id":"r","model":"muse-wonder-1.0","output":[{"type":"message","content":[{"type":"output_text","text":` + string(text) + `}]}],"usage":{"input_tokens":5,"output_tokens":5,"total_tokens":10}}`))
	}))
}

func TestDraftOnceOffline(t *testing.T) {
	ref, err := os.ReadFile(filepath.Join(miniappDir, "handoffproof.json"))
	if err != nil {
		t.Fatal(err)
	}
	withLiteral := strings.Replace(string(ref), "${MAIL_API_TOKEN}", "test-token-value", 1)
	withExtra := strings.Replace(string(ref), `"version": 1,`, `"version": 1, "notes": "x",`, 1)
	spec, err := agent.LoadSpec(filepath.Join("..", "..", "prompts", "f6_profile_draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"MAIL_API_TOKEN": "test-token-value"}
	for _, tc := range []struct {
		name          string
		reply         string
		wantRejection string
		wantInvalid   string
	}{
		{"reference-shaped draft is valid but unshown without baseline", string(ref), "", ""},
		{"literal credential value is rejected", withLiteral, "value of MAIL_API_TOKEN", ""},
		{"unknown field fails strict decode", withExtra, "", "unknown field"},
		{"not JSON fails closed after retries", "not json", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var tools []string
			srv := fakeDraftServer(t, tc.reply, &tools)
			defer srv.Close()
			cfg := agent.Config{APIKey: "k", ModelID: "muse-wonder-1.0", BaseURL: srv.URL}
			route := agent.PhaseRoute{ModelID: "muse-wonder-1.0", BaseURL: srv.URL, APIKey: "k", Provider: agent.ProviderMetaResponses}
			res := draftOnce(context.Background(), t.TempDir(), cfg, route, privatemode.Off, spec, miniappDir, env, false)
			if res.Shown {
				t.Error("a draft without a passing baseline was marked shown")
			}
			for _, name := range tools {
				if name == "submit_repair" || name == "request_drill" {
					t.Errorf("drafter was offered write tool %s", name)
				}
			}
			if strings.Contains(string(res.Draft), "test-token-value") {
				t.Error("recorded draft keeps the literal credential")
			}
			switch {
			case tc.reply == "not json":
				if !strings.Contains(res.ModelError, "schema validation failed") {
					t.Errorf("model error = %q", res.ModelError)
				}
			case tc.wantRejection != "":
				if !strings.Contains(strings.Join(res.Rejections, ";"), tc.wantRejection) {
					t.Errorf("rejections = %v", res.Rejections)
				}
			case tc.wantInvalid != "":
				if !strings.Contains(res.ValidationError, tc.wantInvalid) || res.Attempts != draftAttempts {
					t.Errorf("validation error = %q after %d attempts", res.ValidationError, res.Attempts)
				}
			default:
				if res.ModelError != "" || res.ValidationError != "" || len(res.Rejections) > 0 || res.BaselineRan ||
					!res.FirstAttemptValid || res.Attempts != 1 {
					t.Errorf("result = %+v", res)
				}
			}
		})
	}
}

func TestDraftValidatorFeedbackRetry(t *testing.T) {
	ref, err := os.ReadFile(filepath.Join(miniappDir, "handoffproof.json"))
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(ref), `"id": "miniapp"`, `"id": "Mini App"`, 1)
	spec, err := agent.LoadSpec(filepath.Join("..", "..", "prompts", "f6_profile_draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tools, bodies []string
	srv := fakeDraftSeq(t, []string{bad, string(ref)}, &tools, &bodies)
	defer srv.Close()
	cfg := agent.Config{APIKey: "k", ModelID: "muse-wonder-1.0", BaseURL: srv.URL}
	route := agent.PhaseRoute{ModelID: "muse-wonder-1.0", BaseURL: srv.URL, APIKey: "k", Provider: agent.ProviderMetaResponses}
	res := draftOnce(context.Background(), t.TempDir(), cfg, route, privatemode.Off, spec, miniappDir,
		map[string]string{"MAIL_API_TOKEN": "test-token-value"}, false)
	if res.Attempts != 2 || res.FirstAttemptValid || res.ValidationError != "" {
		t.Errorf("result = attempts %d first %v err %q", res.Attempts, res.FirstAttemptValid, res.ValidationError)
	}
	if len(bodies) != 2 || !strings.Contains(bodies[1], "lowercase slug") {
		t.Errorf("second request lacks validator feedback (%d requests)", len(bodies))
	}
	for _, b := range bodies {
		if strings.Contains(b, "test-token-value") {
			t.Error("request leaked a configured value")
		}
	}
}

func TestRedactedJSONKeepsStructure(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`{"env":[{"name":"MAIL_API_TOKEN","secret":true}],"h":"sk-abcdefghijklmnopqrstu"}`,
			`{"env":[{"name":"MAIL_API_TOKEN","secret":true}],"h":"[REDACTED:api-key]"}`},
		{`not json sk-abcdefghijklmnopqrstu`, `"not json [REDACTED:api-key]"`},
	} {
		if got := string(redactedJSON(tc.in)); got != tc.want {
			t.Errorf("redactedJSON(%s) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestDraftRoute(t *testing.T) {
	for _, tc := range []struct {
		name, override, want string
		wantErr              bool
	}{
		{"defaults to Wonder", "", draft.DefaultModelID, false},
		{"override honoured", "muse-spark-1.3", "muse-spark-1.3", false},
		{"contributor refused for imported apps", "muse-spark-1.3-contributor", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(privatemode.EnvMode, "")
			t.Setenv("META_API_KEY", "k")
			t.Setenv("META_MODEL_ID", "muse-spark-1.3")
			t.Setenv("HANDOFF_MODEL_DRAFT", tc.override)
			_, route, _, err := draftRoute()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if !tc.wantErr && route.ModelID != tc.want {
				t.Errorf("model = %q, want %q", route.ModelID, tc.want)
			}
		})
	}
}

func writeDraftResults(t *testing.T, dir string, results ...draft.Result) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, r := range results {
		raw, _ := json.Marshal(r)
		if err := os.WriteFile(filepath.Join(dir, "draft-"+r.App+"-2026110"+string(rune('1'+i))+".json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRenderDraftSectionAndReplace(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "evidence", "drafts")
	writeDraftResults(t, dir,
		draft.Result{App: "miniapp", ModelID: "muse-wonder-1.0", Shown: true, BaselineRan: true, BaselinePassed: true},
		draft.Result{App: "miniapp", ModelID: "muse-wonder-1.0", ValidationError: "draft is not a valid profile: unknown field"},
	)
	section := renderDraftSection(dir)
	if !strings.Contains(section, "| miniapp | muse-wonder-1.0 | 2 | 2 | 0 | 1 | 1/2 (50%) |") || !strings.Contains(section, "unknown field") {
		t.Errorf("section = %s", section)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	results := filepath.Join(root, "docs", "RESULTS.md")
	if err := os.WriteFile(results, []byte("# Results\n\nkeep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := replaceResultsSection(root, "f6", section); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(results)
	if strings.Count(string(raw), "<!-- f6:begin -->") != 1 || !strings.Contains(string(raw), "keep me") {
		t.Errorf("RESULTS.md = %s", raw)
	}
}

func TestDraftLatestServesOnlyShown(t *testing.T) {
	s := testServer(t)
	dir := filepath.Join(s.root, "evidence", "drafts")
	writeDraftResults(t, dir,
		draft.Result{App: "miniapp", ModelID: "m", Shown: true, Draft: json.RawMessage(`{"id":"good"}`), EvidenceIDs: []string{"EV-1"}},
		draft.Result{App: "miniapp", ModelID: "m", Shown: false, Draft: json.RawMessage(`{"id":"failed-later"}`)},
	)
	h := s.routes()
	rec := authedGet(s, h, "/api/drafts/latest?app=miniapp")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"good"`) || strings.Contains(rec.Body.String(), "failed-later") {
		t.Errorf("latest = %d %s", rec.Code, rec.Body)
	}
	if rec := authedGet(s, h, "/api/drafts/latest?app=other"); !strings.Contains(rec.Body.String(), `"found":false`) {
		t.Errorf("unknown app = %s", rec.Body)
	}
	if rec := authedGet(s, h, "/api/drafts/latest?app=../x"); rec.Code != 400 {
		t.Errorf("path-like app = %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/drafts/latest?app=miniapp", nil)
	unauth := httptest.NewRecorder()
	h.ServeHTTP(unauth, req)
	if unauth.Code == 200 {
		t.Error("draft served without operator auth")
	}
}
