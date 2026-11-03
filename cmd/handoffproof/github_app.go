package main

import (
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
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"
)

var githubAppSlug = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// App keys remain in server environment configuration. Only the installation ID
// is persisted; installation access tokens are minted afresh for each operation.
func githubAppConfigured() bool {
	return os.Getenv("HANDOFF_GITHUB_APP_ID") != "" && os.Getenv("HANDOFF_GITHUB_APP_PRIVATE_KEY") != ""
}

func githubAppJWT(now time.Time) (string, error) {
	if os.Getenv("HANDOFF_GITHUB_APP_ID") == "" {
		return "", fmt.Errorf("GitHub App ID is required")
	}
	block, _ := pem.Decode([]byte(os.Getenv("HANDOFF_GITHUB_APP_PRIVATE_KEY")))
	if block == nil {
		return "", fmt.Errorf("GitHub App private key must be PEM encoded")
	}
	var key *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = parsed
	} else if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, _ = parsed.(*rsa.PrivateKey)
	}
	if key == nil || key.N.BitLen() < 2048 {
		return "", fmt.Errorf("GitHub App requires an RSA private key of at least 2048 bits")
	}
	claims, err := json.Marshal(map[string]any{"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": os.Getenv("HANDOFF_GITHUB_APP_ID")})
	if err != nil {
		return "", fmt.Errorf("encode GitHub App JWT: %w", err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign GitHub App JWT: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func githubRequest(ctx context.Context, method, token, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, githubAPIBaseURL()+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build GitHub request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "HandoffProof")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		// Transport errors can contain URLs or credentials. Do not expose them.
		return fmt.Errorf("GitHub request could not be completed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("GitHub API returned %d; check access and reconnect if needed", res.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out); err != nil {
		return fmt.Errorf("invalid GitHub response")
	}
	return nil
}

type githubInstallation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
	} `json:"account"`
	SuspendedAt *time.Time `json:"suspended_at"`
}

func githubInstallationToken(ctx context.Context, id int64) (string, error) {
	jwt, err := githubAppJWT(time.Now())
	if err != nil {
		return "", err
	}
	var token struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	// Request only the read access required by archive imports and branch lists,
	// even when the installed App has additional permissions.
	err = githubRequest(ctx, http.MethodPost, jwt, "/app/installations/"+strconv.FormatInt(id, 10)+"/access_tokens", []byte(`{"permissions":{"contents":"read","metadata":"read"}}`), &token)
	if err != nil {
		return "", err
	}
	if token.Token == "" || !token.ExpiresAt.After(time.Now().Add(time.Minute)) {
		return "", fmt.Errorf("GitHub returned an empty or expired installation token")
	}
	return token.Token, nil
}

func (s *server) githubCredentials(ctx context.Context) (githubConnection, githubTokens, error) {
	conn, tokens, ok := s.currentGitHubConnection()
	if !ok {
		return conn, tokens, fmt.Errorf("GitHub is not connected")
	}
	if tokens.InstallationID > 0 {
		token, err := githubInstallationToken(ctx, tokens.InstallationID)
		if err != nil {
			return conn, tokens, err
		}
		tokens.AccessToken = token
	}
	return conn, tokens, nil
}

// Installation selection is an authenticated operator action, not a callback
// trusting a browser-supplied installation ID. GitHub verifies App ownership.
func (s *server) githubInstallations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	if !s.authorized(r) {
		writeJSON(w, 403, map[string]string{"error": "only console operators can manage GitHub installations"})
		return
	}
	jwt, err := githubAppJWT(time.Now())
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "configure HANDOFF_GITHUB_APP_ID and a valid HANDOFF_GITHUB_APP_PRIVATE_KEY on the server"})
		return
	}
	if r.Method == http.MethodGet {
		page, ok := githubPage(r)
		if !ok {
			writeJSON(w, 400, map[string]string{"error": "invalid page"})
			return
		}
		var installations []githubInstallation
		if err := githubRequest(r.Context(), http.MethodGet, jwt, "/app/installations?per_page=100&page="+strconv.Itoa(page), nil, &installations); err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		var app struct {
			Slug string `json:"slug"`
		}
		installURL := ""
		if githubRequest(r.Context(), http.MethodGet, jwt, "/app", nil, &app) == nil && githubAppSlug.MatchString(app.Slug) {
			installURL = githubBaseURL() + "/apps/" + app.Slug + "/installations/new"
		}
		writeJSON(w, 200, map[string]any{"installations": installations, "page": page, "has_more": len(installations) == 100, "install_url": installURL})
		return
	}
	var body struct {
		InstallationID int64 `json:"installation_id"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body) != nil || body.InstallationID <= 0 {
		writeJSON(w, 400, map[string]string{"error": "choose an installation"})
		return
	}
	var installation githubInstallation
	if err := githubRequest(r.Context(), http.MethodGet, jwt, "/app/installations/"+strconv.FormatInt(body.InstallationID, 10), nil, &installation); err != nil || installation.ID != body.InstallationID || installation.Account.Login == "" || installation.SuspendedAt != nil {
		writeJSON(w, 400, map[string]string{"error": "installation is unavailable for this GitHub App"})
		return
	}
	if _, err := githubInstallationToken(r.Context(), body.InstallationID); err != nil {
		writeJSON(w, 400, map[string]string{"error": "installation cannot grant repository read access"})
		return
	}
	conn := githubConnection{Login: installation.Account.Login, Scope: "contents:read, metadata:read"}
	if err := s.saveGitHubConnection(conn, githubTokens{InstallationID: installation.ID}); err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not save installation"})
		return
	}
	writeJSON(w, 200, map[string]bool{"connected": true})
}

func githubPage(r *http.Request) (int, bool) {
	if r.URL.Query().Get("page") == "" {
		return 1, true
	}
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	return page, err == nil && page > 0 && page <= 10000
}
