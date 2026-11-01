# HandoffProof security posture

Status: **no known CRITICAL or HIGH findings open under the threat model below.**
This is not a claim of being "fully secure" — no honest audit can make that claim.
Last remediation: 2026-11-01, after a strict multi-agent audit (3 CRITICAL, 23 HIGH/MEDIUM/LOW findings).

## Threat model

In scope: untrusted repository content (prompt injection, malicious files, symlinks,
hostile compose files), a malicious or mistaken model, unauthenticated network callers,
and secret leakage into model context, reports, or logs.

Out of scope: an attacker with local filesystem or Docker-daemon access to the host
running HandoffProof (they already own the machine), compromise of the Meta API or of
pinned upstream images, and physical access.

## Trust boundaries

- Only `internal/controller` (+`internal/drills`) issues pass/fail and writes evidence.
  The model proposes maps, diagnoses, and repair diffs; it never sets verdicts.
- Protected paths (`acceptance/**`, `internal/controller/**`, `internal/drills/**`,
  `internal/evidence/**`, `internal/agent`, `internal/scan`, `internal/report`,
  `internal/score`, `cmd`, `prompts`, `evidence/**`, `.manifest.sha256`,
  `scripts/manifest.sh`) are hash-frozen in `.manifest.sha256` and verified in Go
  (`ManifestVerify`) before and after drills and reruns. The verifier never executes
  the script it verifies.
- Repair diffs apply only to a sandbox copy, only under `app/`, and only after
  `ChangedPaths`/`DiffTouchesProtected` screening (rename/copy/quoted/case/`./`
  normalization, traversal rejection).

## Finding-by-finding remediation

### CRITICAL

- **C1 — unsanitized app compose executed.** The controller now validates the app
  compose against a denylist (privileged, docker.sock, `network_mode: host`, devices,
  `cap_add`, host pid/ipc, host bind mounts) and generates its own override: drill
  sandboxes publish **no ports** and run on `internal: true` networks; limits
  (memory/CPU/pids, `no-new-privileges`) are forced. Only Playwright reruns republish,
  bound to `127.0.0.1` only. Verified: all four drills pass in this mode.
- **C2 — unauthenticated console could launch Docker drills.** The console binds
  `127.0.0.1:8787` by default and always requires a token (`HANDOFF_CONSOLE_TOKEN`,
  or a random one printed at startup). Sessions use an HttpOnly, SameSite=Strict
  cookie holding only the token's SHA-256. POSTs require an `X-HP-Request` header
  (CSRF), APIs are rate-limited (240/min/client), jobs expire after 30 minutes and
  time out at 15. Verified live: 401 unauthenticated, 403 without the header,
  200 with token; covered by `cmd/handoffproof/serve_test.go`.
- **C3 — demo stack exposed on 0.0.0.0, shared network.** All published ports bind
  `127.0.0.1` via `HANDOFF_*_PORT`; services carry resource limits and
  `no-new-privileges`. Sandbox traffic never leaves the internal network: the
  controller reaches services with `docker exec` (`execFetch`), not host ports.

### HIGH

- **H1 — secret files could enter model context.** New `internal/secretpath`
  classifier denies by meaning: any `.env` variant, key material
  (`.pem/.key/.p12/.keystore/...`), credential stores (`.npmrc`, `.pgpass`,
  `credentials.json`, service-account files, private keys). Enforced in the static
  scanner, the agent's `read_file`/`list_files`/`search_code`, and citation checks.
  Only `.env.example/.sample/.template/.dist` pass.
- **H2 — redaction was an allowlist of spellings.** `agent.Redact` now classifies by
  credential shape: private keys, AWS/GitHub/Slack/API-key and JWT shapes, any
  `Authorization` value, generic scheme connection strings, and quoted or short
  `key=value` credentials under any field name (JSON keys, camelCase, long names).
  Provider error text is redacted and truncated before surfacing. Regression tests
  probe variants that were **not** in the original pattern list
  (`internal/agent/redact_test.go`).
- **Symlink escapes.** Scanner and agent walks never follow symlinks;
  `Executor.safePath` resolves symlinks and requires the real path to stay inside
  the real workspace; the sandbox copy fails closed on any symlink in the app tree.
- **H3 — diff parser bypasses.** `ChangedPaths` handles rename/copy lines, quoted
  paths, `./` prefixes, `/dev/null`, and case-insensitive dedupe; protected-prefix
  checks run on normalized paths. Tests: `TestChangedPathsEdgeCases`,
  `TestDiffTouchesProtected`.
- **H4 — host environment leaked to compose.** `composeEnvBase()` passes an allowlist
  (PATH/HOME and tool basics) plus explicit per-run variables; the host's full
  environment (including API keys) is never inherited.
- **H5 — predictable sandbox tokens.** Mail/Admin/Scheduler tokens are crypto-random
  per run (`genToken`); nothing depends on documented dev defaults inside sandboxes.
- **H6 — unbounded resources.** Compose limits in both dev and sandbox modes; sink
  caps bodies at 1 MB and stores at most 1,000 messages; demo app caps JSON at 1 MB
  and uploads at 1 MB with input validation; outbound mail fetch times out at 5 s.
- **H7 — integrity claims were asserted, not measured.** Report `HashMatch` is now
  `ManifestVerify(root) == nil` at render time, and the assessment prompt receives
  controller-measured before/after hashes plus both verify results.
  `CheckCitations` resolves every cited `evidence_id` against real controller
  records — fabricated IDs fail (`unknown_ids`); citations escaping the workspace,
  pointing at symlinks, or at credential files are invalid.
- **H8 — console XSS via innerHTML.** Every data-driven interpolation in
  `cmd/handoffproof/web/index.html` passes through `esc()`; responses carry CSP
  (`frame-ancestors 'none'`, `base-uri 'none'`), `nosniff`, `no-referrer`,
  `X-Frame-Options: DENY`, and `Cache-Control: no-store` on APIs. Report HTML is
  `html/template`-escaped and every free-text field is redacted
  (`internal/report/report.go`).
- **H10/H11 — sink and demo-app abuse surface.** Sink `GET /messages` requires the
  admin token whenever `MAIL_SINK_REQUIRE_AUTH=1` (always set in sandboxes);
  admin comparisons are constant-time; `/health` no longer discloses message counts.
  Demo app validates registration input and caps payload sizes.
- **H12 — boundary coverage.** Evidence package 0% → 100%, score 2.2% → 69.6%,
  scan 91.7%, secretpath 100%; console auth/CSRF/rate-limit tests added.
  Controller coverage (11.3%) is mostly Docker-bound paths, exercised by the four
  live drills instead of unit tests.
- **H13 — supply chain.** Demo image builds with `npm ci` from the committed
  lockfile; both images run as `USER node`; CI workflows declare
  `permissions: contents: read`.

### MEDIUM / LOW (summary)

Security headers and job TTL/timeout on the console; report redaction extended to
all fields; mail-sink timing-safe admin checks and quota caps; demo input
validation; sandbox teardown errors logged instead of swallowed; free-port failures
clean up the sandbox directory; Go-native manifest verification (no shelling out to
the verified script).

## Operating rules for production use

1. Set `HANDOFF_CONSOLE_TOKEN` (16+ chars) before `handoffproof serve`; keep the
   default loopback bind. Expose beyond loopback only behind your own TLS proxy.
2. Keep `.env` out of the repository (git-ignored). The Meta key is read from the
   environment only and is never logged.
3. Run `bash scripts/check.sh` before use; it re-verifies the protected-path
   manifest. Re-freeze with `bash scripts/manifest.sh create` only for reviewed
   controller changes.
4. Treat every report, diagnosis, and diff as untrusted output until the controller
   rerun passes. The console states this in its footer; keep it that way.

## Residual risks (accepted, documented)

- Prompt injection can still influence model *text* (never verdicts, evidence, or
  accepted diffs — those gates are deterministic).
- Pattern redaction is best-effort: the deny-by-meaning path classifier is the hard
  boundary; redaction is defense in depth, not a proof.
- Docker and npm supply chains are trusted at their pinned tags/lockfiles; image
  digest pinning is a recommended follow-up.
- The console token is a single shared secret (no per-user accounts); suitable for
  a single-operator handover tool, not a multi-tenant service.
