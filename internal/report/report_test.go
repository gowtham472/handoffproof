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
			SourceRef:  "src/index.ts:71", SourceRefs: []string{"src/index.ts:68-73", "src/mail.ts:2-16"}, RootCause: "token withdrawn", BusinessHit: "no confirmations", Confidence: 0.93},
			{ID: "F-2", Class: evidence.ObservedFailure, Workflow: "confirmation_email",
				EvidenceIDs: []string{"EV-def"}, FailedStep: "email delivered",
				RootCause: "source missing case", BusinessHit: "no confirmations", Confidence: 0.5}},
		Evidence: []evidence.Record{
			{ID: "EV-abc", Drill: "withdrawn_test_token", Workflow: "confirmation_email", Assertion: "email in sink", Passed: false},
			{ID: "EV-def", Drill: "withdrawn_test_token", Workflow: "confirmation_email", Assertion: "email in sink", Passed: false},
		},
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
	for _, want := range []string{"src/index.ts:68-73", "src/mail.ts:2-16", "Not provided — citation missing"} {
		if !strings.Contains(html, want) {
			t.Errorf("finding source display %q missing", want)
		}
	}
	if !strings.Contains(html, "unchanged") {
		t.Error("integrity footer missing match state")
	}
	for _, want := range []string{
		`id="btn-copy"`, `id="btn-dl"`, `id="btn-slack"`, `id="btn-slack-text"`, `/api/integrations/slack/connect?from=report`, // share + Slack connect actions
		`window.print()`, `@media print`, "Save as PDF", // PDF path
		`id="evidence-records"`, `id="report-pagination"`, `id="report-page-size"`,
		`id="report-pages"`, `paginated-hidden`, `.paginated-hidden{display:table-row!important}`, // evidence pagination; print shows every row
		`"IBM Plex Sans"`, `"IBM Plex Mono"`, `--blue:#0f62fe`, `--purple:#6929c4`, // application type and palette
		`class="report-header"`, `class="hero"`, `class="report-nav"`, `class="stats"`, // Control Room report structure
		`id="i-chart"`, `id="i-search"`, `id="i-shield"`, `id="i-slack"`, `viewBox="0 0 500 500"`, // inline Carbon/brand icons + embedded illustrations
		`id="summary"`, `id="findings"`, `id="risks"`, `id="untested"`, `id="evidence"`, `id="integrity"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("share feature %q missing from report", want)
		}
	}
}
