# Results

Measured on the demo app and a second fixture (author-written). Misses are reported plainly; anything not run is labelled a target.

Date: 2026-11-01 · Commit: 434b6dc · Model: muse-spark-1.3

| Arm | App | Injected | Caught | Missed | False alarms | Tokens/run |
| --- | --- | --- | --- | --- | --- | --- |
| static_scan_only | demo-app | 5 | 3 (F1-missing-config, F3-incomplete-backup, F4-handover-ask-ravi) | 2 (F2-silent-mail-failure, F5-injection-comment) | 0 | 0 (deterministic) |
| scan + drills, no model | demo-app | 5 | 4 (F1-missing-config, F2-silent-mail-failure, F3-incomplete-backup, F4-handover-ask-ravi) | 1 (F5-injection-comment) | 0 | 0 (deterministic) |
| full HandoffProof | demo-app | 5 | 5 (F1-missing-config, F2-silent-mail-failure, F3-incomplete-backup, F4-handover-ask-ravi, F5-injection-comment) | 0 (-) | 0 | 100749 (scan + one full loop, logged) |
| static_scan_only (second fixture, author-written) | heldout-app | 3 | 3 (H1-undocumented-key, H2-empty-backup, H3-ask-sam) | 0 (-) | 0 | 0 (deterministic) |

Evidence behind each row: `evidence/run-*.json`, `evidence/diagnosis-*.json`, `evidence/verification.json`.
Outsider fault slot: add `eval/outsider_fault.json` (kind + target) and rerun `handoffproof eval`; until then the outsider arm is a **target**, not a result.

## Model quality (from existing artifacts only)

Citation verification, run `verification.json (modified 2026-11-01)`: 5 verdicts, 5 verified, 0 invalid, 0 missing-citation issues, 0 unknown-evidence-id verdicts.

Dependency-map recall, run `live-scan.log (modified 2026-11-01)`: 4 of 4 observable ground-truth faults have a matching dependency (F1-missing-config -> mail-api-token; F2-silent-mail-failure -> mail-api-token; F3-incomplete-backup -> file-storage-uploads; F4-handover-ask-ravi -> handover-docs-gap). Fault kinds with no dependency form (for example prompt_injection) are excluded from the denominator.

Tokens and schema retries, runs `live-scan.log (modified 2026-11-01), live-full.log (modified 2026-11-01), verify.log (modified 2026-11-01)`:

- `intake_summary` from `live-scan.log (modified 2026-11-01)`: in=27253 out=4016 total=31269 tokens.
- `dependency_map` from `live-scan.log (modified 2026-11-01)`: in=27980 out=7099 total=35079 tokens.
- `diagnosis` from `live-full.log (modified 2026-11-01)`: in=3919 out=3556 total=7475 tokens.
- `repair_proposal` from `live-full.log (modified 2026-11-01)`: total=20281 tokens.
- `assessment` from `live-full.log (modified 2026-11-01)`: total=6645 tokens.
- `verification` from `verify.log (modified 2026-11-01)`: total=52932 tokens.
- Schema-retry markers recorded across those logs: 0 (the loop logs a line only when retries are exhausted, so a zero count means none were recorded, not that none occurred).
- The `Tokens/run` figure for full HandoffProof above excludes the adversarial verifier (52932 tokens), so total recorded model usage across these runs is about 153681 tokens.

These model-quality numbers come from one recorded run with five findings and are not a general accuracy claim.
