---
name: hp-repair-and-rerun
description: Use when implementing or running the loop that proposes a repair, applies it in a sandbox copy and reruns the same acceptance tests.
---
# Task: repair and rerun

## Steps
1. The agent submits a unified diff plus rationale (repair_proposal schema).
2. The controller rejects any diff touching a protected path (hp-protected-paths).
3. The controller applies the diff to a fresh sandbox copy of the repo, never the original.
4. Verify the manifest hash equals the pre-repair hash, then rerun the identical acceptance suite.
5. Record before and after evidence IDs. If the rerun fails, report the failure plainly.
6. Actions only a human can do (transfer an account, rotate a key) are `human_action_required`.

## Rules
Prefer documentation or configuration fixes. Keep diffs minimal and reviewable.

## Done when
One fault goes red to green with the same acceptance hashes, shown in the report.
