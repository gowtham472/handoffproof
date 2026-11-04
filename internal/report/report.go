// Package report renders the single-file HTML evidence report
// (hp-evidence-report). Self-contained, offline, escaped and redacted.
package report

import (
	_ "embed"
	"fmt"
	htmltemplate "html/template"
	"io"
	"strings"

	"github.com/gowtham472/handoffproof/internal/agent"
	"github.com/gowtham472/handoffproof/internal/evidence"
)

// Storyset illustrations, recolored to the application palette and embedded
// so the single-file report keeps the Control Room artwork offline.
//
//go:embed assets/evidence.svg
var evidenceArt []byte

//go:embed assets/insights.svg
var insightsArt []byte

//go:embed assets/handover.svg
var handoverArt []byte

func inlineSVG(raw []byte) htmltemplate.HTML {
	// The illustrations are first-party embedded assets, checked for script
	// content by the report tests; inlining keeps the report self-contained.
	return htmltemplate.HTML(raw)
}

type pageData struct {
	Data
	EvidenceArt htmltemplate.HTML
	InsightsArt htmltemplate.HTML
	HandoverArt htmltemplate.HTML
}

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
	SourceRefs  []string
	RootCause   string
	BusinessHit string
	Confidence  float64
}

// InterviewClaim is one exit-interview claim with its deterministic status.
type InterviewClaim struct {
	ID, Speaker, Timestamp, Type, Subject, Statement, Status, Detail string
	EvidenceIDs                                                      []string
	NeedsRunbook                                                     bool
}

// InterviewSection carries the exit-interview panel. Statuses come from
// drill evidence and the static scan, never from the model.
type InterviewSection struct {
	App, Model, Started string
	Dropped             []string
	Claims              []InterviewClaim
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
	Interview  *InterviewSection
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
		"passed": func(records []evidence.Record) int {
			n := 0
			for _, r := range records {
				if r.Passed {
					n++
				}
			}
			return n
		},
		"failed": func(records []evidence.Record) int {
			n := 0
			for _, r := range records {
				if !r.Passed {
					n++
				}
			}
			return n
		},
	}
	t, err := htmltemplate.New("report").Funcs(funcs).Parse(page)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}
	if d.Title == "" {
		d.Title = "Evidence Report"
	}
	return t.Execute(w, pageData{
		Data:        d,
		EvidenceArt: inlineSVG(evidenceArt),
		InsightsArt: inlineSVG(insightsArt),
		HandoverArt: inlineSVG(handoverArt),
	})
}

const page = `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@300;400;600;700&family=IBM+Plex+Mono:wght@400;600&display=swap" rel="stylesheet">
<style>
:root{--bg:#f4f4f4;--layer:#fff;--layer-2:#e0e0e0;--dark:#161616;--dark-2:#262626;--text:#161616;--text-2:#525252;--text-3:#6f6f6f;--blue:#0f62fe;--blue-hover:#0353e9;--purple:#6929c4;--purple-2:#8a3ffc;--border:#e0e0e0;--border-strong:#8d8d8d;--green-bg:#defbe6;--green:#0e6027;--red-bg:#fff1f1;--red:#a2191f;--warm-bg:#fdf6dd;--warm:#8e6a00;--gray-bg:#e0e0e0;--gray:#393939}
*{box-sizing:border-box}html{scroll-behavior:smooth}body{font-family:"IBM Plex Sans",system-ui,sans-serif;margin:0;color:var(--text);background:var(--bg);line-height:1.5}.mono,code{font-family:"IBM Plex Mono",ui-monospace,monospace}.ic{width:18px;height:18px;fill:currentColor;flex:none;vertical-align:-3px}.brand-ic{width:22px;height:22px;flex:none}a{color:var(--blue)}button:focus-visible,a:focus-visible,select:focus-visible{outline:2px solid var(--blue);outline-offset:1px}
.report-header{position:sticky;top:0;z-index:50;height:48px;display:flex;align-items:center;background:var(--layer);border-bottom:1px solid var(--border)}.brand{display:flex;align-items:center;height:100%;padding:0 24px 0 16px;color:var(--text);font-size:.95rem;white-space:nowrap;text-decoration:none}.brand b{font-weight:700;margin-right:7px}.brand span{color:var(--text-2)}.header-integrity{margin-left:auto;display:flex;align-items:center;gap:8px;padding:0 16px;color:var(--text-2);font-size:.82rem;white-space:nowrap}.status-dot{width:8px;height:8px;border-radius:50%;background:#24a148;flex:none}.status-dot.bad{background:var(--red)}
.hero{background:var(--dark);color:#fff;position:relative;overflow:hidden}.hero:before{content:"";position:absolute;inset:0 0 auto 0;height:4px;background:linear-gradient(90deg,var(--purple) 0%,var(--purple-2) 45%,var(--blue) 100%)}.hero-inner{max-width:1312px;margin:0 auto;padding:48px 32px 40px;display:grid;grid-template-columns:minmax(0,1.2fr) minmax(260px,.8fr);gap:40px;align-items:center}.hero-copy{min-width:0}.hero-label{color:#a6c8ff;font-size:.9rem;margin:0 0 12px}.hero h1{font-size:2.65rem;font-weight:300;letter-spacing:-.02em;line-height:1.12;margin:0 0 16px}.hero h1 strong{font-weight:600}.hero-sub{color:#c6c6c6;font-size:1.05rem;max-width:620px;margin:0}.hero-art{background:var(--layer);padding:20px;display:flex;align-items:center;justify-content:center;min-height:250px}.hero-art .artwork{width:100%;max-width:340px}.hero-art svg{width:100%;height:auto;display:block}
.actions{display:flex;gap:10px;flex-wrap:wrap;margin-top:24px}.actions button{display:inline-flex;align-items:center;gap:9px;background:var(--blue);color:#fff;border:1px solid var(--blue);padding:12px 20px;font:inherit;font-weight:600;font-size:.95rem;cursor:pointer}.actions button:hover{background:var(--blue-hover)}.actions button.ghost{background:transparent;color:#fff;border-color:#fff}.actions button.ghost:hover{background:#393939}.actions button:disabled{opacity:.5;cursor:wait}#sharemsg{color:#a6c8ff;font-size:.88rem;margin-top:12px;min-height:1.25em}
.pagewrap{max-width:1312px;margin:0 auto;padding:32px 32px 64px}.report-nav{position:sticky;top:48px;z-index:40;display:flex;gap:2px;overflow-x:auto;background:var(--layer);border-bottom:1px solid var(--border);margin-bottom:16px}.report-nav a{padding:12px 16px;color:var(--text-2);font-weight:600;font-size:.9rem;text-decoration:none;white-space:nowrap;border-bottom:3px solid transparent}.report-nav a:hover{color:var(--text);background:var(--bg)}
.stats{display:grid;grid-template-columns:repeat(4,1fr);gap:20px;background:var(--layer);padding:26px 28px;border-bottom:1px solid var(--border);margin-bottom:16px}.stat{border-top:2px solid var(--dark);padding-top:10px;min-width:0}.stat.purple{border-top-color:var(--purple)}.stat .v{font-size:2.1rem;font-weight:300;letter-spacing:-.02em;line-height:1.1}.stat .l{color:var(--text-2);font-size:.88rem;margin-top:2px}
.section{background:var(--layer);padding:24px;margin-bottom:16px;scroll-margin-top:112px}.section-head{display:flex;gap:12px;align-items:flex-start;margin-bottom:18px}.section-icon{width:40px;height:40px;display:grid;place-items:center;background:var(--bg);color:var(--blue);flex:none}.section-icon.purple{color:var(--purple)}.section-head h2{font-size:1.25rem;font-weight:600;margin:0;line-height:1.35}.section-head p{color:var(--text-2);font-size:.92rem;margin:3px 0 0}.section-art{margin-left:auto;width:112px;max-width:24%;height:auto;flex:none}.section-art svg{width:100%;height:auto;display:block}.table-wrap{overflow-x:auto}table{border-collapse:collapse;width:100%;background:var(--layer);color:var(--text);font-size:.92rem}th,td{padding:11px 14px;text-align:left;vertical-align:top;border-bottom:1px solid var(--border)}th{background:var(--bg);color:var(--text);font-size:.85rem;border-bottom:1px solid var(--border-strong)}table a{font-weight:600}
.badge{display:inline-block;padding:.18rem .6rem;border-radius:999px;font-size:.76rem;font-weight:600;white-space:nowrap}.pass{background:var(--green-bg);color:var(--green)}.fail{background:var(--red-bg);color:var(--red)}.risk{background:var(--warm-bg);color:var(--warm)}.untested{background:var(--gray-bg);color:var(--gray)}
.finding-card{border:1px solid var(--border);padding:20px;margin:0 0 12px;background:var(--layer)}.finding-top{display:flex;align-items:center;gap:10px;flex-wrap:wrap}.finding-top h3{font-size:1rem;margin:0}.finding-workflow{color:var(--text-2);font-size:.88rem}.meta-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin:16px 0}.meta-cell{background:var(--bg);padding:12px 14px;min-width:0}.meta-cell .k{color:var(--text-2);font-size:.74rem;margin-bottom:3px}.meta-cell .v{font-weight:600;font-size:.9rem;overflow-wrap:anywhere}.finding-columns{display:grid;grid-template-columns:1fr 1fr;gap:20px}.finding-columns h4{font-size:.88rem;margin:0 0 5px}.finding-columns p{margin:0;color:var(--text-2)}code{background:var(--gray-bg);color:var(--text);padding:.1rem .35rem;font-size:.85em;overflow-wrap:anywhere}.evidence-links{margin-top:14px;color:var(--text-2);font-size:.9rem}.log{white-space:pre-wrap;background:var(--bg);border:1px solid var(--border);padding:12px 14px;margin:14px 0 0;font-family:"IBM Plex Mono",ui-monospace,monospace;font-size:.84rem;overflow-wrap:anywhere}.empty{color:var(--text-2);background:var(--bg);padding:16px;margin:0}
.integrity-grid{display:grid;grid-template-columns:minmax(0,1fr) 220px;gap:32px;align-items:center}.integrity-status{font-size:1.05rem}.hash-list{display:grid;gap:8px;margin-top:14px}.hash-list div{display:flex;gap:10px;flex-wrap:wrap}.hash-list .k{color:var(--text-2);min-width:110px}.integrity-art{width:100%;max-width:220px;height:auto;justify-self:end}.integrity-art svg{width:100%;height:auto;display:block}
.pagination{display:flex;align-items:center;gap:16px;flex-wrap:wrap;min-height:48px;padding:8px 12px;background:var(--layer);border-top:1px solid var(--border-strong)}.pagination-group,.pagination-nav,.pagination-pages{display:flex;align-items:center;gap:8px}.pagination-group{color:var(--text-2);font-size:.86rem}.pagination select{min-height:32px;padding:5px 8px;font:inherit}.pagination-nav{margin-left:auto}.page-btn{min-width:32px;height:32px;padding:0 8px;border:1px solid transparent;background:transparent;color:var(--blue);font:inherit;font-size:.88rem;cursor:pointer}.page-btn:hover:not(:disabled){background:#e8efff}.page-btn.active{background:#393939;color:#fff}.page-btn:disabled{color:var(--text-3);cursor:not-allowed;opacity:.55}.pagination-ellipsis{color:var(--text-3);padding:0 2px}.paginated-hidden{display:none!important}
.report-footer{border-top:1px solid var(--border);padding-top:16px;color:var(--text-2);font-size:.88rem;display:flex;gap:9px;align-items:flex-start}.report-footer .ic{color:var(--blue);margin-top:2px}
@media(max-width:900px){.hero-inner{grid-template-columns:1fr;padding:40px 24px 32px}.hero-art{min-height:0;justify-content:flex-start}.hero-art .artwork{max-width:260px}.stats{grid-template-columns:repeat(2,1fr)}.meta-grid{grid-template-columns:repeat(2,1fr)}.finding-columns{grid-template-columns:1fr}.integrity-grid{grid-template-columns:1fr}.integrity-art{justify-self:start;max-width:180px}.section-art{display:none}}
@media(max-width:672px){.pagewrap{padding:24px 16px 48px}.header-integrity span:last-child{display:none}.hero h1{font-size:2.15rem}.stats{padding:20px}.section{padding:20px 16px}.pagination-nav{margin-left:0}.brand span{display:none}}
@media print{.report-header,.report-nav,.actions,#sharemsg,.pagination{display:none!important}.paginated-hidden{display:table-row!important}body{background:#fff}.pagewrap{max-width:100%;padding:0}.hero-inner{max-width:100%;padding:24px 0}.hero-art,.section-art,.integrity-art{display:none!important}.hero,.section,.finding-card,.stat{-webkit-print-color-adjust:exact;print-color-adjust:exact}.section,.finding-card,table,tr{break-inside:avoid}h2,h3{break-after:avoid}}
@media(prefers-reduced-motion:reduce){html{scroll-behavior:auto}}
</style></head><body>
<svg width="0" height="0" style="position:absolute" aria-hidden="true">
  <!-- IBM Carbon icons (Apache-2.0), inlined to match the Control Room. -->
  <symbol id="i-dashboard" viewBox="0 0 32 32"><path d="M24 21H26V26H24zM20 16H22V26H20zM11 26a5.0059 5.0059 0 01-5-5H8a3 3 0 103-3V16a5 5 0 010 10z"/><path d="M28,2H4A2.002,2.002,0,0,0,2,4V28a2.0023,2.0023,0,0,0,2,2H28a2.0027,2.0027,0,0,0,2-2V4A2.0023,2.0023,0,0,0,28,2Zm0,9H14V4H28ZM12,4v7H4V4ZM4,28V13H28.0007l.0013,15Z"/></symbol>
  <symbol id="i-doc" viewBox="0 0 32 32"><path d="M25.7,9.3l-7-7A.9078.9078,0,0,0,18,2H8A2.0059,2.0059,0,0,0,6,4V28a2.0059,2.0059,0,0,0,2,2H24a2.0059,2.0059,0,0,0,2-2V10A.9078.9078,0,0,0,25.7,9.3ZM18,4.4,23.6,10H18ZM24,28H8V4h8v6a2.0059,2.0059,0,0,0,2,2h6Z"/></symbol>
  <symbol id="i-search" viewBox="0 0 32 32"><path d="M29,27.5859l-7.5521-7.5521a11.0177,11.0177,0,1,0-1.4141,1.4141L27.5859,29ZM4,13a9,9,0,1,1,9,9A9.01,9.01,0,0,1,4,13Z"/></symbol>
  <symbol id="i-warn" viewBox="0 0 32 32"><path d="M16 23a1.5 1.5 0 101.5 1.5A1.5 1.5 0 0016 23zM15 12H17V21H15z"/><path d="M29,30H3a1,1,0,0,1-.8872-1.4614l13-25a1,1,0,0,1,1.7744,0l13,25A1,1,0,0,1,29,30ZM4.6507,28H27.3493l.002-.0033L16.002,6.1714h-.004L4.6487,27.9967Z"/></symbol>
  <symbol id="i-check" viewBox="0 0 32 32"><path d="M13 24L4 15 5.414 13.586 13 21.171 26.586 7.586 28 9 13 24z"/></symbol>
  <symbol id="i-arrow" viewBox="0 0 32 32"><path d="M18 6L16.57 7.393 24.15 15 4 15 4 17 24.15 17 16.57 24.573 18 26 28 16 18 6z"/></symbol>
  <!-- Official Slack mark (Slack brand CDN). -->
  <symbol id="i-slack" viewBox="0 0 54 54"><path d="M11.379 33.9993C11.379 37.1358 8.84512 39.6507 5.7276 39.6507C2.61008 39.6507 0.0572205 37.1168 0.0572205 33.9993C0.0572205 30.8817 2.5911 28.3479 5.70862 28.3479H11.36V33.9993H11.379Z" fill="#E3066A"/><path d="M14.1962 33.9997C14.1962 30.8632 16.7301 28.3483 19.8476 28.3483C22.9651 28.3483 25.499 30.8822 25.499 33.9997V48.1353C25.499 51.2718 22.9651 53.7867 19.8476 53.7867C16.7301 53.7867 14.1962 51.2718 14.1962 48.1353V33.9997Z" fill="#E3066A"/><path d="M19.8662 11.2673C16.7296 11.2673 14.2148 8.73347 14.2148 5.61594C14.2148 2.49842 16.7486 -0.0354538 19.8662 -0.0354538C22.9837 -0.0354538 25.5175 2.49842 25.5175 5.61594V11.2673H19.8662Z" fill="#00B3FF"/><path d="M19.8682 14.1334C23.0047 14.1334 25.5196 16.6673 25.5196 19.7848C25.5196 22.9023 22.9857 25.4362 19.8682 25.4362H5.67566C2.53916 25.4362 0.0242615 22.9023 0.0242615 19.7848C0.0242615 16.6673 2.55814 14.1334 5.67566 14.1334H19.8682Z" fill="#00B3FF"/><path d="M42.5323 19.7853C42.5323 16.6488 45.0662 14.1339 48.1837 14.1339C51.3012 14.1339 53.8351 16.6678 53.8351 19.7853C53.8351 22.9028 51.3012 25.4367 48.1837 25.4367H42.5323V19.7853Z" fill="#41B658"/><path d="M39.7126 19.7934C39.7126 22.9299 37.1787 25.4448 34.0612 25.4448C30.9436 25.4448 28.4098 22.911 28.4098 19.7934V5.61986C28.4098 2.48336 30.9436 -0.0315399 34.0612 -0.0315399C37.1787 -0.0315399 39.7126 2.48336 39.7126 5.61986V19.7934Z" fill="#41B658"/><path d="M34.0376 42.482C37.1741 42.482 39.689 45.0158 39.689 48.1334C39.689 51.2509 37.1552 53.7848 34.0376 53.7848C30.9201 53.7848 28.3862 51.2509 28.3862 48.1334V42.482H34.0376Z" fill="#FCC003"/><path d="M34.0381 39.6507C30.9016 39.6507 28.3867 37.1168 28.3867 33.9993C28.3867 30.8818 30.9206 28.3479 34.0381 28.3479H48.2306C51.3671 28.3479 53.882 30.8818 53.882 33.9993C53.882 37.1168 51.3482 39.6507 48.2306 39.6507H34.0381Z" fill="#FCC003"/></symbol>
  <symbol id="i-chart" viewBox="0 0 32 32"><path d="M4,2H2V28a2,2,0,0,0,2,2H30V28H4Z"/><path d="M30,9H23v2h3.59L19,18.59l-4.29-4.3a1,1,0,0,0-1.42,0L6,21.59,7.41,23,14,16.41l4.29,4.3a1,1,0,0,0,1.42,0L28,12.41V16h2Z"/></symbol>
  <symbol id="i-shield" viewBox="0 0 32 32"><path d="M14 16.59L11.41 14 10 15.41 14 19.41 22 11.41 20.59 10 14 16.59z"/><path d="M16,30,9.8242,26.7071A10.9818,10.9818,0,0,1,4,17V4A2.0021,2.0021,0,0,1,6,2H26a2.0021,2.0021,0,0,1,2,2V17a10.9818,10.9818,0,0,1-5.8242,9.7071ZM6,4V17a8.9852,8.9852,0,0,0,4.7656,7.9423L16,27.7333l5.2344-2.791A8.9852,8.9852,0,0,0,26,17V4Z"/></symbol>
</svg>
<header class="report-header">
  <a class="brand" href="#top" aria-label="Back to the top of this report"><b>HandoffProof</b><span>Report</span></a>
  <div class="header-integrity"><span class="status-dot{{if not .HashMatch}} bad{{end}}"></span><span>{{if .HashMatch}}Manifest verified{{else}}Manifest mismatch{{end}}</span></div>
</header>
<section class="hero" id="top">
  <div class="hero-inner">
    <div class="hero-copy">
      <p class="hero-label">Controller evidence report</p>
      <h1><strong>HandoffProof</strong> {{.Title}}</h1>
      <p class="hero-sub">Pass and fail are decided by the controller. The model proposes, explains, and repairs - it never issues a verdict. Every observed row cites an evidence ID.</p>
      <div class="actions">
        <button id="btn-copy" type="button"><svg class="ic"><use href="#i-doc"/></svg>Copy link to share</button>
        <button id="btn-dl" class="ghost" type="button"><svg class="ic"><use href="#i-arrow"/></svg>Download PDF</button>
        <button id="btn-slack" class="ghost" type="button"><svg class="brand-ic"><use href="#i-slack"/></svg><span id="btn-slack-text">Post to Slack</span></button>
      </div>
      <div id="sharemsg" role="status" aria-live="polite"></div>
    </div>
    <div class="hero-art"><div class="artwork" role="img" aria-label="Storyset illustration: reviewing evidence documents">{{.EvidenceArt}}</div></div>
  </div>
</section>
<main class="pagewrap">
<nav class="report-nav" aria-label="Report sections">
  <a href="#summary">Summary</a><a href="#findings">Observed findings</a><a href="#risks">Inferred risks</a><a href="#untested">Untested</a><a href="#evidence">Evidence records</a><a href="#integrity">Integrity</a>
</nav>
<section class="stats" aria-label="Report totals">
  <div class="stat"><div class="v">{{len .Evidence}}</div><div class="l">evidence records</div></div>
  <div class="stat"><div class="v">{{passed .Evidence}}</div><div class="l">observed pass</div></div>
  <div class="stat"><div class="v">{{failed .Evidence}}</div><div class="l">observed fail</div></div>
  <div class="stat purple"><div class="v">{{len .Findings}}</div><div class="l">observed findings</div></div>
</section>
<script>
(function(){
var msg=function(t){document.getElementById('sharemsg').textContent=t;};
var copyBtn=document.getElementById('btn-copy');
if(copyBtn)copyBtn.addEventListener('click',function(){
  msg('Making a read-only share link (valid 24h)…');
  var done=function(url){msg('Link copied. Read-only, valid 24h - safe to send to anyone.');};
  fetch('/api/share',{method:'POST',headers:{'content-type':'application/json','x-hp-request':'1'},body:'{}'}).then(function(r){return r.json().then(function(j){return{ok:r.ok,j:j};});}).then(function(out){
    if(!out.ok){msg('Could not make a share link: '+(out.j.error||'failed'));return;}
    var url=location.origin+'/report?s='+out.j.share;
    if(navigator.clipboard&&navigator.clipboard.writeText){navigator.clipboard.writeText(url).then(function(){done(url);},function(){msg('Copy failed. Link: '+url);});}
    else{msg('Copy not available. Link: '+url);}
  }).catch(function(){msg('Could not make a share link.');});
});
var dl=document.getElementById('btn-dl');
if(dl)dl.addEventListener('click',function(){
  msg('Opening the print dialog - choose "Save as PDF". The PDF uses this page’s theme.');
  setTimeout(function(){window.print();},300);
});
var slack=document.getElementById('btn-slack');
var slackText=document.getElementById('btn-slack-text');
var slackState=null;
function renderSlackState(){
  if(!slack||!slackText||!slackState)return;
  if(slackState.connected)slackText.textContent='Post to Slack';
  else if(slackState.can_connect)slackText.textContent='Add to Slack';
  else slackText.textContent='Connect Slack';
}
function refreshSlack(){
  if(!slack)return Promise.resolve(null);
  return fetch('/api/integrations/slack').then(function(r){return r.json();}).then(function(j){slackState=j;renderSlackState();return j;}).catch(function(){slackState={connected:false,can_connect:false,setup_reason:'Open this report through the HandoffProof server to connect or post to Slack.'};renderSlackState();return slackState;});
}
if(slack){
  var slackResult=new URLSearchParams(location.search).get('slack');
  refreshSlack().then(function(){if(slackResult==='connected')msg('Slack connected. You can now post the counts-only summary.');else if(slackResult==='error')msg('Slack connection did not complete. Check the Slack app redirect URL and server credentials, then try Add to Slack again.');});
  slack.addEventListener('click',function(){
    refreshSlack().then(function(state){
      if(!state)return;
      if(!state.connected){
        if(state.can_connect){location.href='/api/integrations/slack/connect?from=report';return;}
        msg(state.setup_reason||'Slack is not configured on the server.');
        return;
      }
      slack.disabled=true;msg('Posting summary to Slack…');
      fetch('/api/integrations/slack',{method:'POST',headers:{'content-type':'application/json','x-hp-request':'1'},body:'{}'}).then(function(r){return r.json().then(function(j){return{ok:r.ok,j:j};});}).then(function(out){
        if(out.ok){msg('Posted to Slack.');}else{msg('Slack: '+(out.j.error||'failed')+'. Posting is limited to console operators.');}
        slack.disabled=false;
      }).catch(function(){msg('Slack post failed.');slack.disabled=false;});
    });
  });
}
function initReportPagination(){
  var table=document.getElementById('evidence-records');
  if(!table)return;
  var rows=Array.prototype.slice.call(table.querySelectorAll('tbody tr'));
  var sizeSelect=document.getElementById('report-page-size');
  var range=document.getElementById('report-range');
  var pagesEl=document.getElementById('report-pages');
  var first=document.getElementById('report-first');
  var prev=document.getElementById('report-prev');
  var next=document.getElementById('report-next');
  var last=document.getElementById('report-last');
  var page=1;
  var storedSize=Number(localStorage.getItem('hp-page-size'))||10;
  if([10,25,50].indexOf(storedSize)>=0)sizeSelect.value=String(storedSize);
  var pageSize=Number(sizeSelect.value)||10;
  function totalPages(){return Math.max(1,Math.ceil(rows.length/pageSize));}
  function pageNumbers(total,current){
    if(total<=7){var all=[];for(var i=1;i<=total;i++)all.push(i);return all;}
    var seen={},raw=[1,2,current-1,current,current+1,total-1,total],pages=[];
    raw.forEach(function(p){if(p>=1&&p<=total&&!seen[p]){seen[p]=true;pages.push(p);}});
    pages.sort(function(a,b){return a-b;});
    var out=[];
    pages.forEach(function(p,i){if(i&&p-pages[i-1]>1)out.push('…');out.push(p);});
    return out;
  }
  function render(){
    var total=totalPages();
    if(page>total)page=total;
    if(page<1)page=1;
    var start=rows.length===0?0:(page-1)*pageSize;
    var end=Math.min(rows.length,page*pageSize);
    rows.forEach(function(row,i){row.classList.toggle('paginated-hidden',i<start||i>=end);});
    range.textContent=rows.length===0?'0 records':'Showing '+(start+1)+'–'+end+' of '+rows.length+' records';
    first.disabled=prev.disabled=page<=1;
    next.disabled=last.disabled=page>=total;
    pagesEl.innerHTML='';
    pageNumbers(total,page).forEach(function(p){
      if(p==='…'){var ellipsis=document.createElement('span');ellipsis.className='pagination-ellipsis';ellipsis.setAttribute('aria-hidden','true');ellipsis.textContent='…';pagesEl.appendChild(ellipsis);return;}
      var button=document.createElement('button');
      button.type='button';button.className='page-btn'+(p===page?' active':'');button.textContent=p;
      button.setAttribute('aria-label','Page '+p);
      if(p===page)button.setAttribute('aria-current','page');
      button.addEventListener('click',function(){page=p;render();});
      pagesEl.appendChild(button);
    });
  }
  sizeSelect.addEventListener('change',function(){pageSize=Number(sizeSelect.value)||10;localStorage.setItem('hp-page-size',String(pageSize));page=1;render();});
  first.addEventListener('click',function(){page=1;render();});
  prev.addEventListener('click',function(){page=Math.max(1,page-1);render();});
  next.addEventListener('click',function(){page=Math.min(totalPages(),page+1);render();});
  last.addEventListener('click',function(){page=totalPages();render();});
  document.querySelectorAll('a[href^="#"]').forEach(function(link){
    link.addEventListener('click',function(){
      var row=document.getElementById(link.getAttribute('href').slice(1));
      var index=rows.indexOf(row);
      if(index<0)return;
      page=Math.floor(index/pageSize)+1;
      render();
      setTimeout(function(){row.scrollIntoView({block:'center'});},0);
    });
  });
  if(location.hash){
    var target=document.getElementById(location.hash.slice(1));
    var targetIndex=rows.indexOf(target);
    if(targetIndex>=0)page=Math.floor(targetIndex/pageSize)+1;
  }
  render();
}
if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',initReportPagination);else initReportPagination();
})();
</script>
<section class="section" id="summary">
  <div class="section-head"><span class="section-icon"><svg class="ic"><use href="#i-chart"/></svg></span><div><h2>Summary</h2><p>Controller checks before and after the rehearsed exit, linked to the records that prove them.</p></div></div>
  <div class="table-wrap"><table><thead><tr><th>Check</th><th>Class</th><th>Before</th><th>After</th><th>Note</th></tr></thead><tbody>
  {{range .Checks}}<tr><td>{{redact .Name}}</td><td><span class="badge {{badge .Class}}">{{.Class}}</span></td>
  <td>{{outcome .BeforePassed}}{{if .BeforeID}} (<a href="#{{.BeforeID}}">{{.BeforeID}}</a>){{end}}</td>
  <td>{{outcome .AfterPassed}}{{if .AfterID}} (<a href="#{{.AfterID}}">{{.AfterID}}</a>){{end}}</td><td>{{redact .Note}}</td></tr>
  {{else}}<tr><td colspan="5" class="empty">No checks recorded.</td></tr>{{end}}
  </tbody></table></div>
</section>
<section class="section" id="findings">
  <div class="section-head"><span class="section-icon"><svg class="ic"><use href="#i-search"/></svg></span><div><h2>Observed findings</h2><p>Diagnosed from controller evidence. Each finding links back to the records behind it.</p></div><div class="section-art" role="img" aria-label="Storyset illustration: analyzing data">{{.InsightsArt}}</div></div>
  {{range .Findings}}<article class="finding-card" id="{{index .EvidenceIDs 0}}">
    <div class="finding-top"><h3>{{redact .ID}}</h3><span class="badge {{badge .Class}}">{{.Class}}</span><span class="finding-workflow">{{redact .Workflow}}</span></div>
    <div class="meta-grid">
      <div class="meta-cell"><div class="k">Workflow</div><div class="v">{{redact .Workflow}}</div></div>
      <div class="meta-cell"><div class="k">Failed step</div><div class="v">{{redact .FailedStep}}</div></div>
      <div class="meta-cell"><div class="k">Source</div><div class="v">{{if .SourceRefs}}{{range .SourceRefs}}<code>{{redact .}}</code><br>{{end}}{{else if .SourceRef}}<code>{{redact .SourceRef}}</code>{{else}}<span class="muted">Not provided: citation missing</span>{{end}}</div></div>
      <div class="meta-cell"><div class="k">Confidence</div><div class="v">{{printf "%.2f" .Confidence}}</div></div>
    </div>
    <div class="finding-columns">
      <div><h4>Root cause</h4><p>{{redact .RootCause}}</p></div>
      <div><h4>Business impact</h4><p>{{redact .BusinessHit}}</p></div>
    </div>
    <p class="evidence-links">Evidence: {{range .EvidenceIDs}}<a href="#{{.}}"><code>{{.}}</code></a> {{end}}</p>
    {{if .LogExcerpt}}<pre class="log">{{redact .LogExcerpt}}</pre>{{end}}
  </article>
  {{else}}<p class="empty">No observed findings. That means no diagnosed failure was anchored to a controller record in this report.</p>{{end}}
</section>
<section class="section" id="risks">
  <div class="section-head"><span class="section-icon purple"><svg class="ic"><use href="#i-warn"/></svg></span><div><h2>Inferred risks</h2><p>Not observed in a drill. These are leads to verify, not verdicts.</p></div></div>
  {{range .Inferred}}<article class="finding-card">
    <div class="finding-top"><h3>{{redact .ID}}</h3><span class="badge risk">{{.Class}}</span><span class="finding-workflow">{{redact .Workflow}}</span></div>
    <div class="meta-grid"><div class="meta-cell"><div class="k">Workflow</div><div class="v">{{redact .Workflow}}</div></div><div class="meta-cell"><div class="k">Confidence</div><div class="v">{{printf "%.2f" .Confidence}}</div></div></div>
    <div class="finding-columns"><div><h4>Risk</h4><p>{{redact .RootCause}}</p></div></div>
  </article>
  {{else}}<p class="empty">No inferred risks recorded.</p>{{end}}
</section>
<section class="section" id="untested">
  <div class="section-head"><span class="section-icon purple"><svg class="ic"><use href="#i-doc"/></svg></span><div><h2>Untested dependencies</h2><p>Not observed in a drill and not promoted to findings.</p></div></div>
  {{range .Untested}}<article class="finding-card">
    <div class="finding-top"><h3>{{redact .ID}}</h3><span class="badge untested">{{.Class}}</span><span class="finding-workflow">{{redact .Workflow}}</span></div>
    <div class="finding-columns"><div><h4>Dependency</h4><p>{{redact .RootCause}}</p></div></div>
  </article>
  {{else}}<p class="empty">No untested dependencies recorded.</p>{{end}}
</section>
<section class="section" id="evidence">
  <div class="section-head"><span class="section-icon"><svg class="ic"><use href="#i-dashboard"/></svg></span><div><h2>Evidence records</h2><p>The controller-issued record set behind every verdict in this report.</p></div></div>
  <div class="table-wrap"><table id="evidence-records"><thead><tr><th>Evidence ID</th><th>Drill</th><th>Workflow</th><th>Assertion</th><th>Result</th></tr></thead><tbody>
  {{range .Evidence}}<tr id="{{.ID}}"><td class="mono">{{.ID}}</td><td>{{redact .Drill}}</td><td>{{redact .Workflow}}</td>
  <td>{{redact .Assertion}}</td><td><span class="badge {{if .Passed}}pass{{else}}fail{{end}}">{{if .Passed}}passed{{else}}failed{{end}}</span></td></tr>
  {{else}}<tr><td colspan="5" class="empty">No evidence records in this report.</td></tr>{{end}}
  </tbody></table></div>
<div class="pagination" id="report-pagination" aria-label="Evidence records pagination">
  <div class="pagination-group">
    <label for="report-page-size">Rows per page</label>
    <select id="report-page-size"><option value="10">10</option><option value="25">25</option><option value="50">50</option></select>
    <span id="report-range" aria-live="polite"></span>
  </div>
  <div class="pagination-nav">
    <button class="page-btn" id="report-first" type="button" aria-label="First page">First</button>
    <button class="page-btn" id="report-prev" type="button" aria-label="Previous page">‹</button>
    <div class="pagination-pages" id="report-pages"></div>
    <button class="page-btn" id="report-next" type="button" aria-label="Next page">›</button>
    <button class="page-btn" id="report-last" type="button" aria-label="Last page">Last</button>
  </div>
</div>
</section>
{{if .Interview}}<section class="section" id="interview">
  <div class="section-head"><span class="section-icon"><svg class="ic"><use href="#i-doc"/></svg></span><div><h2>Exit interview</h2><p>Departing-developer claims checked against drill evidence and the static scan on {{redact .Interview.Started}} (model {{redact .Interview.Model}}). Confirmed and contradicted rows cite evidence; unverifiable rows need a written runbook. Transcripts are stored redacted.</p></div></div>
  {{if .Interview.Dropped}}<p class="empty">{{len .Interview.Dropped}} injected claim(s) ignored and logged: {{redact (join .Interview.Dropped ", ")}}</p>{{end}}
  <div class="table-wrap"><table><thead><tr><th>Status</th><th>Claim</th><th>Type</th><th>Speaker</th><th>Time</th><th>Evidence</th></tr></thead><tbody>
  {{range .Interview.Claims}}<tr><td><span class="badge {{if eq .Status "confirmed"}}pass{{else if eq .Status "contradicted"}}fail{{else}}untested{{end}}">{{redact .Status}}</span></td>
  <td>{{redact .Statement}}<div class="muted small mono">{{redact .ID}} · {{redact .Subject}}</div><div class="muted small">{{redact .Detail}}{{if .NeedsRunbook}} Needs a written runbook.{{end}}</div></td>
  <td>{{redact .Type}}</td><td>{{redact .Speaker}}</td><td class="mono">{{redact .Timestamp}}</td><td class="mono">{{if .EvidenceIDs}}{{redact (join .EvidenceIDs ", ")}}{{else}}-{{end}}</td></tr>
  {{else}}<tr><td colspan="6" class="empty">No claims extracted.</td></tr>{{end}}
  </tbody></table></div>
</section>{{end}}
<section class="section" id="integrity">
  <div class="section-head"><span class="section-icon"><svg class="ic"><use href="#i-shield"/></svg></span><div><h2>Acceptance integrity</h2><p>Measured by the controller at render time - never asserted by the model.</p></div></div>
  <div class="integrity-grid">
    <div>
      <p class="integrity-status">{{if .HashMatch}}<strong>Verified unchanged.</strong> The acceptance and controller files match the frozen manifest.{{else}}<strong>MISMATCH - do not trust this report.</strong> A protected file changed after the manifest was frozen.{{end}}</p>
      <div class="hash-list">
        <div><span class="k">Manifest hash</span><code>{{.HashBefore}}</code></div>
        <div><span class="k">Current hash</span><code>{{.HashAfter}}</code></div>
        <div><span class="k">State</span><span class="badge {{if .HashMatch}}pass{{else}}fail{{end}}">{{if .HashMatch}}verified unchanged{{else}}mismatch{{end}}</span></div>
      </div>
    </div>
    <div class="integrity-art" role="img" aria-label="Storyset illustration: handing work over">{{.HandoverArt}}</div>
  </div>
</section>
<footer class="report-footer"><svg class="ic"><use href="#i-shield"/></svg><span>Pass/fail is decided by the controller. The model proposes, explains and repairs - it never issues a verdict. Illustrations by <a href="https://storyset.com" target="_blank" rel="noopener">Storyset</a>, recolored to the application palette.</span></footer>
</main>
</body></html>`
