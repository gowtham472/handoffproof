# Results

Measured on the demo app and a held-out fixture. Misses are reported plainly; anything not run is labelled a target.

Date: 2026-11-01 · Commit: f9ad02e · Model: muse-spark-1.3

| Arm | App | Injected | Caught | Missed | False alarms | Tokens/run |
| --- | --- | --- | --- | --- | --- | --- |
| static_scan_only | demo-app | 5 | 3 (F1-missing-config, F3-incomplete-backup, F4-handover-ask-ravi) | 2 (F2-silent-mail-failure, F5-injection-comment) | 0 | 0 (deterministic) |
| full HandoffProof | demo-app | 5 | 5 (F1-missing-config, F2-silent-mail-failure, F3-incomplete-backup, F4-handover-ask-ravi, F5-injection-comment) | 0 (—) | 0 | 100749 (scan + one full loop, logged) |
| static_scan_only (held-out fixture) | heldout-app | 3 | 3 (H1-undocumented-key, H2-empty-backup, H3-ask-sam) | 0 (—) | 0 | 0 (deterministic) |

Evidence behind each row: `evidence/run-*.json`, `evidence/diagnosis-*.json`, `evidence/verification.json`.
Outsider fault slot: add `eval/outsider_fault.json` (kind + target) and rerun `handoffproof eval`; until then the outsider arm is a **target**, not a result.
