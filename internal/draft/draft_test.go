package draft

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gowtham472/handoffproof/internal/byoa"
	"github.com/gowtham472/handoffproof/internal/evidence"
)

const miniapp = "../byoa/testdata/miniapp"

func TestCheckModel(t *testing.T) {
	for _, tc := range []struct {
		id      string
		wantErr bool
	}{
		{"muse-wonder-1.0", false},
		{"muse-spark-1.3", false},
		{"muse-spark-1.3-contributor", true},
		{"Muse-Wonder-1.0-CONTRIBUTOR", true},
	} {
		if err := CheckModel(tc.id); (err != nil) != tc.wantErr {
			t.Errorf("CheckModel(%q) = %v, wantErr %v", tc.id, err, tc.wantErr)
		}
	}
}

func TestWorkspaceDropsExistingProfile(t *testing.T) {
	ws, cleanup, err := Workspace(miniapp)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(ws, byoa.ProfileFile)); !os.IsNotExist(err) {
		t.Errorf("drafting workspace still has %s (err=%v)", byoa.ProfileFile, err)
	}
	if _, err := os.Stat(filepath.Join(ws, "docker-compose.yml")); err != nil {
		t.Errorf("compose file missing from workspace: %v", err)
	}
}

func TestDecodeIsStrict(t *testing.T) {
	if _, err := Decode([]byte(`{"version":1,"id":"a","secret_value":"x"}`)); err == nil {
		t.Error("unknown field accepted")
	}
	if _, err := Decode([]byte(`{"version":1,"id":"ok"}`)); err != nil {
		t.Errorf("valid shape rejected: %v", err)
	}
}

func TestScreen(t *testing.T) {
	good, err := byoa.Load(miniapp)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"MAIL_API_TOKEN": "test-token-value", "PORT": "3000", "MODE": "production-ish"}
	withValue := func(v string) *byoa.Profile {
		p := *good
		p.Workflows = append([]byoa.Workflow(nil), good.Workflows...)
		p.Workflows[1].Steps = []byoa.Step{{Name: "h", Type: "http", Service: "app", Path: "/headers", Headers: map[string]string{"X-Test-Key": v}}}
		return &p
	}
	for _, tc := range []struct {
		name string
		p    *byoa.Profile
		want string
	}{
		{"reference profile is clean (ports and ${NAME} refs)", good, ""},
		{"literal credential value", withValue("test-token-value"), "value of MAIL_API_TOKEN"},
		{"secret-like literal", withValue("sk-live-abcdefghijklmnopqrstuvwx"), "secret-like"},
		{"non-secret env value is ignored", withValue("production-ish"), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probs := Screen(tc.p, env)
			got := strings.Join(probs, "; ")
			if tc.want == "" && got != "" {
				t.Errorf("unexpected problems: %s", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckAppliesComposeDenylist(t *testing.T) {
	dir := t.TempDir()
	if err := byoa.CopySource(miniapp, dir); err != nil {
		t.Fatal(err)
	}
	p, err := byoa.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(p, dir); err != nil {
		t.Fatalf("reference profile failed: %v", err)
	}
	compose := filepath.Join(dir, "docker-compose.yml")
	raw, _ := os.ReadFile(compose)
	if err := os.WriteFile(compose, append(raw, []byte("    privileged: true\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Check(p, dir); err == nil || !strings.Contains(err.Error(), "privileged") {
		t.Errorf("privileged compose accepted: %v", err)
	}
	p.ProbeRuntime = "bash"
	if err := Check(p, dir); err == nil {
		t.Error("invalid profile accepted")
	}
}

func TestBaseline(t *testing.T) {
	for _, tc := range []struct {
		name       string
		recs       []evidence.Record
		wantPassed bool
		wantFails  int
	}{
		{"no records never passes", nil, false, 0},
		{"all passed", []evidence.Record{{ID: "a", Passed: true}, {ID: "b", Passed: true}}, true, 0},
		{"one failure", []evidence.Record{{ID: "a", Passed: true}, {ID: "b", Workflow: "mail", Assertion: "x"}}, false, 1},
	} {
		passed, fails, ids := Baseline(tc.recs)
		if passed != tc.wantPassed || len(fails) != tc.wantFails || len(ids) != len(tc.recs) {
			t.Errorf("%s: passed=%v fails=%v ids=%v", tc.name, passed, fails, ids)
		}
	}
}

func TestScrub(t *testing.T) {
	env := map[string]string{"MAIL_API_TOKEN": "test-token-value", "PORT": "3000", "SHORT": "abc", "LONG": "test-token-value-2"}
	got := Scrub(`{"a":"test-token-value","b":"test-token-value-2","port":3000,"c":"abc"}`, env)
	want := `{"a":"[REDACTED:env:MAIL_API_TOKEN]","b":"[REDACTED:env:LONG]","port":3000,"c":"abc"}`
	if got != want {
		t.Errorf("Scrub = %s\nwant    %s", got, want)
	}
}
