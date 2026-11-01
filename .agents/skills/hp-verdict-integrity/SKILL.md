---
name: hp-verdict-integrity
description: Use whenever code, tests, reports or prompts mention pass, fail, verified, recovered or fixed. Enforces that only the controller issues verdicts and defines the four finding classes.
---
# Verdict integrity (constraint)

## Rule
Only code under `internal/controller/` may decide that a workflow passed or failed. The reasoning
agent proposes, explains and requests. It never decides.

## Finding classes (exactly one per finding)
- observed_failure: a controller assertion failed. Requires an evidence_id.
- observed_pass: a controller assertion passed. Requires an evidence_id.
- inferred_risk: suggested by reading code or docs, no runtime evidence.
- untested_dependency: outside the sandbox's reach (DNS, registrar, third-party account ownership).

## Do
- Attach an evidence_id to every observed_* claim.
- State what runtime check would confirm an inferred_risk.
- Cap confidence: 0.9 without runtime evidence, 0.97 with it.

## Do not
- Add any function, field or tool that lets the agent set a verdict.
- Write report text that says "verified" or "passed" without an evidence_id.
- Convert an inferred_risk into an observed_failure by wording.

## Verify
- `grep -rn "Passed" internal --include=*.go | grep -v internal/controller | grep -v internal/evidence`
  must show no assignments.
- Every finding in a sample report has a class and, for observed_*, an evidence_id.
