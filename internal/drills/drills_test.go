package drills

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gowtham472/handoffproof/internal/evidence"
)

func TestValidateParams(t *testing.T) {
	tests := []struct {
		drill string
		p     map[string]string
		ok    bool
	}{
		{"withdrawn_test_token", map[string]string{"target_credential_name": "MAIL_API_TOKEN"}, true},
		{"withdrawn_test_token", map[string]string{"target_credential_name": "OTHER"}, false},
		{"withdrawn_test_token", map[string]string{"evil": "x"}, false},
		{"missing_configuration", map[string]string{"suspected_undocumented_variables": "SCHEDULER_TOKEN"}, true},
		{"missing_configuration", map[string]string{"suspected_undocumented_variables": "OTHER"}, false},
		{"fresh_deployment_baseline", map[string]string{}, true},
		{"fresh_deployment_baseline", map[string]string{"x": "y"}, false},
		{"incomplete_backup", map[string]string{"suspected_gaps": "uploads"}, true},
	}
	reg := Registry()
	for _, tc := range tests {
		err := reg[tc.drill].ValidateParams(tc.p)
		if tc.ok && err != nil {
			t.Errorf("%s %v: unexpected error %v", tc.drill, tc.p, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s %v: want error", tc.drill, tc.p)
		}
	}
}

type fakeEnv struct {
	revoked bool
	users   map[string]int
	books   map[int]int
	nextID  int
	logs    string
}

func newFake() *fakeEnv {
	return &fakeEnv{users: map[string]int{}, books: map[int]int{}, logs: "SCHEDULER_TOKEN is not set; reminder job is disabled"}
}
func (f *fakeEnv) Register(_ context.Context, email, _ string) (int, error) {
	f.nextID++
	f.users[email] = f.nextID
	return f.nextID, nil
}
func (f *fakeEnv) Book(_ context.Context, userID int, _ string) (int, error) {
	f.nextID++
	f.books[f.nextID] = userID
	return f.nextID, nil
}
func (f *fakeEnv) SinkCount(_ context.Context, email string) (int, error) {
	if f.revoked {
		return 0, nil
	}
	for e := range f.users {
		if e == email {
			return 1, nil
		}
	}
	return 0, nil
}
func (f *fakeEnv) Upload(_ context.Context, _ string, _ []byte) (string, string, error) {
	return "doc-1", "abc", nil
}
func (f *fakeEnv) DocStored(_ context.Context, _ string) (bool, string, error) {
	return true, "abc", nil
}
func (f *fakeEnv) UserExists(_ context.Context, email string) (bool, error) {
	_, ok := f.users[email]
	return ok, nil
}
func (f *fakeEnv) BookingExists(_ context.Context, id int) (bool, error) {
	_, ok := f.books[id]
	return ok, nil
}
func (f *fakeEnv) RevokeMailToken(_ context.Context) error { f.revoked = true; return nil }
func (f *fakeEnv) BackupRestore(_ context.Context) (int, bool, error) {
	return 3, false, nil
}
func (f *fakeEnv) AppLogs(_ context.Context) (string, error) { return f.logs, nil }

func findRec(recs []evidence.Record, workflow string, passed bool) *evidence.Record {
	for i := range recs {
		if recs[i].Workflow == workflow && recs[i].Passed == passed {
			return &recs[i]
		}
	}
	return nil
}

func TestWithdrawnTokenFakeEnv(t *testing.T) {
	sinkWait = 200 * time.Millisecond
	defer func() { sinkWait = 10 * time.Second }()
	seq := 0
	recs, err := WithdrawnToken{}.Run(context.Background(), newFake(), "run-test", &seq)
	if err != nil {
		t.Fatal(err)
	}
	if findRec(recs, "booking", true) == nil {
		t.Error("want observed booking pass after withdrawal")
	}
	fail := findRec(recs, "confirmation_email", false)
	if fail == nil {
		t.Fatal("want observed confirmation_email failure after withdrawal")
	}
	if fail.ID == "" || !strings.HasPrefix(fail.ID, "EV-") {
		t.Errorf("failure record ID = %q, want EV- prefix", fail.ID)
	}
}

func TestMissingConfigFakeEnv(t *testing.T) {
	seq := 0
	recs, err := MissingConfig{}.Run(context.Background(), newFake(), "run-test", &seq)
	if err != nil {
		t.Fatal(err)
	}
	if findRec(recs, "reminder_job", false) == nil {
		t.Error("want reminder_job failure when scheduler token missing")
	}
	if findRec(recs, "registration", true) == nil {
		t.Error("want registration pass under missing config")
	}
}
