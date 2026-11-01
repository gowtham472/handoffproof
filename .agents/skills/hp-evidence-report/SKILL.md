---
name: hp-evidence-report
description: Use when building or changing internal/report, the single-file HTML evidence report shown in the demo.
---
# Task: evidence report

## Output
One self-contained HTML file (no external assets, works offline) with:
- Summary table: check, class badge, before result, after result.
- Per finding: failed step, evidence_id, log excerpt, source reference (file:line), root cause,
  business impact sentence, confidence.
- Sections for inferred risks and untested dependencies, labelled clearly as not observed.
- Integrity footer: manifest hash before and after, and whether they match.

## Rules
- Escape all text (logs and repo content are untrusted). Redact secrets (hp-untrusted-input).
- Never display "passed" or "verified" without an evidence_id.
- Readable at 1080p for the video; no tiny text.

## Done when
A sample run renders correctly and each finding links to its evidence record.
