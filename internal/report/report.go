// Package report renders the single-file HTML evidence report
// (hp-evidence-report). Self-contained, offline, escaped and redacted.
package report

import (
	"fmt"
	htmltemplate "html/template"
	"io"
	"strings"

	"github.com/gowtham472/handoffproof/internal/agent"
	"github.com/gowtham472/handoffproof/internal/evidence"
)

// Check is one summary-table row.
type Check struct {
	Name         string
	BeforeID     string
	AfterID      string
	BeforePassed *bool
	AfterPassed  *bool
	Class        evidence.Classification
	Note         string
}

// Finding is one diagnosed finding with its evidence links.
type Finding struct {
	ID          string
	Class       evidence.Classification
	Workflow    string
	EvidenceIDs []string
	FailedStep  string
	LogExcerpt  string
	SourceRef   string
	RootCause   string
	BusinessHit string
	Confidence  float64
}

// Data is everything the report renders.
type Data struct {
	Title      string
	Checks     []Check
	Findings   []Finding
	Inferred   []Finding
	Untested   []Finding
	Evidence   []evidence.Record
	HashBefore string
	HashAfter  string
	HashMatch  bool
}

func classBadge(c evidence.Classification) string {
	switch c {
	case evidence.ObservedPass:
		return "pass"
	case evidence.ObservedFailure:
		return "fail"
	case evidence.InferredRisk:
		return "risk"
	default:
		return "untested"
	}
}

func outcome(p *bool) string {
	if p == nil {
		return "not run"
	}
	if *p {
		return "passed"
	}
	return "failed"
}

// Render writes the full HTML report.
func Render(w io.Writer, d Data) error {
	funcs := htmltemplate.FuncMap{
		"badge":   classBadge,
		"outcome": outcome,
		"join":    strings.Join,
		"redact":  agent.Redact,
	}
	t, err := htmltemplate.New("report").Funcs(funcs).Parse(page)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}
	if d.Title == "" {
		d.Title = "HandoffProof evidence report"
	}
	return t.Execute(w, d)
}

const page = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
body{font-family:system-ui,Segoe UI,Arial,sans-serif;margin:2rem;color:#111;background:#fff;line-height:1.45}
h1{font-size:1.6rem} h2{margin-top:2rem}
table{border-collapse:collapse;width:100%}
th,td{border:1px solid #999;padding:.45rem .6rem;text-align:left;vertical-align:top}
.badge{display:inline-block;padding:.1rem .5rem;border-radius:1rem;color:#fff;font-size:.85rem}
.pass{background:#137333}.fail{background:#b3261e}.risk{background:#b06000}.untested{background:#5f6368}
.card{border:1px solid #999;border-radius:.5rem;padding:1rem;margin:1rem 0}
code{background:#f1f1f1;padding:.1rem .3rem}
footer{margin-top:2rem;border-top:2px solid #111;padding-top:1rem}
.log{white-space:pre-wrap;background:#f7f7f7;padding:.6rem;border-radius:.4rem}
</style></head><body>
<h1>{{.Title}}</h1>
<p>Pass and fail are decided by the controller. Observed rows always cite an evidence ID.</p>
<h2>Summary</h2>
<table><tr><th>Check</th><th>Class</th><th>Before</th><th>After</th><th>Note</th></tr>
{{range .Checks}}<tr><td>{{redact .Name}}</td><td><span class="badge {{badge .Class}}">{{.Class}}</span></td>
<td>{{outcome .BeforePassed}}{{if .BeforeID}} (<a href="#{{.BeforeID}}">{{.BeforeID}}</a>){{end}}</td>
<td>{{outcome .AfterPassed}}{{if .AfterID}} (<a href="#{{.AfterID}}">{{.AfterID}}</a>){{end}}</td><td>{{redact .Note}}</td></tr>
{{end}}</table>
<h2>Observed findings</h2>
{{range .Findings}}<div class="card" id="{{index .EvidenceIDs 0}}">
<strong>{{.ID}}</strong> <span class="badge {{badge .Class}}">{{.Class}}</span> workflow: {{redact .Workflow}}<br>
Failed step: {{redact .FailedStep}}<br>Evidence: {{join .EvidenceIDs ", "}}<br>
Source: <code>{{redact .SourceRef}}</code><br>Root cause: {{redact .RootCause}}<br>
Business impact: {{redact .BusinessHit}}<br>Confidence: {{printf "%.2f" .Confidence}}
<div class="log">{{redact .LogExcerpt}}</div></div>
{{end}}
<h2>Inferred risks (not observed)</h2>
{{range .Inferred}}<div class="card"><strong>{{.ID}}</strong> <span class="badge risk">{{.Class}}</span>
workflow: {{redact .Workflow}}<br>{{redact .RootCause}}<br>Confidence: {{printf "%.2f" .Confidence}}</div>{{end}}
<h2>Untested dependencies (not observed)</h2>
{{range .Untested}}<div class="card"><strong>{{.ID}}</strong> <span class="badge untested">{{.Class}}</span>
workflow: {{redact .Workflow}}<br>{{redact .RootCause}}</div>{{end}}
<h2>Evidence records</h2>
<table><tr><th>Evidence ID</th><th>Drill</th><th>Workflow</th><th>Assertion</th><th>Result</th></tr>
{{range .Evidence}}<tr id="{{.ID}}"><td>{{.ID}}</td><td>{{redact .Drill}}</td><td>{{redact .Workflow}}</td>
<td>{{redact .Assertion}}</td><td>{{if .Passed}}passed{{else}}failed{{end}}</td></tr>
{{end}}</table>
<footer>Acceptance integrity: manifest <code>{{.HashBefore}}</code>:
{{if .HashMatch}}<strong>verified unchanged</strong>{{else}}<strong>MISMATCH — do not trust this report</strong>{{end}}</footer>
</body></html>`
