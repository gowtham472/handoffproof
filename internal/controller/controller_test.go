package controller

import (
	"strings"
	"testing"
)

func TestEnsureDiffHeaders(t *testing.T) {
	in := "--- a/app/x.md\n+++ b/app/x.md\n@@ -1 +1 @@\n-a\n+b\n"
	out := EnsureDiffHeaders(in)
	if !strings.HasPrefix(out, "diff --git a/app/x.md b/app/x.md\n") {
		t.Errorf("missing synthesized header:\n%s", out)
	}
	already := "diff --git a/app/x.md b/app/x.md\n" + in
	if got := EnsureDiffHeaders(already); strings.Count(got, "diff --git") != 1 {
		t.Errorf("duplicated header:\n%s", got)
	}
}

func TestChangedPathsEdgeCases(t *testing.T) {
	diff := "diff --git a/app/old name.md b/app/new name.md\n" +
		"similarity index 90%\nrename from app/old name.md\nrename to app/new name.md\n" +
		"--- a/app/old name.md\n+++ b/app/new name.md\n@@ -1 +1 @@\n-a\n+b\n" +
		"diff --git a/app/gone.txt b/app/gone.txt\n--- a/app/gone.txt\n+++ /dev/null\n" +
		"--- \"a/app/quoted file.txt\"\n+++ \"b/app/quoted file.txt\"\n" +
		"--- a/./app/dot.txt\n+++ b/APP/DOT.txt\n"
	got := ChangedPaths(diff)
	joined := strings.Join(got, "|")
	for _, want := range []string{"app/old name.md", "app/new name.md", "app/gone.txt", "app/quoted file.txt", "app/dot.txt"} {
		if !strings.Contains(joined, want) {
			t.Errorf("ChangedPaths missing %q in %v", want, got)
		}
	}
	for _, p := range got {
		if p == "/dev/null" || strings.HasPrefix(p, `"`) || strings.HasPrefix(p, "./") {
			t.Errorf("ChangedPaths returned unnormalized path %q", p)
		}
	}
	// Case-only duplicates collapse to one entry.
	count := 0
	for _, p := range got {
		if strings.EqualFold(p, "app/dot.txt") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("case variants not deduped: %v", got)
	}
}

func TestDiffTouchesProtected(t *testing.T) {
	tests := []struct {
		name string
		diff string
		want bool
	}{
		{"app config ok", "--- a/.env.example\n+++ b/.env.example\n@@\n-OLD\n+NEW\n", false},
		{"acceptance blocked", "--- a/acceptance/booking.spec.ts\n+++ b/acceptance/booking.spec.ts\n", true},
		{"controller blocked", "+++ b/internal/controller/controller.go\n", true},
		{"drills blocked", "+++ b/internal/drills/drills.go\n", true},
		{"evidence code blocked", "+++ b/internal/evidence/evidence.go\n", true},
		{"evidence data blocked", "+++ b/evidence/run.json\n", true},
		{"manifest blocked", "+++ b/.manifest.sha256\n", true},
		{"manifest script blocked", "+++ b/scripts/manifest.sh\n", true},
		{"traversal blocked", "+++ b/../secret.txt\n", true},
		{"docs ok", "+++ b/docs/HANDOVER.md\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := DiffTouchesProtected(tc.diff)
			if got != tc.want {
				t.Errorf("DiffTouchesProtected = %v, want %v", got, tc.want)
			}
		})
	}
}
