package score

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFaultsFromGroundTruthFile(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"evidence", "demo-app", "eval"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	truth := `{"planted_faults":[{"id":"X1","kind":"silent_failure"},{"id":"X2","kind":"documentation_gap"},{"id":"X3","kind":"unknown_kind"}]}`
	if err := os.WriteFile(filepath.Join(root, "eval", "ground_truth.json"), []byte(truth), 0o644); err != nil {
		t.Fatal(err)
	}
	recs := `[{"evidence_id":"EV-2","run_id":"r1","drill":"withdrawn_test_token","workflow":"confirmation_email","assertion":"b","passed":false,"artifact_sha256":"","created_at":"2026-01-01T00:00:01Z"}]`
	if err := os.WriteFile(filepath.Join(root, "evidence", "run-r1.json"), []byte(recs), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Compute(root)
	// 1 of 2 observable faults caught; unknown kind excluded, not scored.
	if res.Components["faults"] != 50 {
		t.Fatalf("faults = %d, want 50 from ground-truth file", res.Components["faults"])
	}
	if _, ok := res.Components["backup"]; ok {
		t.Fatalf("backup must be omitted when no data stores are detected, got %v", res.Components)
	}
}

func TestCombineWeights(t *testing.T) {
	if got := Combine(100, 100, 100, 100); got != 100 {
		t.Fatalf("Combine(all 100) = %d, want 100", got)
	}
	if got := Combine(0, 0, 0, 0); got != 0 {
		t.Fatalf("Combine(all 0) = %d, want 0", got)
	}
	if got := Combine(100, 0, 0, 0); got != 40 {
		t.Fatalf("Combine(workflows only) = %d, want 40", got)
	}
}

func TestComputeEmptyRoot(t *testing.T) {
	res := Compute(t.TempDir())
	if res.Total < 0 || res.Total > 100 {
		t.Fatalf("total out of range: %d", res.Total)
	}
	if res.Components == nil {
		t.Fatal("components must be present")
	}
}

func TestComputeCountsEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "demo-app"), 0o755); err != nil {
		t.Fatal(err)
	}
	recs := `[
	 {"evidence_id":"EV-1","run_id":"r1","drill":"fresh_deployment_baseline","workflow":"registration","assertion":"a","passed":true,"artifact_sha256":"","created_at":"2026-01-01T00:00:00Z"},
	 {"evidence_id":"EV-2","run_id":"r1","drill":"withdrawn_test_token","workflow":"confirmation_email","assertion":"b","passed":false,"artifact_sha256":"","created_at":"2026-01-01T00:00:01Z"}
	]`
	if err := os.WriteFile(filepath.Join(root, "evidence", "run-r1.json"), []byte(recs), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Compute(root)
	if res.Components["workflows"] != 50 {
		t.Fatalf("workflows = %d, want 50 (1 of 2 passed)", res.Components["workflows"])
	}
}
