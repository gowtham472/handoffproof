---
name: hp-implement-drill
description: Use when adding or changing a drill (fresh baseline, withdrawn token, missing configuration, incomplete backup) in internal/drills and its runner in internal/controller.
---
# Task: implement a drill

## Interface (Go, internal/drills)
Name; ValidateParams against an allowlist; Setup (fresh sandbox); Inject (the fault); Observe
(run acceptance workflows, collect evidence); Teardown. Returns `[]evidence.Record`.

## Drills
- fresh_deployment_baseline: start with only documented configuration. Gate for the others.
- withdrawn_test_token: revoke the mail token in the sink, trigger booking and the scheduled job.
- missing_configuration: remove one undocumented variable, start the app, run workflows.
- incomplete_backup: run `backup.sh`, destroy data and storage, restore into a fresh environment,
  assert record counts and file checksums.

## Rules
- Docker only, isolated internal network, resource limits (see hp-secrets-and-safety).
- Faults are deterministic and repeatable. Teardown always runs, even on failure.
- Evidence is written by the controller, never by the model (see hp-verdict-integrity).

## Done when
A table-driven test covers parameter validation, and one real run produces evidence records with IDs.
