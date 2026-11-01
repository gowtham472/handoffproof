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

Work in progress. See `docs/PLAN.md`.

## Layout

See `AGENTS.md` for the architecture and rules.
