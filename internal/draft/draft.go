// Package draft checks model-drafted BYOA profiles (handoffproof.json).
// The model only proposes a draft; this package decodes it strictly,
// screens it for secret values, validates it with the existing profile and
// Compose rules, and summarizes the controller's sandbox baseline. A draft
// is shown to a human only when every check and the baseline pass.
package draft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gowtham472/handoffproof/internal/agent"
	"github.com/gowtham472/handoffproof/internal/byoa"
	"github.com/gowtham472/handoffproof/internal/evidence"
)

// DefaultModelID drafts with Muse Wonder (exploration across many files).
const DefaultModelID = "muse-wonder-1.0"

// CheckModel refuses "contributor" model IDs, which let Meta train on
// prompts: imported applications are not ours to share.
func CheckModel(modelID string) error {
	if strings.Contains(strings.ToLower(modelID), "contributor") {
		return fmt.Errorf("model %q is a contributor variant; it is not allowed for imported applications", modelID)
	}
	return nil
}

// Workspace copies the application into a temporary directory without any
// existing handoffproof.json, so the draft is written from the code alone.
func Workspace(src string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "hp-draft-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := byoa.CopySource(src, dir); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("copy application: %w", err)
	}
	if err := os.Remove(filepath.Join(dir, byoa.ProfileFile)); err != nil && !os.IsNotExist(err) {
		cleanup()
		return "", nil, err
	}
	return dir, cleanup, nil
}

// Decode parses a draft strictly: unknown fields are an error.
func Decode(raw []byte) (*byoa.Profile, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var p byoa.Profile
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("draft is not a valid profile: %w", err)
	}
	return &p, nil
}

// secretNameRe marks env names that hold credentials.
var secretNameRe = regexp.MustCompile(`(?i)(TOKEN|SECRET|PASSWORD|PASSWD|PASS|KEY|CREDENTIAL|AUTH|DATABASE_URL|DSN)`)

// Screen returns problems when the draft carries secret material: the
// configured value of any secret env var (named like one, or declared
// secret by the draft) or anything the redactor would mask. Profiles hold
// env names and ${NAME} references only. Short or numeric values (ports)
// are skipped so they cannot collide with ordinary profile fields.
func Screen(p *byoa.Profile, env map[string]string) []string {
	raw, err := json.Marshal(p)
	if err != nil {
		return []string{"draft cannot be encoded: " + err.Error()}
	}
	text := string(raw)
	declared := map[string]bool{}
	for _, e := range p.Env {
		if e.Secret {
			declared[e.Name] = true
		}
	}
	var probs []string
	names := make([]string, 0, len(env))
	for k := range env {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		v := env[k]
		if !secretNameRe.MatchString(k) && !declared[k] {
			continue
		}
		if _, err := strconv.Atoi(v); err == nil || len(v) < 8 {
			continue
		}
		if strings.Contains(text, v) {
			probs = append(probs, "draft contains the value of "+k)
		}
	}
	var doc any
	_ = json.Unmarshal(raw, &doc)
	for _, v := range stringLeaves(doc, nil) {
		v = envRefRe.ReplaceAllString(v, "")
		if agent.Redact(v) != v {
			probs = append(probs, "draft contains a secret-like string")
			break
		}
	}
	return probs
}

// envRefRe matches ${NAME} references, the allowed way to use a secret.
var envRefRe = regexp.MustCompile(`\$\{[A-Z][A-Z0-9_]*\}`)

// stringLeaves collects every string value (not keys or booleans) so the
// redactor sees only text the draft wrote.
func stringLeaves(v any, out []string) []string {
	switch t := v.(type) {
	case string:
		out = append(out, t)
	case []any:
		for _, x := range t {
			out = stringLeaves(x, out)
		}
	case map[string]any:
		for _, x := range t {
			out = stringLeaves(x, out)
		}
	}
	return out
}

// Check validates the draft against dir with the profile schema and the
// Compose denylist used for every imported application.
func Check(p *byoa.Profile, dir string) error {
	if err := p.Validate(dir); err != nil {
		return err
	}
	return byoa.ValidateCompose(dir, p.ComposeFile)
}

// Baseline summarizes a fresh_deployment_baseline run: it passes only when
// there is at least one record and every record passed.
func Baseline(recs []evidence.Record) (passed bool, failures, ids []string) {
	for _, r := range recs {
		ids = append(ids, r.ID)
		if !r.Passed {
			failures = append(failures, r.Workflow+": "+r.Assertion)
		}
	}
	return len(recs) > 0 && len(failures) == 0, failures, ids
}

// Result is one draft attempt as recorded under evidence/drafts/.
type Result struct {
	StartedUTC        string          `json:"started_utc"`
	Commit            string          `json:"commit"`
	App               string          `json:"app"`
	ModelID           string          `json:"model"`
	Provider          string          `json:"provider"`
	Mode              string          `json:"mode"`
	Draft             json.RawMessage `json:"draft,omitempty"`
	ModelError        string          `json:"model_error,omitempty"`
	Rejections        []string        `json:"rejections,omitempty"`
	ValidationError   string          `json:"validation_error,omitempty"`
	BaselineRan       bool            `json:"baseline_ran"`
	BaselinePassed    bool            `json:"baseline_passed"`
	BaselineError     string          `json:"baseline_error,omitempty"`
	BaselineFailures  []string        `json:"baseline_failures,omitempty"`
	EvidenceIDs       []string        `json:"evidence_ids,omitempty"`
	Shown             bool            `json:"shown_to_human"`
	Attempts          int             `json:"attempts"`
	FirstAttemptValid bool            `json:"first_attempt_valid"`
	Tokens            int             `json:"tokens"`
	LatencyMS         int64           `json:"latency_ms"`
}

// Scrub replaces every configured test value (8+ characters, non-numeric)
// in text with [REDACTED:env:NAME], so a rejected draft can be recorded
// without the value it leaked. Redaction of other secrets is separate.
func Scrub(text string, env map[string]string) string {
	names := make([]string, 0, len(env))
	for k := range env {
		names = append(names, k)
	}
	// Longest values first so a value containing another is replaced whole.
	sort.Slice(names, func(i, j int) bool { return len(env[names[i]]) > len(env[names[j]]) })
	for _, k := range names {
		v := env[k]
		if _, err := strconv.Atoi(v); err == nil || len(v) < 8 {
			continue
		}
		text = strings.ReplaceAll(text, v, "[REDACTED:env:"+k+"]")
	}
	return text
}
