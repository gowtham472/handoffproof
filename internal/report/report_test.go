package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gowtham472/handoffproof/internal/evidence"
)

func boolp(b bool) *bool { return &b }

func TestRenderEscapesAndLinks(t *testing.T) {
	d := Data{
		Checks: []Check{{Name: "confirmation email", Class: evidence.ObservedFailure,
			BeforeID: "EV-abc", BeforePassed: boolp(false), Note: "silent failure"}},
		Findings: []Finding{{ID: "F-1", Class: evidence.ObservedFailure, Workflow: "confirmation_email",
			EvidenceIDs: []string{"EV-abc"}, FailedStep: "email delivered",
			LogExcerpt: "x <script>alert(1)</script> Bearer abcdefgh12345678",
			SourceRef:  "src/index.ts:71", RootCause: "token withdrawn", BusinessHit: "no confirmations", Confidence: 0.93}},
		Evidence:   []evidence.Record{{ID: "EV-abc", Drill: "withdrawn_test_token", Workflow: "confirmation_email", Assertion: "email in sink", Passed: false}},
		HashBefore: "aaa", HashAfter: "aaa", HashMatch: true,
	}
	var buf bytes.Buffer
	if err := Render(&buf, d); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Error("log excerpt was not escaped")
	}
	if strings.Contains(html, "abcdefgh12345678") {
		t.Error("secret-like token leaked into report")
	}
	if !strings.Contains(html, `href="#EV-abc"`) || !strings.Contains(html, `id="EV-abc"`) {
		t.Error("finding is not linked to its evidence record")
	}
	if !strings.Contains(html, "unchanged") {
		t.Error("integrity footer missing match state")
	}
}
