// Package drills defines the exit drills (hp-implement-drill).
// Drills describe setup and fault injection; the controller runs them and
// writes evidence. Nothing here issues a product verdict beyond recording
// assertion outcomes as evidence records.
package drills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gowtham472/handoffproof/internal/evidence"
)

// Env is the sandbox surface the controller provides to a drill.
type Env interface {
	Register(ctx context.Context, email, name string) (int, error)
	Book(ctx context.Context, userID int, service string) (int, error)
	SinkCount(ctx context.Context, email string) (int, error)
	Upload(ctx context.Context, name string, content []byte) (id, sha string, err error)
	DocStored(ctx context.Context, docID string) (stored bool, sha string, err error)
	UserExists(ctx context.Context, email string) (bool, error)
	BookingExists(ctx context.Context, bookingID int) (bool, error)
	RevokeMailToken(ctx context.Context) error
	BackupRestore(ctx context.Context) (dbRows int, fileOK bool, err error)
	AppLogs(ctx context.Context) (string, error)
}

// Drill is one rehearsed failure.
type Drill interface {
	Name() string
	ValidateParams(p map[string]string) error
	Run(ctx context.Context, env Env, runID string, seq *int) ([]evidence.Record, error)
}

// Registry returns every supported drill by name.
func Registry() map[string]Drill {
	out := map[string]Drill{}
	for _, d := range []Drill{Baseline{}, WithdrawnToken{}, MissingConfig{}, IncompleteBackup{}} {
		out[d.Name()] = d
	}
	return out
}

// Names lists the drills in run order (baseline first, backup last).
func Names() []string {
	return []string{"fresh_deployment_baseline", "withdrawn_test_token", "missing_configuration", "incomplete_backup"}
}

func emit(recs *[]evidence.Record, runID string, seq *int, drill, workflow, assertion string, passed bool, logRef, artifactSrc string) evidence.Record {
	*seq++
	sum := sha256.Sum256([]byte(artifactSrc))
	r := evidence.Record{
		ID:             evidence.NewID(runID, drill, workflow, *seq),
		RunID:          runID,
		Drill:          drill,
		Workflow:       workflow,
		Assertion:      assertion,
		Passed:         passed,
		LogRef:         logRef,
		ArtifactSHA256: hex.EncodeToString(sum[:]),
		CreatedAt:      time.Now().UTC(),
	}
	*recs = append(*recs, r)
	return r
}

func uniqEmail(tag string) string {
	return fmt.Sprintf("%s-%d@example.test", tag, time.Now().UnixNano())
}

// sinkWait bounds how long a workflow waits for the expected sink count.
var sinkWait = 10 * time.Second

func waitSink(ctx context.Context, env Env, email string, want int) (int, error) {
	deadline := time.Now().Add(sinkWait)
	for {
		n, err := env.SinkCount(ctx, email)
		if err != nil {
			return n, err
		}
		if n == want || time.Now().After(deadline) {
			return n, nil
		}
		select {
		case <-ctx.Done():
			return n, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// runWorkflows executes register/book/email once and records assertions.
func runWorkflows(ctx context.Context, env Env, runID string, seq *int, drill, tag string) ([]evidence.Record, string, error) {
	var recs []evidence.Record
	email := uniqEmail(tag)
	uid, err := env.Register(ctx, email, "Drill User")
	if err != nil {
		return recs, email, fmt.Errorf("register: %w", err)
	}
	exists, err := env.UserExists(ctx, email)
	if err != nil {
		return recs, email, fmt.Errorf("user check: %w", err)
	}
	emit(&recs, runID, seq, drill, "registration", "user row exists after register", exists, "", email)
	bid, err := env.Book(ctx, uid, "checkup")
	if err != nil {
		return recs, email, fmt.Errorf("book: %w", err)
	}
	bok, err := env.BookingExists(ctx, bid)
	if err != nil {
		return recs, email, fmt.Errorf("booking check: %w", err)
	}
	emit(&recs, runID, seq, drill, "booking", "booking row exists after booking", bok, "", fmt.Sprint(bid))
	n, err := waitSink(ctx, env, email, 1)
	if err != nil {
		return recs, email, fmt.Errorf("sink check: %w", err)
	}
	emit(&recs, runID, seq, drill, "confirmation_email", "exactly one confirmation email in sink", n == 1, "", fmt.Sprint(n))
	return recs, email, nil
}

func allowKeys(p map[string]string, allowed ...string) error {
	var bad []string
	for k := range p {
		ok := false
		for _, a := range allowed {
			if k == a {
				ok = true
			}
		}
		if !ok {
			bad = append(bad, k)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("unsupported parameters: %s", strings.Join(bad, ", "))
	}
	return nil
}

// Baseline is the fresh-deployment gate for every other drill.
type Baseline struct{}

func (Baseline) Name() string { return "fresh_deployment_baseline" }
func (Baseline) ValidateParams(p map[string]string) error {
	return allowKeys(p)
}
func (b Baseline) Run(ctx context.Context, env Env, runID string, seq *int) ([]evidence.Record, error) {
	recs, _, err := runWorkflows(ctx, env, runID, seq, b.Name(), "base")
	return recs, err
}

// WithdrawnToken revokes the mail credential, then re-runs the workflows.
// Expected: booking still succeeds, the confirmation email never arrives.
type WithdrawnToken struct{}

func (WithdrawnToken) Name() string { return "withdrawn_test_token" }
func (WithdrawnToken) ValidateParams(p map[string]string) error {
	if err := allowKeys(p, "target_credential_name", "workflow_expected_to_break", "expected_symptom"); err != nil {
		return err
	}
	if v, ok := p["target_credential_name"]; ok && v != "MAIL_API_TOKEN" {
		return fmt.Errorf("target_credential_name must be MAIL_API_TOKEN")
	}
	if v, ok := p["workflow_expected_to_break"]; ok && v != "confirmation_email" {
		return fmt.Errorf("workflow_expected_to_break must be confirmation_email")
	}
	return nil
}
func (d WithdrawnToken) Run(ctx context.Context, env Env, runID string, seq *int) ([]evidence.Record, error) {
	var recs []evidence.Record
	base, _, err := runWorkflows(ctx, env, runID, seq, d.Name(), "pre")
	if err != nil {
		return append(recs, base...), err
	}
	recs = append(recs, base...)
	if err := env.RevokeMailToken(ctx); err != nil {
		return recs, fmt.Errorf("revoke: %w", err)
	}
	email := uniqEmail("post")
	uid, err := env.Register(ctx, email, "Drill User")
	if err != nil {
		return recs, fmt.Errorf("register after revoke: %w", err)
	}
	bid, err := env.Book(ctx, uid, "checkup")
	if err != nil {
		return recs, fmt.Errorf("book after revoke: %w", err)
	}
	bok, err := env.BookingExists(ctx, bid)
	if err != nil {
		return recs, fmt.Errorf("booking check: %w", err)
	}
	emit(&recs, runID, seq, d.Name(), "booking", "booking row exists after token withdrawal", bok, "", fmt.Sprint(bid))
	n, err := waitSink(ctx, env, email, 1)
	if err != nil {
		return recs, fmt.Errorf("sink check: %w", err)
	}
	// After withdrawal the healthy outcome is zero messages: the assertion
	// "email delivered" must be recorded as failed.
	emit(&recs, runID, seq, d.Name(), "confirmation_email", "exactly one confirmation email in sink", n == 1, "", fmt.Sprint(n))
	return recs, nil
}

// MissingConfig starts from documented configuration only. The controller
// starts this sandbox without SCHEDULER_TOKEN; workflows must still pass and
// the disabled reminder job must be visible in the app log.
type MissingConfig struct{}

func (MissingConfig) Name() string { return "missing_configuration" }
func (MissingConfig) ValidateParams(p map[string]string) error {
	if err := allowKeys(p, "suspected_undocumented_variables", "expected_symptom"); err != nil {
		return err
	}
	if v, ok := p["suspected_undocumented_variables"]; ok && !strings.Contains(v, "SCHEDULER_TOKEN") {
		return fmt.Errorf("suspected_undocumented_variables must include SCHEDULER_TOKEN")
	}
	return nil
}
func (d MissingConfig) Run(ctx context.Context, env Env, runID string, seq *int) ([]evidence.Record, error) {
	recs, _, err := runWorkflows(ctx, env, runID, seq, d.Name(), "cfg")
	if err != nil {
		return recs, err
	}
	logs, err := env.AppLogs(ctx)
	if err != nil {
		return recs, fmt.Errorf("app logs: %w", err)
	}
	disabled := strings.Contains(logs, "SCHEDULER_TOKEN is not set")
	emit(&recs, runID, seq, d.Name(), "reminder_job", "reminder job enabled with documented configuration only", !disabled, "app log", logs)
	return recs, nil
}

// IncompleteBackup uploads a document, runs backup + restore into a fresh
// sandbox, then checks record counts and the uploaded file.
type IncompleteBackup struct{}

func (IncompleteBackup) Name() string { return "incomplete_backup" }
func (IncompleteBackup) ValidateParams(p map[string]string) error {
	return allowKeys(p, "data_stores_in_use", "stores_covered_by_backup_script", "suspected_gaps", "expected_symptom")
}
func (d IncompleteBackup) Run(ctx context.Context, env Env, runID string, seq *int) ([]evidence.Record, error) {
	var recs []evidence.Record
	content := []byte("handoff-backup-probe")
	id, sha, err := env.Upload(ctx, "probe.txt", content)
	if err != nil {
		return recs, fmt.Errorf("upload: %w", err)
	}
	stored, gotSHA, err := env.DocStored(ctx, id)
	if err != nil {
		return recs, fmt.Errorf("doc check: %w", err)
	}
	emit(&recs, runID, seq, d.Name(), "documents", "uploaded file stored before backup", stored && gotSHA == sha, "", id)
	rows, fileOK, err := env.BackupRestore(ctx)
	if err != nil {
		return recs, fmt.Errorf("backup/restore: %w", err)
	}
	emit(&recs, runID, seq, d.Name(), "documents", "database rows restored after backup", rows > 0, "", fmt.Sprint(rows))
	emit(&recs, runID, seq, d.Name(), "documents", "uploaded file restored after backup", fileOK, "", id)
	return recs, nil
}
