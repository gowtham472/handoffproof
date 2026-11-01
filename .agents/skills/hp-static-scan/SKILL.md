---
name: hp-static-scan
description: Use when building or changing internal/scan, the deterministic scanner that is both the agent's ground truth for static facts and the baseline the model must beat.
---
# Task: static scan

## Output
JSON returned by the `static_scan_report` tool: env vars read in code vs documented in `.env.example` and
docs, scheduled jobs and the env vars they use, upload and storage paths, whether the backup script
covers each data store, hardcoded hosts and bucket names, TODO or "ask <name>" phrases.

## Rules
- Pure Go and regex or simple parsing. No model calls.
- Each item carries file and line references.
- Output is stable (sorted) so runs are comparable.

## Baseline use
Run the scan alone against `eval/ground_truth.json` and record what it catches and misses.
The model must be shown to add value on the rest (for example prose such as "ask Ravi").

## Done when
Unit tests with fixture repos pass, and the demo app's missing variable is detected.
