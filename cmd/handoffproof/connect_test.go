package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gowtham472/handoffproof/internal/evidence"
)

func operatorRequest(s *server, method, path, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "hp_session", Value: s.tokenHash})
	r.Header.Set("X-HP-Request", "1")
	return r
}

func TestSlackNotifications(t *testing.T) {
	s := testServer(t)
	t.Setenv("HANDOFF_SLACK_STORE", filepath.Join(t.TempDir(), "slack.json"))
	t.Setenv("HANDOFF_SLACK_ENCRYPTION_KEY", "test-key")
	var count atomic.Int32
	var status atomic.Int32
	status.Store(200)
	var text string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg struct {
			Text string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&msg)
		text = msg.Text
		count.Add(1)
		w.WriteHeader(int(status.Load()))
	}))
	defer hook.Close()
	t.Setenv("HANDOFF_SLACK_WEBHOOK", hook.URL)
	h := s.routes()
	put := func(body string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, operatorRequest(s, http.MethodPut, "/api/integrations/slack/notifications", body))
		return rec.Code
	}
	// Fixture records stand in for controller output, not a new measured drill.
	records := []evidence.Record{{ID: "EV-1234abcd", Assertion: "https://secret.test/ @channel xoxb-secret"}}
	s.notifySlack(records, nil)
	if count.Load() != 0 {
		t.Fatal("notifications must default off")
	}
	if code := put(`{"enabled":true,"failures_only":true}`); code != 200 {
		t.Fatalf("enable: %d", code)
	}
	// Settings survive a server restart.
	settings, err := testServer(t).notificationSettings()
	if err != nil || !settings.Enabled || !settings.FailuresOnly {
		t.Fatalf("persisted settings: %+v %v", settings, err)
	}
	s.notifySlack(nil, nil)
	if count.Load() != 0 {
		t.Fatal("failure-only filter posted a successful run")
	}
	s.notifySlack(records, nil)
	if count.Load() != 1 || !strings.Contains(text, "EV-1234abcd") || strings.Contains(text, "secret") || strings.Contains(text, "@channel") {
		t.Fatalf("unsafe or missing notification: %q", text)
	}
	status.Store(429)
	s.notifySlack(nil, fmt.Errorf("secret execution details"))
	if count.Load() != 2 || s.slackDelivery.Status != "error" || strings.Contains(s.slackDelivery.Error, hook.URL) {
		t.Fatal("delivery failure not isolated or redacted")
	}
	if records[0].Passed {
		t.Fatal("notification changed outcome")
	}
	if code := put(`{"enabled":true,"extra":true}`); code != 400 {
		t.Fatalf("unknown setting: %d", code)
	}
	if code := put(`{"enabled":false}`); code != 200 {
		t.Fatalf("disable: %d", code)
	}
	s.notifySlack(records, nil)
	if count.Load() != 2 {
		t.Fatal("disabled notifications posted")
	}
	// Shared report access never authorizes notification controls.
	r := httptest.NewRecorder()
	s.slackNotifications(r, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"enabled":true}`)))
	if r.Code != 403 {
		t.Fatalf("unprivileged settings: %d", r.Code)
	}
	// Do not silently reactivate automatic messages after reconnecting.
	if err := s.deleteSlackConnection(); err != nil {
		t.Fatal(err)
	}
	settings, err = s.notificationSettings()
	if err != nil || settings.Enabled {
		t.Fatal("disconnect retained automatic posting")
	}
}

func TestGitHubAppConnection(t *testing.T) {
	s := testServer(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HANDOFF_GITHUB_APP_ID", "123")
	t.Setenv("HANDOFF_GITHUB_APP_PRIVATE_KEY", string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})))
	t.Setenv("HANDOFF_GITHUB_STORE", filepath.Join(t.TempDir(), "github.json"))
	t.Setenv("HANDOFF_GITHUB_ENCRYPTION_KEY", "test-key")
	var minted atomic.Int32
	var expired, revoked atomic.Bool
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	entry, err := zw.Create("team-app/README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("synthetic application"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/app") {
			parts := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), ".")
			if len(parts) != 3 {
				t.Error("missing App JWT")
				w.WriteHeader(401)
				return
			}
			digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
			if rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig) != nil {
				t.Error("invalid JWT signature")
			}
			claimsRaw, _ := base64.RawURLEncoding.DecodeString(parts[1])
			var claims struct {
				Iat int64  `json:"iat"`
				Exp int64  `json:"exp"`
				Iss string `json:"iss"`
			}
			_ = json.Unmarshal(claimsRaw, &claims)
			if claims.Iss != "123" || claims.Iat > time.Now().Unix() || claims.Exp > time.Now().Add(10*time.Minute).Unix() {
				t.Error("invalid JWT claims")
			}
		} else if r.Header.Get("Authorization") != "Bearer installation-secret" {
			t.Error("missing installation token")
		}
		switch r.URL.Path {
		case "/app":
			_, _ = fmt.Fprint(w, `{"slug":"handoffproof"}`)
		case "/app/installations":
			_, _ = fmt.Fprint(w, `[{"id":42,"account":{"login":"team"}}]`)
		case "/app/installations/42":
			_, _ = fmt.Fprint(w, `{"id":42,"account":{"login":"team"}}`)
		case "/app/installations/42/access_tokens":
			if revoked.Load() {
				w.WriteHeader(403)
				return
			}
			if r.Method != http.MethodPost {
				t.Error("token request must POST")
			}
			var body struct {
				Permissions map[string]string `json:"permissions"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.Permissions) != 2 || body.Permissions["contents"] != "read" || body.Permissions["metadata"] != "read" {
				t.Error("token permissions too broad")
			}
			minted.Add(1)
			expires := time.Now().Add(time.Hour)
			if expired.Load() {
				expires = time.Now().Add(-time.Hour)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "installation-secret", "expires_at": expires})
		case "/installation/repositories":
			if r.URL.Query().Get("per_page") != "100" {
				t.Error("repository list must be bounded")
			}
			if r.URL.Query().Get("page") == "1" {
				repos := make([]githubRepo, 100)
				for i := range repos {
					repos[i] = githubRepo{FullName: fmt.Sprintf("team/app%d", i), DefaultBranch: "main"}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"repositories": repos})
			} else {
				_, _ = fmt.Fprint(w, `{"repositories":[{"full_name":"team/app","default_branch":"main"}]}`)
			}
		case "/repos/team/app":
			_, _ = fmt.Fprint(w, `{"full_name":"team/app","default_branch":"main"}`)
		case "/repos/team/app/branches/main":
			_, _ = fmt.Fprint(w, `{"name":"main"}`)
		case "/repos/team/app/zipball/main":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(archive.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer fake.Close()
	t.Setenv("HANDOFF_GITHUB_API_BASE_URL", fake.URL)
	h := s.routes()
	request := func(method, path, body string, want int) string {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, operatorRequest(s, method, path, body))
		if rec.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "installation-secret") {
			t.Fatal("response leaked token")
		}
		return rec.Body.String()
	}
	request("GET", "/api/integrations/github/installations", "", 200)
	request("POST", "/api/integrations/github/installations", `{"installation_id":99}`, 400)
	request("POST", "/api/integrations/github/installations", `{"installation_id":42}`, 200)
	status := request("GET", "/api/integrations/github", "", 200)
	if !strings.Contains(status, `"authentication":"github_app"`) {
		t.Fatal("missing authentication mode")
	}
	firstPage := request("GET", "/api/integrations/github/repositories?page=1", "", 200)
	secondPage := request("GET", "/api/integrations/github/repositories?page=2", "", 200)
	if !strings.Contains(firstPage, `"has_more":true`) || !strings.Contains(secondPage, `"has_more":false`) || !strings.Contains(secondPage, `"page":2`) {
		t.Fatal("repository pagination metadata missing")
	}
	request("GET", "/api/integrations/github/repositories?page=-1", "", 400)
	request("POST", "/api/integrations/github/repository", `{"repository":"team/app","branch":"missing"}`, 400)
	request("POST", "/api/integrations/github/repository", `{"repository":"team/app","branch":"main"}`, 200)
	dest := t.TempDir()
	if err := importGitHubSource(context.Background(), s, "team/app", "", dest); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(dest, "README.md")); err != nil || string(raw) != "synthetic application" {
		t.Fatal("installation archive import did not use saved branch")
	}
	_, tokens, ok := s.currentGitHubConnection()
	if !ok || tokens.InstallationID != 42 || tokens.AccessToken != "" {
		t.Fatal("installation token persisted")
	}
	if minted.Load() < 4 {
		t.Fatal("operations did not mint fresh credentials")
	}
	expired.Store(true)
	request("GET", "/api/integrations/github/repositories", "", 400)
	expired.Store(false)
	revoked.Store(true)
	request("GET", "/api/integrations/github/repositories", "", 400)
	// Disconnect remains available even when GitHub has revoked access.
	request("DELETE", "/api/integrations/github", "", 200)
	if _, _, ok := s.currentGitHubConnection(); ok {
		t.Fatal("disconnect left credentials")
	}
	unauth := httptest.NewRecorder()
	s.githubInstallations(unauth, httptest.NewRequest("GET", "/", nil))
	if unauth.Code != 403 {
		t.Fatal("installation list allowed without operator access")
	}
	// The encrypted file is removed on disconnect, and keys never enter its JSON.
	path, _ := githubStorePath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("credential store remains after disconnect")
	}
}

func TestCorruptedConnectionNonce(t *testing.T) {
	s := testServer(t)
	t.Setenv("HANDOFF_SLACK_STORE", filepath.Join(t.TempDir(), "slack.json"))
	t.Setenv("HANDOFF_SLACK_ENCRYPTION_KEY", "test-key")
	t.Setenv("HANDOFF_GITHUB_STORE", filepath.Join(t.TempDir(), "github.json"))
	t.Setenv("HANDOFF_GITHUB_ENCRYPTION_KEY", "test-key")
	if err := s.saveSlackConnection(slackConnection{Webhook: "synthetic"}); err != nil {
		t.Fatal(err)
	}
	if err := s.saveGitHubConnection(githubConnection{}, githubTokens{AccessToken: "synthetic"}); err != nil {
		t.Fatal(err)
	}
	for _, provider := range []string{"slack", "github"} {
		t.Run(provider, func(t *testing.T) {
			path, _ := slackStorePath()
			if provider == "github" {
				path, _ = githubStorePath()
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var stored map[string]any
			if err := json.Unmarshal(raw, &stored); err != nil {
				t.Fatal(err)
			}
			stored["nonce"] = ""
			raw, _ = json.Marshal(stored)
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if provider == "slack" {
				if _, _, err := s.loadSlackConnection(); err == nil {
					t.Fatal("corrupted nonce accepted")
				}
			} else if _, _, _, err := s.loadGitHubConnectionBody(); err == nil {
				t.Fatal("corrupted nonce accepted")
			}
		})
	}
}
