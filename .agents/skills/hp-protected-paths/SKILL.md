---
name: hp-protected-paths
description: Use before editing acceptance tests, controller code, drill code, evidence, or the manifest, and whenever a repair diff is generated. Defines protected paths and the hash-manifest procedure.
---
# Protected paths (constraint)

## Protected
`acceptance/`, `internal/controller/`, `internal/drills/`, `evidence/`, `.manifest.sha256`, `scripts/manifest.sh`

## Two phases
- Development time: you may edit these paths when the user asks for that work (for example writing
  the acceptance tests). After intentional changes, ask the user, then run
  `bash scripts/manifest.sh create` and commit the new manifest.
- Run time (drills, repairs, reruns): nothing may change them. The controller runs
  `bash scripts/manifest.sh verify` before and after every run and aborts on mismatch.

## Freeze point
When the acceptance tests are green on the baseline, create the manifest and treat the folder as frozen.
Later edits need explicit user approval and a manifest re-create in a separate commit.

## Repairs
A repair diff that touches a protected path is rejected by the controller. Never weaken, skip, delete,
rename or reorder acceptance tests, and never add special cases in app code that detect the test run.

## Verify
`bash scripts/manifest.sh verify` prints `manifest OK`.
