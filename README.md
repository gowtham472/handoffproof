# HandoffProof

> Rehearse the day your developer leaves.

HandoffProof rehearses a developer-exit drill for a small client application. It starts the app in an
isolated environment, runs essential workflows, withdraws developer-owned test credentials, attempts
recovery from backups, and reports what actually broke, with logs, source references and rerun results.

Pass/fail is decided by deterministic assertions in a controller. A Meta model reads code and docs,
proposes dependencies, plans drills, diagnoses failures from recorded evidence and proposes repairs.
The model cannot award a pass or edit acceptance tests.

## Built during the event

Everything in this repository was created during the Meta Global AI Developer Hackathon
(26 Oct to 4 Nov 2026). First commit: see `git log --reverse`.

## Status

Day 2 closed live; power-up features F1–F6 done; security remediation complete
(no known CRITICAL/HIGH findings under the documented threat model). See
`docs/PLAN.md` and `docs/SECURITY.md`.

## Control Room (frontend)

```text
go run ./cmd/handoffproof serve
```

The console binds `127.0.0.1:8787` and prints a token URL at startup — open
that URL (or set `HANDOFF_CONSOLE_TOKEN` first). It shows live evidence, the
planted-fault scoreboard, drill buttons (controller-executed in Docker
sandboxes), diagnosis and assessment. Unauthenticated requests get 401.

## Security

Full threat model, finding-by-finding remediation log, production operating
rules, and accepted residual risks: [docs/SECURITY.md](docs/SECURITY.md).

## CI handover gate

`.github/workflows/handover.yml` runs on every pull request: the quality gate
(`scripts/check.sh`), the independent acceptance suite on a healthy baseline,
and one controller baseline drill in a fresh sandbox. A red gate means the
handover regressed.

## Adversarial verifier

`handoffproof verify --diagnosis evidence/diagnosis-<drill>.json` checks every
diagnosis twice: a deterministic floor (each root cause must cite a real
`file:line`, each observed claim a real evidence ID) and a model pass that
reads the code and proposes corrected references.

## What-if simulator

In the Control Room, pick a dependency (env var, storage path) and preview
which workflows break and which drill would confirm it — before running
anything.

## Benchmark

`handoffproof eval` regenerates `docs/RESULTS.md` from ground truth and
recorded evidence, including a held-out fixture app the agent prompt was
never tuned against.

## Remediation PRs (guardrailed)

`handoffproof pr --diff <repair.diff>` prepares a reviewable PR: protected
paths are refused, hashes are pinned in the body, and the default is a dry
run. `--execute` creates the branch and commit in a local clone only —
pushing and opening the PR stay human actions.

## Handover Score

`handoffproof score` computes a deterministic readiness score (workflows,
faults caught, documentation, backup coverage), appends it to
`docs/score-history.jsonl`, and the Control Room charts the trend. First
snapshot: 84/100.

## Layout

See `AGENTS.md` for the architecture and rules.
