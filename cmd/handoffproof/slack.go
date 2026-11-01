package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/agent"
	"github.com/gowtham472/handoffproof/internal/controller"
)

// Slack connection support: Slack OAuth v2 with the incoming-webhook scope.
// Slack returns a channel-specific webhook URL; only that webhook is stored,
// encrypted on the server. Bot/user access tokens from the OAuth response are
// discarded. References:
// https://docs.slack.dev/authentication/installing-with-oauth/
// https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks/

type slackConnection struct {
	Webhook   string
	TeamID    string
	TeamName  string
	Channel   string
	ChannelID string
	Source    string
}

type slackStored struct {
	Version    int    `json:"version"`
	TeamID     string `json:"team_id"`
	TeamName   string `json:"team_name"`
	Channel    string `json:"channel"`
	ChannelID  string `json:"channel_id"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type slackOAuthState struct {
	RedirectURI string
	ReturnTo    string
	Expires     time.Time
}

func slackClientID() string {
	if v := os.Getenv("HANDOFF_SLACK_CLIENT_ID"); v != "" {
		return v
	}
	return os.Getenv("SLACK_CLIENT_ID")
}

func slackClientSecret() string {
	if v := os.Getenv("HANDOFF_SLACK_CLIENT_SECRET"); v != "" {
		return v
	}
	return os.Getenv("SLACK_CLIENT_SECRET")
}

func slackBaseURL() string {
	if v := strings.TrimRight(os.Getenv("HANDOFF_SLACK_BASE_URL"), "/"); v != "" {
		return v
	}
	return "https://slack.com"
}

func (s *server) slackRedirectURI(r *http.Request) string {
	if v := strings.TrimRight(os.Getenv("HANDOFF_PUBLIC_URL"), "/"); v != "" {
		return v + "/api/integrations/slack/callback"
	}
	if r != nil && r.Header.Get("X-Forwarded-Proto") == "https" && r.Host != "" {
		return "https://" + r.Host + "/api/integrations/slack/callback"
	}
	return ""
}

func (s *server) slackConfig(r *http.Request) (redirectURI string, reason string) {
	if slackClientID() == "" || slackClientSecret() == "" {
		return "", "Slack app is not configured on the server: set HANDOFF_SLACK_CLIENT_ID and HANDOFF_SLACK_CLIENT_SECRET."
	}
	redirectURI = s.slackRedirectURI(r)
	if redirectURI == "" {
		return "", "Slack needs an HTTPS callback: set HANDOFF_PUBLIC_URL to the public HTTPS address of this console (for local use, an HTTPS tunnel to it)."
	}
	if !strings.HasPrefix(redirectURI, "https://") {
		return "", "HANDOFF_PUBLIC_URL must be an HTTPS address; Slack redirect URLs must use HTTPS."
	}
	return redirectURI, ""
}

func slackStorePath() (string, error) {
	if v := os.Getenv("HANDOFF_SLACK_STORE"); v != "" {
		return v, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "HandoffProof", "slack-connection.json"), nil
}

func slackEncryptionKey(storePath string) ([]byte, error) {
	if v := os.Getenv("HANDOFF_SLACK_ENCRYPTION_KEY"); v != "" {
		if raw, err := base64.StdEncoding.DecodeString(v); err == nil && (len(raw) == 16 || len(raw) == 24 || len(raw) == 32) {
			return raw, nil
		}
		if raw, err := base64.RawStdEncoding.DecodeString(v); err == nil && (len(raw) == 16 || len(raw) == 24 || len(raw) == 32) {
			return raw, nil
		}
		sum := sha256.Sum256([]byte(v))
		return sum[:], nil
	}
	keyPath := storePath + ".key"
	if raw, err := os.ReadFile(keyPath); err == nil {
		if decoded, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw))); derr == nil && len(decoded) == 32 {
			return decoded, nil
		}
		if len(raw) == 32 {
			return raw, nil
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *server) saveSlackConnection(conn slackConnection) error {
	s.slackMu.Lock()
	defer s.slackMu.Unlock()
	path, err := slackStorePath()
	if err != nil {
		return err
	}
	key, err := slackEncryptionKey(path)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	stored := slackStored{
		Version: 1, TeamID: conn.TeamID, TeamName: conn.TeamName,
		Channel: conn.Channel, ChannelID: conn.ChannelID,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, []byte(conn.Webhook), nil)),
	}
	raw, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *server) loadSlackConnection() (slackConnection, bool, error) {
	path, err := slackStorePath()
	if err != nil {
		return slackConnection{}, false, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return slackConnection{}, false, nil
	}
	if err != nil {
		return slackConnection{}, false, err
	}
	var stored slackStored
	if err := json.Unmarshal(raw, &stored); err != nil {
		return slackConnection{}, false, err
	}
	key, err := slackEncryptionKey(path)
	if err != nil {
		return slackConnection{}, false, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return slackConnection{}, false, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return slackConnection{}, false, err
	}
	nonce, err := base64.StdEncoding.DecodeString(stored.Nonce)
	if err != nil {
		return slackConnection{}, false, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(stored.Ciphertext)
	if err != nil {
		return slackConnection{}, false, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return slackConnection{}, false, err
	}
	return slackConnection{
		Webhook: string(plain), TeamID: stored.TeamID, TeamName: stored.TeamName,
		Channel: stored.Channel, ChannelID: stored.ChannelID, Source: "oauth",
	}, true, nil
}

func (s *server) deleteSlackConnection() error {
	s.slackMu.Lock()
	defer s.slackMu.Unlock()
	path, err := slackStorePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func validSlackWebhook(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "hooks.slack.com" && strings.HasPrefix(u.Path, "/services/")
}

func (s *server) currentSlackConnection() slackConnection {
	s.slackMu.Lock()
	defer s.slackMu.Unlock()
	if conn, ok, err := s.loadSlackConnection(); err == nil && ok && validSlackWebhook(conn.Webhook) {
		return conn
	}
	if hook := os.Getenv("HANDOFF_SLACK_WEBHOOK"); hook != "" {
		return slackConnection{Webhook: hook, Source: "env"}
	}
	return slackConnection{}
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *server) putSlackState(state string, rec slackOAuthState) {
	s.slackMu.Lock()
	defer s.slackMu.Unlock()
	if s.slackStates == nil {
		s.slackStates = map[string]slackOAuthState{}
	}
	for key, old := range s.slackStates {
		if time.Now().After(old.Expires) {
			delete(s.slackStates, key)
		}
	}
	sum := sha256.Sum256([]byte(state))
	s.slackStates[hex.EncodeToString(sum[:])] = rec
}

func (s *server) takeSlackState(state string) (slackOAuthState, bool) {
	s.slackMu.Lock()
	defer s.slackMu.Unlock()
	if s.slackStates == nil {
		return slackOAuthState{}, false
	}
	sum := sha256.Sum256([]byte(state))
	key := hex.EncodeToString(sum[:])
	rec, ok := s.slackStates[key]
	if ok {
		delete(s.slackStates, key)
	}
	if !ok || time.Now().After(rec.Expires) {
		return slackOAuthState{}, false
	}
	return rec, true
}

func slackReturnURL(returnTo, status string) string {
	if returnTo == "/report" {
		return "/report?slack=" + status
	}
	if returnTo == "/settings" {
		return "/?slack=" + status + "#settings"
	}
	return "/?slack=" + status + "#evidence"
}

func (s *server) slackConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	if !s.authorized(r) {
		writeJSON(w, 403, map[string]string{"error": "only console operators can connect Slack"})
		return
	}
	redirectURI, reason := s.slackConfig(r)
	if reason != "" {
		writeJSON(w, 400, map[string]string{"error": reason})
		return
	}
	state, err := randomHex(32)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not start Slack connection"})
		return
	}
	returnTo := "/"
	if r.URL.Query().Get("from") == "report" {
		returnTo = "/report"
	}
	if r.URL.Query().Get("from") == "settings" {
		returnTo = "/settings"
	}
	s.putSlackState(state, slackOAuthState{RedirectURI: redirectURI, ReturnTo: returnTo, Expires: time.Now().Add(10 * time.Minute)})
	http.SetCookie(w, &http.Cookie{
		Name: "hp_slack_state", Value: state, Path: "/api/integrations/slack",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600,
		Secure: strings.HasPrefix(redirectURI, "https://"),
	})
	q := url.Values{
		"scope":        {"incoming-webhook"},
		"client_id":    {slackClientID()},
		"redirect_uri": {redirectURI},
		"state":        {state},
	}
	http.Redirect(w, r, slackBaseURL()+"/oauth/v2/authorize?"+q.Encode(), http.StatusFound)
}

func (s *server) slackCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	state := r.URL.Query().Get("state")
	cookie, cookieErr := r.Cookie("hp_slack_state")
	if cookieErr != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Slack connection failed: OAuth state did not match. Start again from HandoffProof.\n"))
		return
	}
	rec, ok := s.takeSlackState(state)
	http.SetCookie(w, &http.Cookie{Name: "hp_slack_state", Value: "", Path: "/api/integrations/slack", HttpOnly: true, MaxAge: -1})
	if !ok {
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Slack connection failed: OAuth state expired. Start again from HandoffProof.\n"))
		return
	}
	if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		http.Redirect(w, r, slackReturnURL(rec.ReturnTo, "error"), http.StatusFound)
		return
	}
	form := url.Values{
		"client_id":     {slackClientID()},
		"client_secret": {slackClientSecret()},
		"code":          {r.URL.Query().Get("code")},
		"redirect_uri":  {rec.RedirectURI},
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, slackBaseURL()+"/api/oauth.v2.access", strings.NewReader(form.Encode()))
	if err != nil {
		http.Redirect(w, r, slackReturnURL(rec.ReturnTo, "error"), http.StatusFound)
		return
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		http.Redirect(w, r, slackReturnURL(rec.ReturnTo, "error"), http.StatusFound)
		return
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Team  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"team"`
		IncomingWebhook struct {
			Channel   string `json:"channel"`
			ChannelID string `json:"channel_id"`
			URL       string `json:"url"`
		} `json:"incoming_webhook"`
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || json.Unmarshal(raw, &out) != nil || !out.OK || !validSlackWebhook(out.IncomingWebhook.URL) {
		http.Redirect(w, r, slackReturnURL(rec.ReturnTo, "error"), http.StatusFound)
		return
	}
	conn := slackConnection{
		Webhook: out.IncomingWebhook.URL, TeamID: out.Team.ID, TeamName: out.Team.Name,
		Channel: out.IncomingWebhook.Channel, ChannelID: out.IncomingWebhook.ChannelID, Source: "oauth",
	}
	if err := s.saveSlackConnection(conn); err != nil {
		http.Redirect(w, r, slackReturnURL(rec.ReturnTo, "error"), http.StatusFound)
		return
	}
	http.Redirect(w, r, slackReturnURL(rec.ReturnTo, "connected"), http.StatusFound)
}

// slackStatus reports the Slack connection (GET), removes a stored OAuth
// connection (DELETE), or posts a controller-built report summary (POST).
// The message contains only counts and failed-assertion names - never tokens,
// links, webhook URLs, or model text.
func (s *server) slackStatus(w http.ResponseWriter, r *http.Request) {
	conn := s.currentSlackConnection()
	if r.Method == http.MethodGet {
		_, reason := s.slackConfig(r)
		writeJSON(w, 200, map[string]any{
			"connected":    conn.Webhook != "",
			"source":       conn.Source,
			"team_name":    conn.TeamName,
			"channel":      conn.Channel,
			"channel_id":   conn.ChannelID,
			"operator":     s.authorized(r),
			"configured":   slackClientID() != "" && slackClientSecret() != "",
			"can_connect":  s.authorized(r) && reason == "",
			"setup_reason": reason,
			"redirect_url": s.slackRedirectURI(r),
			"connect_url":  "/api/integrations/slack/connect",
		})
		return
	}
	if r.Method == http.MethodDelete {
		if !s.authorized(r) {
			writeJSON(w, 403, map[string]string{"error": "only console operators can disconnect Slack"})
			return
		}
		if conn.Source != "oauth" {
			writeJSON(w, 400, map[string]string{"error": "no stored Slack connection to disconnect"})
			return
		}
		if err := s.deleteSlackConnection(); err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not disconnect Slack"})
			return
		}
		writeJSON(w, 200, map[string]bool{"connected": false})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	if !s.authorized(r) {
		writeJSON(w, 403, map[string]string{"error": "only console operators can post to Slack"})
		return
	}
	hook := conn.Webhook
	if hook == "" {
		writeJSON(w, 400, map[string]string{"error": "Slack is not connected: use Add to Slack from the share dialog"})
		return
	}
	store := loadEvidenceStore(s.root)
	recs := store.All()
	passed, failed := 0, 0
	var fails []string
	for _, rec := range recs {
		if rec.Passed {
			passed++
		} else {
			failed++
			if len(fails) < 3 {
				fails = append(fails, agent.Redact(rec.Assertion))
			}
		}
	}
	integrity := "manifest verified unchanged"
	if err := controller.New(s.root).ManifestVerify(context.Background()); err != nil {
		integrity = "manifest MISMATCH - do not trust"
	}
	text := fmt.Sprintf("HandoffProof report: %d evidence records (%d passed, %d failed). %s.", len(recs), passed, failed, integrity)
	if len(fails) > 0 {
		text += " Failing: " + strings.Join(fails, " · ")
	}
	body, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, hook, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "could not build Slack request"})
		return
	}
	req.Header.Set("content-type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "Slack post failed: " + err.Error()})
		return
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		writeJSON(w, 502, map[string]string{"error": fmt.Sprintf("Slack rejected the post (status %d)", res.StatusCode)})
		return
	}
	writeJSON(w, 200, map[string]bool{"posted": true})
}
