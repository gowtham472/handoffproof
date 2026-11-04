# Results

Measured on the demo app and a second fixture (author-written). Misses are reported plainly; anything not run is labelled a target.

Date: 2026-11-04 · Commit: bc4b2ab · Model: muse-spark-1.3

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

## Model comparison (profiles x apps, 3 runs per cell)

Each cell runs the six agent phases live behind the profile router on the same recorded controller drill evidence; detection arms are controller-driven and identical across profiles, while citation, retry, token, latency, cost and repair columns vary by model. Sample size: 3 runs per cell on author-written fixtures (`demo-app`, `eval/fixtures/heldout-app`). Rows labelled `not run` never started; their reason is recorded, not omitted. Denominators: apply = cells whose proposal carried a diff; green = applied cells.

Historical artifact limitations: these saved cells predate application-scoped evidence filtering. Their prompts included mixed demo and BYOA records, and second-fixture repairs were incorrectly attempted against the demo-app copy. Some logs also record a manifest mismatch during a historical repair. These are harness defects, so the table is not a clean controlled model comparison. The fixes have not been evaluated with live models; rendering saved cells does not repair or replace their measurements.

Cit V/I counts model-verifier verdicts: verified requires both citation_ok and evidence_ok; invalid means citation_ok is false. It is not the deterministic file/line checker count, and V/I is two counts, not a success fraction. The deterministic checker resolves diagnosis paths and line ranges and checks evidence IDs; it does not prove semantic support. Several model findings used only EV identifiers instead of file:line references. Apply 1st-try counts whether the first prioritized nonempty diff applied to a sandbox; it does not establish recovery. Bad diff context and the harness workspace error both contributed to failures. Controller verdicts never depend on model citations.

The earlier five-verdict verification.json is a separate model-verifier artifact from the demo workflow. Its exact source revision and invocation are not recorded, so identical checker/workspace/code provenance cannot be established from the artifact; it must not be compared as a repeat of these cells.

| Profile | App | Runs OK | Caught | Missed | False alarms | Cit V/I | Schema-fail turns | Tokens/run | Latency/run | Est. $/run | Apply 1st-try | Rerun green |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| spark13 | demo-app | 3/3 | 5 (F1-missing-config, F2-silent-mail-failure, F3-incomplete-backup, F4-handover-ask-ravi, F5-injection-comment) | 0 (-) | 0 | 6/24 | 0 | 235058 | 415.8s | $0.4290 | 0/3 | n/a |
| spark13 | heldout-app | 3/3 | 3 (H1-undocumented-key, H2-empty-backup, H3-ask-sam) | 0 (-) | 0 | 4/24 | 0 | 152149 | 315.2s | $0.2879 | 0/3 | n/a |
| wonder | demo-app | 3/3 | 5 (F1-missing-config, F2-silent-mail-failure, F3-incomplete-backup, F4-handover-ask-ravi, F5-injection-comment) | 0 (-) | 0 | 0/16 | 0 | 406330 | 604.7s | $2.4630 | 0/1 | n/a |
| wonder | heldout-app | 3/3 | 3 (H1-undocumented-key, H2-empty-backup, H3-ask-sam) | 0 (-) | 0 | 0/13 | 0 | 219654 | 395.0s | $1.4477 | 0/3 | n/a |
| spark-open | demo-app | not run | - | - | - | - | - | - | - | - | - | profile spark-open: TOGETHER_API_KEY is not set |
| spark-open | heldout-app | not run | - | - | - | - | - | - | - | - | - | profile spark-open: TOGETHER_API_KEY is not set |
| glimmer | demo-app | not run | - | - | - | - | - | - | - | - | - | profile glimmer: endpoint http://127.0.0.1:11434/v1 unreachable: dial tcp 127.0.0.1:11434: connectex: No connection could be made because the target machine actively refused it. |
| glimmer | heldout-app | not run | - | - | - | - | - | - | - | - | - | profile glimmer: endpoint http://127.0.0.1:11434/v1 unreachable: dial tcp 127.0.0.1:11434: connectex: No connection could be made because the target machine actively refused it. |

Cell artifacts: `evidence/eval/<profile>/<app>/rep<n>/` (`cell.json`, phase JSONs, `phases.log`).

<!-- f7:begin -->
## Whiteboard claims (F7, experimental)

Generated from `evidence/whiteboard-20261103-192255.json` by `handoffproof whiteboard --render`. One run on one image (synthetic=true, generated by muse-image-1.0 and labelled as such): it shows the pipeline works end to end on that fixture and says nothing about general accuracy.

Verdicts were recomputed by the controller alone (no model call) from the claims recorded in `evidence/whiteboard-20261103-184027.json`, after the claim-polarity fix.

- sam-3.1: 1 screen region(s) pixelated before any model read the image; 7 note region(s) cropped; 8 images sent to muse-spark-1.3 (6403 tokens, 40.9s).
- Injected claims dropped by the controller: 1 (c1).

| Claim | Type | Statement (redacted) | Controller status | Evidence | Detail |
| --- | --- | --- | --- | --- | --- |
| c2 | backup | DB backup runs nightly - restore never tested | confirmed | EV-c8434858, EV-d57ab9d1 | incomplete-backup drill evidence shows the gap; restore was not demonstrated |
| c3 | credential_location | MAIL_API_TOKEN is in the team password vault | unverifiable | - | no withdrawn-token drill evidence for this claim |
| c4 | config | DATABASE_URL is documented in the README | confirmed | - | static scan: DATABASE_URL is used and documented |
| c5 | process | Ravi deploys on Fridays | unverifiable | - | type process has no drill or scan check; needs a written runbook |
<!-- f7:end -->

<!-- f6:begin -->
## Profile auto-drafter (F6)

Generated from `evidence/drafts/draft-*.json` by `handoffproof draft --render`. A draft counts as passing only when it decodes strictly, carries no secret values, validates against the profile schema and Compose denylist, and passes a fresh sandbox baseline unedited. Apps are author-written fixtures.

Each draft gets up to 3 attempts: validator errors (field names only, never values) are fed back to the model, like schema retries. No human edits anything.

| App | Model | Drafts | Model OK | Valid 1st try | Valid + clean | Baseline passed unedited |
| --- | --- | --- | --- | --- | --- | --- |
| miniapp | muse-wonder-1.0 | 3 | 3 | 1 | 1 | 0/3 (0%) |
| demo-app | - | not run | - | - | - | not run with the final code: stopped to stay within the remaining API budget. The earlier v3 run (shared tool budget, archived in evidence/drafts/prompt-v3-shared-budget/) scored 0/3; its one valid draft failed the baseline on an undeclared required env (POSTGRES_DB). demo-app's Compose also builds ../mocks/mail-sink, outside the app folder, which the BYOA sandbox does not copy. |
| heldout-app | - | not run | - | - | - | not applicable: eval/fixtures/heldout-app has no Compose file, so no BYOA baseline can run. |

Failure reasons (final attempt), as recorded:

- miniapp: draft contains the value of MAIL_API_TOKEN
- miniapp: draft contains the value of MAIL_API_TOKEN
- miniapp: application did not become healthy: 
<!-- f6:end -->
