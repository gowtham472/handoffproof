package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckCitations(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "src", "a.ts"), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diag := json.RawMessage(`{"findings":[
		{"finding_id":"F-ok","classification":"observed_failure","evidence_ids":["EV-1"],"source_references":["src/a.ts:2-3"]},
		{"finding_id":"F-missing","classification":"observed_failure","evidence_ids":["EV-2"],"source_references":[]},
		{"finding_id":"F-badfile","classification":"inferred_risk","evidence_ids":[],"source_references":["src/nope.ts:1"]},
		{"finding_id":"F-badline","classification":"inferred_risk","evidence_ids":[],"source_references":["src/a.ts:99"]},
		{"finding_id":"F-noev","classification":"observed_pass","evidence_ids":[],"source_references":["src/a.ts:1"]},
		{"finding_id":"F-fakeev","classification":"observed_failure","evidence_ids":["EV-fabricated"],"source_references":["src/a.ts:1"]},
		{"finding_id":"F-escape","classification":"inferred_risk","evidence_ids":[],"source_references":["../outside.txt:1"]}
	]}`)
	got, err := CheckCitations(ws, diag, map[string]bool{"EV-1": true, "EV-2": true})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"F-ok": "verified", "F-missing": "missing", "F-badfile": "invalid", "F-badline": "invalid", "F-noev": "verified",
		"F-fakeev": "verified", "F-escape": "invalid",
	}
	for _, c := range got {
		if want[c.FindingID] != c.Status {
			t.Errorf("%s status=%s want %s (%s)", c.FindingID, c.Status, want[c.FindingID], c.Detail)
		}
		if c.FindingID == "F-noev" && c.Evidence != "missing_ids" {
			t.Errorf("F-noev evidence=%s want missing_ids", c.Evidence)
		}
		if c.FindingID == "F-fakeev" && c.Evidence != "unknown_ids" {
			t.Errorf("F-fakeev evidence=%s want unknown_ids (fabricated ID must fail)", c.Evidence)
		}
	}
}
