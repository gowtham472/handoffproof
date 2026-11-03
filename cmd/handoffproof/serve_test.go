package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testServer(t *testing.T) *server {
	t.Helper()
	return &server{
		root:      t.TempDir(),
		tokenHash: hashToken("test-console-token-1"),
		jobs:      map[string]*job{},
		jobDone:   map[string]time.Time{},
		rlHits:    map[string][]time.Time{},
	}
}

func TestConsoleRequiresAuth(t *testing.T) {
	s := testServer(t)
	h := s.routes()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated / = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/summary", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/summary = %d, want 401", rec.Code)
	}
}

func TestConsoleTokenSetsSessionCookie(t *testing.T) {
	s := testServer(t)
	h := s.routes()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?token=test-console-token-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("token / = %d, want 200", rec.Code)
	}
	cookie := ""
	for _, c := range rec.Result().Cookies() {
		if c.Name == "hp_session" {
			cookie = c.Value
			if !c.HttpOnly {
				t.Fatal("session cookie must be HttpOnly")
			}
		}
	}
	if cookie == "" {
		t.Fatal("no hp_session cookie set")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/summary", nil)
	req.AddCookie(&http.Cookie{Name: "hp_session", Value: cookie})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie /api/summary = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff header")
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("missing CSP frame-ancestors")
	}
}

func TestConsoleCSRFAndDrillValidation(t *testing.T) {
	s := testServer(t)
	h := s.routes()
	auth := &http.Cookie{Name: "hp_session", Value: s.tokenHash}

	// POST without the custom header is rejected before the handler.
	req := httptest.NewRequest(http.MethodPost, "/api/drills/run", strings.NewReader(`{"drill":"nope"}`))
	req.AddCookie(auth)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST without X-HP-Request = %d, want 403", rec.Code)
	}

	// With the header, the handler runs and rejects the unknown drill.
	req = httptest.NewRequest(http.MethodPost, "/api/drills/run", strings.NewReader(`{"drill":"nope"}`))
	req.AddCookie(auth)
	req.Header.Set("X-HP-Request", "1")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST unknown drill = %d, want 400", rec.Code)
	}

	// Without auth, even a well-formed POST is unauthorized.
	req = httptest.NewRequest(http.MethodPost, "/api/drills/run", strings.NewReader(`{"drill":"nope"}`))
	req.Header.Set("X-HP-Request", "1")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST = %d, want 401", rec.Code)
	}
}

func TestIllustrations(t *testing.T) {
	s := testServer(t)
	h := s.routes()
	for _, name := range []string{"handover.svg", "rehearse.svg", "evidence.svg", "insights.svg", "about.svg"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/assets/illustrations/"+name, nil)
		req.AddCookie(&http.Cookie{Name: "hp_session", Value: s.tokenHash})
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("illustration %s = %d, want 200", name, rec.Code)
		}
		if ct := rec.Header().Get("content-type"); ct != "image/svg+xml" {
			t.Fatalf("illustration %s content-type = %q", name, ct)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/illustrations/../../serve.go", nil)
	req.AddCookie(&http.Cookie{Name: "hp_session", Value: s.tokenHash})
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("traversal illustration returned 200 with protected content")
	}
}

func TestSlackIntegration(t *testing.T) {
	s := testServer(t)
	h := s.routes()
	auth := &http.Cookie{Name: "hp_session", Value: s.tokenHash}
	t.Setenv("HANDOFF_SLACK_STORE", filepath.Join(t.TempDir(), "slack-connection.json"))
	t.Setenv("HANDOFF_SLACK_ENCRYPTION_KEY", "test-slack-store-key")
	t.Setenv("HANDOFF_SLACK_CLIENT_ID", "")
	t.Setenv("HANDOFF_SLACK_CLIENT_SECRET", "")
	t.Setenv("HANDOFF_PUBLIC_URL", "")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/integrations/slack", nil)
	req.AddCookie(auth)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"connected":false`) {
		t.Fatalf("slack status = %d %s, want connected:false", rec.Code, rec.Body.String())
	}

	t.Setenv("HANDOFF_SLACK_WEBHOOK", "")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/integrations/slack", strings.NewReader(`{}`))
	req.AddCookie(auth)
	req.Header.Set("X-HP-Request", "1")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("slack post without webhook = %d, want 400", rec.Code)
	}

	var got struct {
		Text string `json:"text"`
	}
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(200)
	}))
	defer hook.Close()
	t.Setenv("HANDOFF_SLACK_WEBHOOK", hook.URL)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/integrations/slack", strings.NewReader(`{"text":"ignore me"}`))
	req.AddCookie(auth)
	req.Header.Set("X-HP-Request", "1")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("slack post = %d %s, want 200", rec.Code, rec.Body.String())
	}
	if !strings.Contains(got.Text, "HandoffProof report:") {
		t.Fatalf("slack payload not server-built: %q", got.Text)
	}
	if strings.Contains(got.Text, "test-console-token") {
		t.Fatal("slack payload must never carry the console token")
	}
}

func TestSlackOAuthFlow(t *testing.T) {
	s := testServer(t)
	h := s.routes()
	auth := &http.Cookie{Name: "hp_session", Value: s.tokenHash}
	storePath := filepath.Join(t.TempDir(), "slack-connection.json")
	t.Setenv("HANDOFF_SLACK_STORE", storePath)
	t.Setenv("HANDOFF_SLACK_ENCRYPTION_KEY", "test-slack-store-key")
	t.Setenv("HANDOFF_SLACK_CLIENT_ID", "client-123")
	t.Setenv("HANDOFF_SLACK_CLIENT_SECRET", "client-secret-value")
	t.Setenv("HANDOFF_PUBLIC_URL", "https://console.example.test")
	t.Setenv("HANDOFF_SLACK_WEBHOOK", "")

	fakeSlack := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth.v2.access" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.Form.Get("code") != "code-123" || r.Form.Get("client_id") != "client-123" || r.Form.Get("redirect_uri") != "https://console.example.test/api/integrations/slack/callback" {
			t.Errorf("oauth exchange form = %v", r.Form)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"access_token":"xoxb-discard","team":{"id":"T123","name":"Example Workspace"},"incoming_webhook":{"channel":"#alerts","channel_id":"C123","url":"https://hooks.slack.com/services/T123/B123/webhook-secret"}}`))
	}))
	defer fakeSlack.Close()
	t.Setenv("HANDOFF_SLACK_BASE_URL", fakeSlack.URL)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/integrations/slack/connect", nil)
	req.AddCookie(auth)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("connect = %d %s, want 302", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || !strings.HasPrefix(rec.Header().Get("Location"), fakeSlack.URL+"/oauth/v2/authorize") || loc.Query().Get("scope") != "incoming-webhook" {
		t.Fatalf("connect location = %q", rec.Header().Get("Location"))
	}
	state := loc.Query().Get("state")
	var stateCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "hp_slack_state" {
			stateCookie = c
		}
	}
	if state == "" || stateCookie == nil || stateCookie.Value != state || !stateCookie.HttpOnly {
		t.Fatalf("state/cookie = %q/%v", state, stateCookie)
	}

	callback := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/integrations/slack/callback?code=code-123&state="+url.QueryEscape(state), nil)
		req.AddCookie(stateCookie)
		h.ServeHTTP(rec, req)
		return rec
	}
	rec = callback()
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/?slack=connected#evidence" {
		t.Fatalf("callback = %d loc %q body %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	raw, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "webhook-secret") || strings.Contains(string(raw), "client-secret-value") || strings.Contains(string(raw), "xoxb-discard") {
		t.Fatal("stored Slack file leaked a secret")
	}
	conn, ok, err := s.loadSlackConnection()
	if err != nil || !ok || conn.Webhook != "https://hooks.slack.com/services/T123/B123/webhook-secret" || conn.Channel != "#alerts" {
		t.Fatalf("stored connection = %+v ok=%v err=%v", conn, ok, err)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/integrations/slack", nil)
	req.AddCookie(auth)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"connected":true`) || !strings.Contains(rec.Body.String(), `"channel":"#alerts"`) {
		t.Fatalf("oauth status = %d %s", rec.Code, rec.Body.String())
	}
	if rec = callback(); rec.Code != http.StatusForbidden {
		t.Fatalf("replayed callback = %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/integrations/slack", nil)
	req.AddCookie(auth)
	req.Header.Set("X-HP-Request", "1")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disconnect = %d %s, want 200", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(storePath); !os.IsNotExist(err) {
		t.Fatalf("store after disconnect err = %v, want not exist", err)
	}
}

func TestGitHubOAuthFlow(t *testing.T) {
	s := testServer(t)
	h := s.routes()
	auth := &http.Cookie{Name: "hp_session", Value: s.tokenHash}
	storePath := filepath.Join(t.TempDir(), "github-connection.json")
	t.Setenv("HANDOFF_GITHUB_STORE", storePath)
	t.Setenv("HANDOFF_GITHUB_ENCRYPTION_KEY", "test-github-store-key")
	t.Setenv("HANDOFF_GITHUB_CLIENT_ID", "gh-client")
	t.Setenv("HANDOFF_GITHUB_CLIENT_SECRET", "gh-secret-value")
	t.Setenv("HANDOFF_PUBLIC_URL", "https://console.example.test")

	fakeGitHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/login/oauth/access_token":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode token request: %v", err)
			}
			if body["code"] != "gh-code" || body["client_id"] != "gh-client" || body["code_verifier"] == "" || body["redirect_uri"] != "https://console.example.test/api/integrations/github/callback" {
				t.Errorf("github token request = %v", body)
			}
			_, _ = w.Write([]byte(`{"access_token":"github-token-secret","refresh_token":"github-refresh-secret","scope":"repo","token_type":"bearer"}`))
		case "/user":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || len(r.Header.Get("Authorization")) < 20 {
				t.Errorf("github auth header missing bearer token")
			}
			_, _ = w.Write([]byte(`{"login":"octocat","name":"Octo Cat","html_url":"https://github.com/octocat"}`))
		case "/user/repos":
			_, _ = w.Write([]byte(`[{"full_name":"octocat/handoff","private":false,"default_branch":"main","html_url":"https://github.com/octocat/handoff"}]`))
		case "/repos/octocat/handoff":
			_, _ = w.Write([]byte(`{"full_name":"octocat/handoff","private":false,"default_branch":"main","html_url":"https://github.com/octocat/handoff"}`))
		case "/repos/octocat/handoff/branches/main":
			_, _ = w.Write([]byte(`{"name":"main"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer fakeGitHub.Close()
	t.Setenv("HANDOFF_GITHUB_BASE_URL", fakeGitHub.URL)
	t.Setenv("HANDOFF_GITHUB_API_BASE_URL", fakeGitHub.URL)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/integrations/github/connect", nil)
	req.AddCookie(auth)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("github connect = %d %s, want 302", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || !strings.HasPrefix(rec.Header().Get("Location"), fakeGitHub.URL+"/login/oauth/authorize") || loc.Query().Get("scope") != "repo" || loc.Query().Get("code_challenge") == "" || loc.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("github connect location = %q", rec.Header().Get("Location"))
	}
	state := loc.Query().Get("state")
	var stateCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "hp_github_state" {
			stateCookie = c
		}
	}
	if state == "" || stateCookie == nil || stateCookie.Value != state || !stateCookie.HttpOnly {
		t.Fatalf("github state/cookie = %q/%v", state, stateCookie)
	}
	callback := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/integrations/github/callback?code=gh-code&state="+url.QueryEscape(state), nil)
		req.AddCookie(stateCookie)
		h.ServeHTTP(rec, req)
		return rec
	}
	rec = callback()
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/?github=connected#settings" {
		t.Fatalf("github callback = %d loc %q body %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	raw, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "github-token-secret") || strings.Contains(string(raw), "github-refresh-secret") || strings.Contains(string(raw), "gh-secret-value") {
		t.Fatal("stored GitHub file leaked a secret")
	}
	conn, tokens, ok := s.currentGitHubConnection()
	if !ok || conn.Login != "octocat" || tokens.AccessToken != "github-token-secret" {
		t.Fatalf("stored github connection = %+v ok=%v", conn, ok)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/integrations/github/repositories", nil)
	req.AddCookie(auth)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "octocat/handoff") {
		t.Fatalf("github repos = %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/integrations/github/repository", strings.NewReader(`{"repository":"octocat/handoff","branch":""}`))
	req.AddCookie(auth)
	req.Header.Set("X-HP-Request", "1")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"branch":"main"`) {
		t.Fatalf("github repo save = %d %s", rec.Code, rec.Body.String())
	}
	if rec = callback(); rec.Code != http.StatusFound || rec.Header().Get("Location") != "/?github=error#settings" {
		t.Fatalf("replayed github callback = %d loc %q, want error redirect", rec.Code, rec.Header().Get("Location"))
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/integrations/github", nil)
	req.AddCookie(auth)
	req.Header.Set("X-HP-Request", "1")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("github disconnect = %d %s, want 200", rec.Code, rec.Body.String())
	}
}

func TestApplicationsOnboarding(t *testing.T) {
	s := testServer(t)
	h := s.routes()
	auth := &http.Cookie{Name: "hp_session", Value: s.tokenHash}
	t.Setenv("HANDOFF_APPLICATIONS_STORE", filepath.Join(t.TempDir(), "applications.json"))
	t.Setenv("HANDOFF_APPLICATIONS_ENCRYPTION_KEY", "test-applications-key")
	profileRaw, err := os.ReadFile(filepath.Join("..", "..", "internal", "byoa", "testdata", "miniapp", "handoffproof.json"))
	if err != nil {
		t.Fatal(err)
	}
	var profile map[string]any
	if err := json.Unmarshal(profileRaw, &profile); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"name": "Mini Python App", "source_kind": "local",
		"local_path": filepath.Join("..", "..", "internal", "byoa", "testdata", "miniapp"),
		"profile":    profile,
		"env_values": map[string]string{"MAIL_API_TOKEN": "test-token"},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/applications", strings.NewReader(string(body)))
	req.AddCookie(auth)
	req.Header.Set("X-HP-Request", "1")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create application = %d %s, want 201", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "test-token") {
		t.Fatal("application response leaked an environment value")
	}
	raw, err := os.ReadFile(os.Getenv("HANDOFF_APPLICATIONS_STORE"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "test-token") {
		t.Fatal("application store leaked an environment value in plaintext")
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/applications/miniapp/validate", nil)
	req.AddCookie(auth)
	req.Header.Set("X-HP-Request", "1")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("validate application = %d %s, want 200", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/applications/miniapp/scan", nil)
	req.AddCookie(auth)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("scan application = %d %s, want 200", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/applications/miniapp", nil)
	req.AddCookie(auth)
	req.Header.Set("X-HP-Request", "1")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete application = %d %s, want 200", rec.Code, rec.Body.String())
	}
}

func TestShareLinks(t *testing.T) {
	s := testServer(t)
	h := s.routes()
	auth := &http.Cookie{Name: "hp_session", Value: s.tokenHash}

	mint := func(t *testing.T, withAuth, withHeader bool) (int, string) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/share", strings.NewReader(`{}`))
		if withAuth {
			req.AddCookie(auth)
		}
		if withHeader {
			req.Header.Set("X-HP-Request", "1")
		}
		h.ServeHTTP(rec, req)
		var body struct {
			Share string `json:"share"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body.Share
	}
	if code, _ := mint(t, false, true); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated share mint = %d, want 401", code)
	}
	code, id := mint(t, true, true)
	if code != http.StatusOK || id == "" {
		t.Fatalf("share mint = %d id %q, want 200 with id", code, id)
	}

	// Share links open reads for everyone, by default, with no token typing.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/summary?s="+id, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("shared read = %d, want 200", rec.Code)
	}

	// Share links can never write.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/drills/run?s="+id, strings.NewReader(`{"drill":"x"}`))
	req.Header.Set("X-HP-Request", "1")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("shared write = %d, want 403", rec.Code)
	}

	// Unknown ids stay locked out.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/summary?s=deadbeef", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged share = %d, want 401", rec.Code)
	}
}

func TestShareLinksSurviveRestart(t *testing.T) {
	sharePath := filepath.Join(t.TempDir(), "shares.json")
	t.Setenv("HANDOFF_SHARE_FILE", sharePath)
	s1 := testServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/share", strings.NewReader(`{}`))
	req.AddCookie(&http.Cookie{Name: "hp_session", Value: s1.tokenHash})
	req.Header.Set("X-HP-Request", "1")
	s1.routes().ServeHTTP(rec, req)
	var body struct {
		Share string `json:"share"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Share == "" {
		t.Fatalf("mint: code=%d body=%s", rec.Code, rec.Body.String())
	}
	// A fresh server (restart) with the same share file still honors it.
	s2 := testServer(t)
	rec = httptest.NewRecorder()
	s2.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/summary?s="+body.Share, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("share after restart = %d, want 200", rec.Code)
	}
	// The store must not contain the raw share id.
	raw, err := os.ReadFile(sharePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), body.Share) {
		t.Fatal("share store must keep only the SHA-256 of the id")
	}
}

func TestRateLimiter(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/summary", nil)
	for i := 0; i < 3; i++ {
		if !s.allow(req, 3, time.Minute) {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if s.allow(req, 3, time.Minute) {
		t.Fatal("4th request in window should be limited")
	}
}
