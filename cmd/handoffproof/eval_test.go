package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCountCitationsFromVerificationFile(t *testing.T) {
	dir := t.TempDir()
	doc := `{"verdicts":[
		{"finding_id":"A","citation_ok":true,"evidence_ok":true,"issues":[]},
		{"finding_id":"B","citation_ok":false,"evidence_ok":true,"issues":["citation missing in file"]},
		{"finding_id":"C","citation_ok":true,"evidence_ok":false,"issues":[]}
	]}`
	if err := os.WriteFile(filepath.Join(dir, "verification.json"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	c := countCitations(dir)
	if !c.Found || c.Total != 3 || c.Verified != 1 || c.Invalid != 1 || c.UnknownIDs != 1 || c.Missing != 1 {
		t.Fatalf("counts = %+v", c)
	}
	if got := countCitations(t.TempDir()); got.Found {
		t.Fatalf("missing file must not be found: %+v", got)
	}
}

func TestExtractDependencyMapAndRecall(t *testing.T) {
	log := "noise\n--- dependency_map (tokens in=1 out=2 total=3) ---\n" +
		`{"dependencies":[{"id":"scheduler-token","kind":"environment_variable","name":"SCHEDULER_TOKEN undocumented","source_references":[{"path":"src/index.ts"}]}]}` + "\ntrailer"
	deps, ok := extractDependencyMap(log)
	if !ok || len(deps) != 1 {
		t.Fatalf("deps=%v ok=%v", deps, ok)
	}
	truth := []truthFault{
		{ID: "F1", Kind: "missing_configuration", Description: "Code reads SCHEDULER_TOKEN but docs never mention it."},
		{ID: "F5", Kind: "prompt_injection", Description: "Harmless instruction in a comment."},
	}
	matched, observable, details := mapRecall(deps, truth)
	if matched != 1 || observable != 1 || !strings.Contains(strings.Join(details, ";"), "F1 -> scheduler-token") {
		t.Fatalf("matched=%d observable=%d details=%v", matched, observable, details)
	}
}

func TestPhaseStatsFromLogs(t *testing.T) {
	dir := t.TempDir()
	log := "--- diagnosis (tokens in=10 out=5 total=15) ---\n{}\nphase intake_summary: schema validation failed after retries: x\n"
	if err := os.WriteFile(filepath.Join(dir, "live-full.log"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	stats, retries, _ := phaseStats(dir)
	if len(stats) != 1 || stats[0].Phase != "diagnosis" || retries != 1 {
		t.Fatalf("stats=%v retries=%d", stats, retries)
	}
}
