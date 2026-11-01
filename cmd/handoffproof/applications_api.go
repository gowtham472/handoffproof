package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/byoa"
	"github.com/gowtham472/handoffproof/internal/evidence"
	"github.com/gowtham472/handoffproof/internal/scan"
)

func (s *server) requireOperator(w http.ResponseWriter, r *http.Request) bool {
	if !s.authorized(r) {
		writeJSON(w, 403, map[string]string{"error": "only console operators can manage applications"})
		return false
	}
	return true
}

func applicationsRoot() (string, error) {
	path, err := applicationsStorePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "applications"), nil
}

func importLocalSource(localPath, dest string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("local_path must be a directory")
	}
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err != nil {
		return err
	}
	return byoa.CopySource(abs, dest)
}

func importGitHubSource(ctx context.Context, s *server, repository, branch, dest string) error {
	_, tokens, connected := s.currentGitHubConnection()
	if !connected {
		return fmt.Errorf("GitHub is not connected")
	}
	if !githubRepoRe.MatchString(repository) {
		return fmt.Errorf("repository must look like owner/repository")
	}
	if branch == "" {
		branch = "main"
	}
	api := githubAPIBaseURL() + "/repos/" + repository + "/zipball/" + url.PathEscape(branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	req.Header.Set("User-Agent", "HandoffProof")
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return fmt.Errorf("too many redirects")
		}
		if req.URL.Host != "codeload.github.com" && req.URL.Host != urlHost(api) {
			return fmt.Errorf("unexpected archive host %q", req.URL.Host)
		}
		return nil
	}}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download GitHub archive: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("GitHub archive returned %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 51<<20))
	if err != nil {
		return err
	}
	if len(raw) > 50<<20 {
		return fmt.Errorf("GitHub archive exceeds 50 MB")
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		name := filepath.Clean(f.Name)
		parts := strings.Split(filepath.ToSlash(name), "/")
		if len(parts) < 2 {
			continue
		}
		rel := filepath.FromSlash(strings.Join(parts[1:], "/"))
		if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("unsafe path in GitHub archive")
		}
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in GitHub archive is not allowed")
		}
		target := filepath.Join(dest, rel)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(rc, 20<<20))
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func urlHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

type applicationCreateRequest struct {
	Name       string            `json:"name"`
	SourceKind string            `json:"source_kind"`
	LocalPath  string            `json:"local_path"`
	Repository string            `json:"repository"`
	Branch     string            `json:"branch"`
	Profile    byoa.Profile      `json:"profile"`
	EnvValues  map[string]string `json:"env_values"`
}

func (s *server) applications(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
		return
	}
	apps, _, err := s.loadApplications()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not read applications"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		out := []applicationRecord{}
		for _, a := range apps {
			out = append(out, publicApplication(a))
		}
		writeJSON(w, 200, map[string]any{"applications": out, "operator": true})
	case http.MethodPost:
		var req applicationCreateRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid application request"})
			return
		}
		if req.Profile.ID == "" || !applicationIDRe.MatchString(req.Profile.ID) {
			writeJSON(w, 400, map[string]string{"error": "profile.id must be a lowercase slug"})
			return
		}
		if _, _, exists := findApplication(apps, req.Profile.ID); exists {
			writeJSON(w, 409, map[string]string{"error": "an application with this id already exists"})
			return
		}
		root, err := applicationsRoot()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not resolve application store"})
			return
		}
		dest := filepath.Join(root, req.Profile.ID, "source")
		if err := os.MkdirAll(dest, 0o755); err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not create application source"})
			return
		}
		switch req.SourceKind {
		case "local":
			if err := importLocalSource(req.LocalPath, dest); err != nil {
				_ = os.RemoveAll(filepath.Join(root, req.Profile.ID))
				writeJSON(w, 400, map[string]string{"error": "import local application: " + err.Error()})
				return
			}
		case "github":
			if err := importGitHubSource(r.Context(), s, req.Repository, req.Branch, dest); err != nil {
				_ = os.RemoveAll(filepath.Join(root, req.Profile.ID))
				writeJSON(w, 400, map[string]string{"error": "import GitHub application: " + err.Error()})
				return
			}
		default:
			_ = os.RemoveAll(filepath.Join(root, req.Profile.ID))
			writeJSON(w, 400, map[string]string{"error": "source_kind must be local or github"})
			return
		}
		profileRaw, err := json.MarshalIndent(req.Profile, "", "  ")
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid profile"})
			return
		}
		if err := os.WriteFile(filepath.Join(dest, byoa.ProfileFile), profileRaw, 0o644); err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not save application profile"})
			return
		}
		profile, err := byoa.Load(dest)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		storePath, _ := applicationsStorePath()
		nonce, cipherText, err := encryptEnv(storePath, req.EnvValues)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not encrypt application environment"})
			return
		}
		names := make([]string, 0, len(req.EnvValues))
		for k := range req.EnvValues {
			names = append(names, k)
		}
		now := time.Now().UTC()
		rec := storedApplication{
			applicationRecord: applicationRecord{
				ID: profile.ID, Name: req.Name, SourceKind: req.SourceKind, Repository: req.Repository, Branch: req.Branch,
				Profile: *profile, EnvNames: names, CreatedAt: now, UpdatedAt: now,
			},
			SourceDir: dest, EnvCipher: cipherText, Nonce: nonce,
		}
		if rec.Name == "" {
			rec.Name = profile.Name
		}
		apps = append(apps, rec)
		if err := s.saveApplications(apps); err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not save application"})
			return
		}
		writeJSON(w, 201, map[string]any{"application": publicApplication(rec)})
	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

func (s *server) applicationByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperator(w, r) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/applications/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, 404, map[string]string{"error": "application not found"})
		return
	}
	id := parts[0]
	apps, _, err := s.loadApplications()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not read applications"})
		return
	}
	app, idx, ok := findApplication(apps, id)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "application not found"})
		return
	}
	if len(parts) == 1 {
		if r.Method == http.MethodGet {
			writeJSON(w, 200, map[string]any{"application": publicApplication(app)})
			return
		}
		if r.Method == http.MethodDelete {
			root, _ := applicationsRoot()
			_ = os.RemoveAll(filepath.Join(root, id))
			apps = append(apps[:idx], apps[idx+1:]...)
			if err := s.saveApplications(apps); err != nil {
				writeJSON(w, 500, map[string]string{"error": "could not delete application"})
				return
			}
			writeJSON(w, 200, map[string]bool{"deleted": true})
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	switch parts[1] {
	case "scan":
		if r.Method != http.MethodGet {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		rep, err := scan.Scan(app.SourceDir)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"scan": rep})
	case "validate":
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		profile, err := byoa.Load(app.SourceDir)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if err := byoa.ValidateCompose(app.SourceDir, profile.ComposeFile); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"valid": true, "profile": profile})
	case "run":
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		var body struct {
			Kind   string `json:"kind"`
			Target string `json:"target"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid run request"})
			return
		}
		if body.Kind == "" {
			body.Kind = "fresh_deployment_baseline"
		}
		storePath, _ := applicationsStorePath()
		envValues, err := decryptEnv(storePath, app.Nonce, app.EnvCipher)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not decrypt application environment"})
			return
		}
		profile, err := byoa.Load(app.SourceDir)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Minute)
		defer cancel()
		recs, err := byoa.Run(ctx, byoa.RunRequest{SourceDir: app.SourceDir, Profile: profile, Env: envValues, Kind: body.Kind, Target: body.Target, RunID: runID})
		if err != nil && len(recs) == 0 {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		raw, merr := json.MarshalIndent(recs, "", "  ")
		if merr != nil {
			writeJSON(w, 500, map[string]string{"error": "could not encode evidence"})
			return
		}
		name := fmt.Sprintf("run-%s-%s.json", runID, body.Kind)
		if werr := os.WriteFile(filepath.Join(s.root, "evidence", name), raw, 0o644); werr != nil {
			writeJSON(w, 500, map[string]string{"error": "could not write evidence"})
			return
		}
		apps[idx].LastRun = runID
		apps[idx].LastKind = body.Kind
		apps[idx].UpdatedAt = time.Now().UTC()
		_ = s.saveApplications(apps)
		status := 200
		resp := map[string]any{"run_id": runID, "kind": body.Kind, "records": recs, "evidence_file": name}
		if err != nil {
			resp["warning"] = err.Error()
			status = 207
		}
		writeJSON(w, status, resp)
	default:
		writeJSON(w, 404, map[string]string{"error": "application action not found"})
	}
}

var _ = evidence.Record{}
