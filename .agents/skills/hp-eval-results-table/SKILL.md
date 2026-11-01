---
name: hp-eval-results-table
description: Use when measuring and reporting how well HandoffProof detects faults versus baselines. Produces an honest results table in docs/RESULTS.md.
---
# Task: results table

## Setup
- Ground truth: `eval/ground_truth.json` (planted faults) plus, if possible, one fault chosen by
  someone outside the implementation and kept secret until the run.
- Arms: static_scan_only, fresh_deployment_only, full HandoffProof.

## Measure per arm
faults injected, caught, missed, false alarms, inferred risks later confirmed or refuted,
tokens per run.

## Rules
- Report misses and false alarms plainly. A reported miss builds trust.
- Mark anything not actually run as "target", never as a result.
- State the app and conditions the numbers apply to; do not generalize.

## Output
`docs/RESULTS.md` with the table, the date, the commit hash, and the evidence IDs behind each row.
