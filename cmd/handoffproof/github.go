package main

import (
	"bytes"
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
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GitHub connection support: GitHub OAuth web application flow with PKCE.
// The access token is used only server-side to read the connected account and
// repositories and to remember the operator's selected repository for the
// remediation flow. Tokens are encrypted at rest and never returned to the
// browser. References:
// https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps

type githubConnection struct {
	Login      string
	Name       string
	ProfileURL string
	Repository string
	Branch     string
	Scope      string
}

type githubTokens struct {
	AccessToken    string `json:"access_token"`
	RefreshToken   string `json:"refresh_token,omitempty"`
	InstallationID int64  `json:"installation_id,omitempty"`
}

type githubStored struct {
	Version    int    `json:"version"`
	Login      string `json:"login"`
	Name       string `json:"name"`
	ProfileURL string `json:"profile_url"`
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Scope      string `json:"scope"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type githubOAuthState struct {
	RedirectURI string
	Verifier    string
	Expires     time.Time
}

var githubRepoRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9_.-]+$`)

func validGitHubRepository(repository string) bool {
	if !githubRepoRe.MatchString(repository) {
		return false
	}
	name := strings.Split(repository, "/")[1]
	return name != "." && name != ".."
}

func githubClientID() string {
	if v := os.Getenv("HANDOFF_GITHUB_CLIENT_ID"); v != "" {
		return v
	}
	return os.Getenv("GITHUB_CLIENT_ID")
}

func githubClientSecret() string {
	if v := os.Getenv("HANDOFF_GITHUB_CLIENT_SECRET"); v != "" {
		return v
	}
	return os.Getenv("GITHUB_CLIENT_SECRET")
}

func githubBaseURL() string {
	if v := strings.TrimRight(os.Getenv("HANDOFF_GITHUB_BASE_URL"), "/"); v != "" {
		return v
	}
	return "https://github.com"
}

func githubAPIBaseURL() string {
	if v := strings.TrimRight(os.Getenv("HANDOFF_GITHUB_API_BASE_URL"), "/"); v != "" {
		return v
	}
	return "https://api.github.com"
}

func isLoopbackHost(host string) bool {
	h := strings.ToLower(host)
	return strings.HasPrefix(h, "127.0.0.1") || strings.HasPrefix(h, "localhost") || strings.HasPrefix(h, "[::1]")
}

func (s *server) githubRedirectURI(r *http.Request) string {
	if v := strings.TrimRight(os.Getenv("HANDOFF_PUBLIC_URL"), "/"); v != "" {
		return v + "/api/integrations/github/callback"
	}
	if r != nil && r.Header.Get("X-Forwarded-Proto") == "https" && r.Host != "" {
		return "https://" + r.Host + "/api/integrations/github/callback"
	}
	if r != nil && isLoopbackHost(r.Host) {
		return "http://" + r.Host + "/api/integrations/github/callback"
	}
	return ""
}

func (s *server) githubConfig(r *http.Request) (string, string) {
	if githubClientID() == "" || githubClientSecret() == "" {
		return "", "GitHub app is not configured on the server: set HANDOFF_GITHUB_CLIENT_ID and HANDOFF_GITHUB_CLIENT_SECRET."
	}
	redirectURI := s.githubRedirectURI(r)
	if redirectURI == "" {
		return "", "GitHub needs a callback address: set HANDOFF_PUBLIC_URL to the public address of this console (an HTTPS tunnel for local use)."
	}
	if !strings.HasPrefix(redirectURI, "https://") && !isLoopbackHost(strings.TrimPrefix(strings.TrimPrefix(redirectURI, "http://"), "https://")) {
		return "", "HANDOFF_PUBLIC_URL must be HTTPS outside loopback."
	}
	return redirectURI, ""
}

func githubStorePath() (string, error) {
	if v := os.Getenv("HANDOFF_GITHUB_STORE"); v != "" {
		return v, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "HandoffProof", "github-connection.json"), nil
}

func githubEncryptionKey(storePath string) ([]byte, error) {
	if v := os.Getenv("HANDOFF_GITHUB_ENCRYPTION_KEY"); v != "" {
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

func (s *server) saveGitHubConnection(conn githubConnection, tokens githubTokens) error {
	if tokens.InstallationID > 0 {
		tokens.AccessToken, tokens.RefreshToken = "", ""
	}
	s.githubMu.Lock()
	defer s.githubMu.Unlock()
	path, err := githubStorePath()
	if err != nil {
		return err
	}
	key, err := githubEncryptionKey(path)
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
	payload, err := json.Marshal(tokens)
	if err != nil {
		return err
	}
	stored := githubStored{
		Version: 1, Login: conn.Login, Name: conn.Name, ProfileURL: conn.ProfileURL,
		Repository: conn.Repository, Branch: conn.Branch, Scope: conn.Scope,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, payload, nil)),
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

func (s *server) loadGitHubConnectionBody() (githubConnection, githubTokens, bool, error) {
	path, err := githubStorePath()
	if err != nil {
		return githubConnection{}, githubTokens{}, false, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return githubConnection{}, githubTokens{}, false, nil
	}
	if err != nil {
		return githubConnection{}, githubTokens{}, false, err
	}
	var stored githubStored
	if err := json.Unmarshal(raw, &stored); err != nil {
		return githubConnection{}, githubTokens{}, false, err
	}
	key, err := githubEncryptionKey(path)
	if err != nil {
		return githubConnection{}, githubTokens{}, false, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return githubConnection{}, githubTokens{}, false, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return githubConnection{}, githubTokens{}, false, err
	}
	nonce, err := base64.StdEncoding.DecodeString(stored.Nonce)
	if err != nil {
		return githubConnection{}, githubTokens{}, false, err
	}
	if len(nonce) != gcm.NonceSize() {
		return githubConnection{}, githubTokens{}, false, fmt.Errorf("invalid stored GitHub nonce")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(stored.Ciphertext)
	if err != nil {
		return githubConnection{}, githubTokens{}, false, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return githubConnection{}, githubTokens{}, false, err
	}
	var tokens githubTokens
	if err := json.Unmarshal(plain, &tokens); err != nil {
		return githubConnection{}, githubTokens{}, false, err
	}
	return githubConnection{
		Login: stored.Login, Name: stored.Name, ProfileURL: stored.ProfileURL,
		Repository: stored.Repository, Branch: stored.Branch, Scope: stored.Scope,
	}, tokens, true, nil
}

func (s *server) currentGitHubConnection() (githubConnection, githubTokens, bool) {
	s.githubMu.Lock()
	defer s.githubMu.Unlock()
	conn, tokens, ok, err := s.loadGitHubConnectionBody()
	return conn, tokens, err == nil && ok && (tokens.AccessToken != "" || tokens.InstallationID > 0)
}

func (s *server) deleteGitHubConnection() error {
	s.githubMu.Lock()
	defer s.githubMu.Unlock()
	path, err := githubStorePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *server) putGitHubState(state string, rec githubOAuthState) {
	s.githubMu.Lock()
	defer s.githubMu.Unlock()
	if s.githubStates == nil {
		s.githubStates = map[string]githubOAuthState{}
	}
	for key, old := range s.githubStates {
		if time.Now().After(old.Expires) {
			delete(s.githubStates, key)
		}
	}
	sum := sha256.Sum256([]byte(state))
	s.githubStates[hex.EncodeToString(sum[:])] = rec
}

func (s *server) takeGitHubState(state string) (githubOAuthState, bool) {
	s.githubMu.Lock()
	defer s.githubMu.Unlock()
	if s.githubStates == nil {
		return githubOAuthState{}, false
	}
	sum := sha256.Sum256([]byte(state))
	key := hex.EncodeToString(sum[:])
	rec, ok := s.githubStates[key]
	if ok {
		delete(s.githubStates, key)
	}
	if !ok || time.Now().After(rec.Expires) {
		return githubOAuthState{}, false
	}
	return rec, true
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *server) githubConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	if !s.authorized(r) {
		writeJSON(w, 403, map[string]string{"error": "only console operators can connect GitHub"})
		return
	}
	redirectURI, reason := s.githubConfig(r)
	if reason != "" {
		writeJSON(w, 400, map[string]string{"error": reason})
		return
	}
	state, err := randomHex(32)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not start GitHub connection"})
		return
	}
	verifier, err := randomHex(32)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not start GitHub connection"})
		return
	}
	s.putGitHubState(state, githubOAuthState{RedirectURI: redirectURI, Verifier: verifier, Expires: time.Now().Add(10 * time.Minute)})
	http.SetCookie(w, &http.Cookie{
		Name: "hp_github_state", Value: state, Path: "/api/integrations/github",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600,
		Secure: strings.HasPrefix(redirectURI, "https://"),
	})
	q := url.Values{
		"client_id":             {githubClientID()},
		"redirect_uri":          {redirectURI},
		"scope":                 {"repo"},
		"state":                 {state},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	http.Redirect(w, r, githubBaseURL()+"/login/oauth/authorize?"+q.Encode(), http.StatusFound)
}

func (s *server) githubAPI(ctxReq *http.Request, token, path string, out any) error {
	return githubRequest(ctxReq.Context(), http.MethodGet, token, path, nil, out)
}

func (s *server) githubCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	state := r.URL.Query().Get("state")
	cookie, cookieErr := r.Cookie("hp_github_state")
	if cookieErr != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("GitHub connection failed: OAuth state did not match. Start again from HandoffProof Settings.\n"))
		return
	}
	rec, ok := s.takeGitHubState(state)
	http.SetCookie(w, &http.Cookie{Name: "hp_github_state", Value: "", Path: "/api/integrations/github", HttpOnly: true, MaxAge: -1})
	if !ok || r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		http.Redirect(w, r, "/?github=error#settings", http.StatusFound)
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"client_id": githubClientID(), "client_secret": githubClientSecret(),
		"code": r.URL.Query().Get("code"), "redirect_uri": rec.RedirectURI, "code_verifier": rec.Verifier,
	})
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, githubBaseURL()+"/login/oauth/access_token", bytes.NewReader(payload))
	if err != nil {
		http.Redirect(w, r, "/?github=error#settings", http.StatusFound)
		return
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		http.Redirect(w, r, "/?github=error#settings", http.StatusFound)
		return
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || json.Unmarshal(raw, &tok) != nil || tok.Error != "" || tok.AccessToken == "" {
		http.Redirect(w, r, "/?github=error#settings", http.StatusFound)
		return
	}
	var user struct {
		Login   string `json:"login"`
		Name    string `json:"name"`
		HTMLURL string `json:"html_url"`
	}
	if err := s.githubAPI(r, tok.AccessToken, "/user", &user); err != nil || user.Login == "" {
		http.Redirect(w, r, "/?github=error#settings", http.StatusFound)
		return
	}
	conn := githubConnection{Login: user.Login, Name: user.Name, ProfileURL: user.HTMLURL, Scope: tok.Scope}
	if err := s.saveGitHubConnection(conn, githubTokens{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken}); err != nil {
		http.Redirect(w, r, "/?github=error#settings", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/?github=connected#settings", http.StatusFound)
}

func (s *server) githubStatus(w http.ResponseWriter, r *http.Request) {
	conn, tokens, connected := s.currentGitHubConnection()
	if r.Method == http.MethodGet {
		redirectURI, reason := s.githubConfig(r)
		writeJSON(w, 200, map[string]any{
			"connected": connected, "operator": s.authorized(r),
			"configured":  githubClientID() != "" && githubClientSecret() != "",
			"can_connect": s.authorized(r) && reason == "", "setup_reason": reason,
			"redirect_url": redirectURI, "connect_url": "/api/integrations/github/connect",
			"login": conn.Login, "name": conn.Name, "profile_url": conn.ProfileURL,
			"repository": conn.Repository, "branch": conn.Branch, "scope": conn.Scope,
			"app_configured": githubAppConfigured(),
			"authentication": func() string {
				if tokens.InstallationID > 0 {
					return "github_app"
				}
				return "oauth"
			}(),
			"installation_id": tokens.InstallationID,
			"repository_url": func() string {
				if conn.Repository == "" {
					return ""
				}
				return "https://github.com/" + conn.Repository
			}(),
		})
		return
	}
	if r.Method == http.MethodDelete {
		if !s.authorized(r) {
			writeJSON(w, 403, map[string]string{"error": "only console operators can disconnect GitHub"})
			return
		}
		if !connected {
			writeJSON(w, 400, map[string]string{"error": "GitHub is not connected"})
			return
		}
		if err := s.deleteGitHubConnection(); err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not disconnect GitHub"})
			return
		}
		writeJSON(w, 200, map[string]bool{"connected": false})
		return
	}
	writeJSON(w, 405, map[string]string{"error": "method not allowed"})
}

type githubRepo struct {
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
}

func (s *server) githubRepositories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	if !s.authorized(r) {
		writeJSON(w, 403, map[string]string{"error": "only console operators can list GitHub repositories"})
		return
	}
	page, valid := githubPage(r)
	if !valid {
		writeJSON(w, 400, map[string]string{"error": "invalid page"})
		return
	}
	_, tokens, err := s.githubCredentials(r.Context())
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	var repos []githubRepo
	path := "/user/repos?per_page=100&sort=updated&affiliation=owner,collaborator,organization_member&page=" + strconv.Itoa(page)
	var listErr error
	if tokens.InstallationID > 0 {
		var out struct {
			Repositories []githubRepo `json:"repositories"`
		}
		listErr = s.githubAPI(r, tokens.AccessToken, "/installation/repositories?per_page=100&page="+strconv.Itoa(page), &out)
		repos = out.Repositories
	} else {
		listErr = s.githubAPI(r, tokens.AccessToken, path, &repos)
	}
	if listErr != nil {
		writeJSON(w, 502, map[string]string{"error": "could not list GitHub repositories"})
		return
	}
	writeJSON(w, 200, map[string]any{"repositories": repos, "page": page, "has_more": len(repos) == 100})
}

func (s *server) githubRepository(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	if !s.authorized(r) {
		writeJSON(w, 403, map[string]string{"error": "only console operators can choose the GitHub repository"})
		return
	}
	conn, tokens, err := s.githubCredentials(r.Context())
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	var body struct {
		Repository string `json:"repository"`
		Branch     string `json:"branch"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil || !validGitHubRepository(body.Repository) {
		writeJSON(w, 400, map[string]string{"error": "repository must look like owner/repository"})
		return
	}
	var repo githubRepo
	if err := s.githubAPI(r, tokens.AccessToken, "/repos/"+body.Repository, &repo); err != nil || !strings.EqualFold(repo.FullName, body.Repository) {
		writeJSON(w, 400, map[string]string{"error": "GitHub repository is not accessible with this connection"})
		return
	}
	branch := strings.TrimSpace(body.Branch)
	if branch == "" {
		branch = repo.DefaultBranch
	}
	if branch == "" || len(branch) > 255 || strings.ContainsAny(branch, " \t\r\n") {
		writeJSON(w, 400, map[string]string{"error": "invalid branch"})
		return
	}
	if err := s.githubAPI(r, tokens.AccessToken, "/repos/"+body.Repository+"/branches/"+url.PathEscape(branch), nil); err != nil {
		writeJSON(w, 400, map[string]string{"error": "branch is not accessible in this repository"})
		return
	}
	conn.Repository = repo.FullName
	conn.Branch = branch
	if err := s.saveGitHubConnection(conn, tokens); err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not save GitHub repository"})
		return
	}
	writeJSON(w, 200, map[string]any{"connected": true, "repository": conn.Repository, "branch": conn.Branch, "repository_url": repo.HTMLURL})
}
