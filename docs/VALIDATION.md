# Recorded WSL validation

Date: 2026-11-04 (Asia/Kolkata). Code revision tested:
`b552cf94a1d9d1f5db6685733bfb0eb34d262522`.

A fresh public-repository clone ran in Ubuntu WSL2 on Linux kernel
`6.18.33.2-microsoft-standard-WSL2`, using Go `1.27.3 linux/amd64` and Docker Engine
`29.7.2`. The Ubuntu disk and Linux toolchain were stored on D: on the recorded host.
No real application or model credentials were supplied. Model diagnosis was disabled,
and no live model calls were made.

## Checks performed

| Check | Recorded outcome |
| --- | --- |
| Internal Docker network creation and removal | passed; `Internal=true` |
| Isolated `TestRunnerPythonFixture` | passed in 13.84 seconds |
| `go test ./... -count=1 -timeout 12m` | all 13 packages passed; zero skipped tests and zero failures |
| `go vet ./...` | passed |
| `bash scripts/check.sh` | passed: formatting, vet, Go tests, 16 skills and protected manifest |
| Controller fresh-deployment baseline | exited 0 in 32.2 seconds |
| Protected manifest after the baseline | passed |
| Tracked working-tree changes in the Linux checkout | none |

## Controller baseline evidence

The [controller record](evidence-sample/wsl-baseline-2026-11-04.json) is copied without
changing its values from the fresh run. Its assertions, timestamps, artifact hashes
and evidence IDs are retained. It contains only synthetic-fixture results.

| Workflow | Controller result | Evidence ID |
| --- | --- | --- |
| registration | pass | `EV-65e1518f` |
| booking | pass | `EV-915408cd` |
| confirmation_email | pass | `EV-78b1f7b0` |

These observed passes apply to the bundled fixture in this environment. They do not
measure model accuracy, first-try repairs, profile drafting or whiteboard quality.
The historical model measurements in [RESULTS.md](RESULTS.md) remain unchanged.

## Reproduce in Ubuntu

Install Linux Go at the version required by [go.mod](../go.mod) or newer and add it
to the shell's PATH. Enable Ubuntu's Docker Desktop WSL integration. Then use the
[README WSL2 commands](../README.md#wsl2-checks). To run the Docker fixture alone:

```bash
go test ./internal/byoa -run '^TestRunnerPythonFixture$' -count=1 -v -timeout 10m
```

Require an actual `PASS` for that test. A skip caused by unavailable Docker does not
verify the fixture. The recorded host used a user-local Go installation at
`~/.local/share/handoffproof-validation/go1.27.3/go/bin`; reusing that installation
requires adding that directory to PATH in the shell.

## Environment recovery notes

The earlier Windows run stalled during Docker network creation, and fresh Windows
executables were also blocked by Application Control. The later host recovery freed
cache space, restarted the unresponsive WSL service, and cleared the specifically
approved zero-byte Docker runtime endpoints after startup logs identified them.
Desktop and its engine then started successfully. No Docker data purge, image prune,
Windows security-policy change or application-code change was needed for this run.
The original stall's single root cause was not isolated.

Runtime endpoints are distinct from Docker's stored image and volume data. Investigate
the current error and stop Desktop before considering runtime-file cleanup; the
recorded recovery is a case history, not a blanket instruction to delete Docker files.
