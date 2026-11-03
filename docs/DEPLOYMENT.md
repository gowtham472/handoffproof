# Deployment Guide

How to run HandoffProof for real users: one Go binary, a Docker host for the
drill sandboxes, and a browser. This guide covers production setup, the
environment variables that matter, HTTPS, the Slack and GitHub integrations,
day-2 operations, and troubleshooting.

## 1. Architecture at a glance

| Piece | What it is | Needs |
| --- | --- | --- |
| `handoffproof serve` | CLI + Control Room web console + APIs (one Go binary) | Port `127.0.0.1:8787` behind your HTTPS proxy/tunnel |
| Drill sandboxes | Temporary Docker Compose stacks per rehearsal | Docker Engine + Compose v2 on the same host |
| Evidence | JSON files under `evidence/` next to the working directory | Disk + backup |
| Integration stores | Encrypted Slack/GitHub/BYOA files **outside** the repo | Disk + backup, `0600` permissions |
| Meta Model API | Diagnosis, map, and repair proposals (`muse-spark-1.3`) | `META_API_KEY`, outbound HTTPS |

The Go controller owns every verdict. The model proposes and explains only;
if the API key is missing, agent phases are skipped and labelled as skipped —
nothing is fabricated.

## 2. Prerequisites

- Go 1.27+ (build only; the binary is self-contained afterwards)
- Docker Engine with Compose v2, running, with permission to build images
- A Meta Model API key from [dev.meta.ai](https://dev.meta.ai)
- For Slack: a public **HTTPS** URL for the console (Slack rejects HTTP
  redirect URIs). GitHub also accepts a loopback callback for local use.

## 3. Configure

Copy the template and fill in values. `.env` is git-ignored; never commit it.

```bash
cp .env.example .env
```

| Variable | Required | Purpose |
| --- | --- | --- |
| `META_API_KEY` | Yes, for agent features | Meta Model API key |
| `META_MODEL_ID` | Yes | `muse-spark-1.3` |
| `META_API_BASE_URL` | No | Defaults to `https://api.meta.ai/v1` |
| `HANDOFF_CONSOLE_TOKEN` | Yes | Console login token (long, random) |
| `HANDOFF_PUBLIC_URL` | For Slack/GitHub OAuth | Public HTTPS origin of the console, no trailing path |
| `HANDOFFPROOF_OWNERSHIP_FILE` | No | Controller-held ownership declaration file outside the repo |
| `HANDOFF_SLACK_CLIENT_ID` / `HANDOFF_SLACK_CLIENT_SECRET` | For Slack | From your Slack app (see [Connect Slack](guide/connect-slack.md)) |
| `HANDOFF_SLACK_STORE` / `HANDOFF_SLACK_ENCRYPTION_KEY` | No | Encrypted webhook store location / key (auto-generated sidecar key otherwise) |
| `HANDOFF_SLACK_WEBHOOK` | No | Operator-managed fallback webhook instead of OAuth |
| `HANDOFF_GITHUB_CLIENT_ID` / `HANDOFF_GITHUB_CLIENT_SECRET` | For GitHub | From your GitHub OAuth app (see [Connect GitHub](guide/connect-github.md)) |
| `HANDOFF_GITHUB_APP_ID` / `HANDOFF_GITHUB_APP_PRIVATE_KEY` | For GitHub App mode | App ID and full multiline RSA PEM from the server's secret environment; Contents/Metadata read permissions, selected repositories only |
| `HANDOFF_GITHUB_STORE` / `HANDOFF_GITHUB_ENCRYPTION_KEY` | No | Encrypted token store location / key |
| `HANDOFF_SHARE_FILE` | No | Persistent read-only share-link store (SHA-256 hashes only) |
| `HANDOFF_SLACK_BASE_URL`, `HANDOFF_GITHUB_BASE_URL`, `HANDOFF_GITHUB_API_BASE_URL` | No | Endpoint overrides (enterprise/testing) |

Configuration is read at startup. Restart the server after editing `.env`.

## 4. Build and run

```bash
go build -o handoffproof ./cmd/handoffproof
./handoffproof serve --addr 127.0.0.1:8787
```

The server prints a token URL. Put it behind HTTPS before sharing it:

- **Tunnel (quick):** `cloudflared tunnel --url http://127.0.0.1:8787` or
  `ngrok http 8787`, then set `HANDOFF_PUBLIC_URL` to the HTTPS origin and
  restart.
- **Reverse proxy (durable):** Nginx/Caddy terminating TLS in front of
  `127.0.0.1:8787`. Keep the app bound to loopback; do not publish the drill
  sandbox ports (the controller never publishes any).

Run it as a service with your init system (systemd/Windows Service/NSSM).
Working directory must be the repository root so `evidence/`,
`.manifest.sha256`, and `prompts/` resolve.

## 5. Connect Slack and GitHub

Follow the step-by-step guides, including the exact redirect URLs to
register:

- [Connect Slack](guide/connect-slack.md) — OAuth app, `incoming-webhook`
  scope, HTTPS callback, channel posting, disconnect.
- [Connect GitHub](guide/connect-github.md) — OAuth app with PKCE, `repo`
  scope, repository/branch selection, importing applications.

Only console operators can connect, post, disconnect, or manage
applications. Read-only share recipients can do none of these.

## 6. Day-2 operations

- **Evidence** lives in `evidence/` (JSON records, diagnoses, reports,
  `ownership-requests.jsonl`). Back it up with the integration stores.
- **Stores** (Slack/GitHub/BYOA/share files) live outside the repo at the
  paths printed in Settings or set via the `*_STORE` variables. Losing a
  store means reconnecting that integration; losing `evidence/` means
  losing history, not safety.
- **Share links** are read-only, GET-only, expire after 24 hours, and now
  persist across restarts (only SHA-256 hashes are stored). Mint them from
  Evidence → Full report.
- **Scores** are recomputed from evidence on every `/api/score` call and in
  `handoffproof score`. No number in the UI is typed by hand.
- **Updates:** `git pull`, rebuild, restart. The quality gate
  (`bash scripts/check.sh`) must pass before you deploy a change; protected
  paths are verified against `.manifest.sha256` at runtime.
- **Ownership requests** made by the agent are appended to
  `evidence/ownership-requests.jsonl` as `pending_human_confirmation`. A
  human resolves them outside the tool; the agent can never confirm
  ownership itself.

## 7. Production rules

From [SECURITY.md](SECURITY.md); the short version:

- Keep the console bound to loopback behind authenticated HTTPS.
- Treat share links as read credentials: anyone with the link can read.
- Never commit `.env`, store files, or keys. Rotate the console token if it
  leaks; revoke Slack/GitHub apps from their own settings when decommissioning.
- Run drills only on hosts where Docker is trusted: BYOA executes the
  onboarded application's code inside the sandbox by design.
- Keep Docker and the Go toolchain patched; re-run `bash scripts/check.sh`
  and the acceptance suite after upgrades.

## 8. Troubleshooting

| Symptom | Check |
| --- | --- |
| Console returns 401 | Open the printed token URL once (sets the session cookie), or send `Authorization: Bearer <token>` |
| Slack connect blocked | `HANDOFF_PUBLIC_URL` must be HTTPS and match the registered redirect URL exactly |
| GitHub connect works locally but not remote | Register the HTTPS callback in the OAuth app; loopback callbacks only work locally |
| Drill fails immediately | Docker daemon running? `docker info`. Images build on first run — allow a few minutes |
| BYOA header-authenticated endpoint fails | Add a `headers` entry to the step; use `${ENV_NAME}` to reference configured env values |
| Report looks stale | `/report` regenerates automatically when evidence is newer; or run `handoffproof report` |
| Share link 401 after restart | Store file moved/deleted, or the 24h expiry passed — mint a new link |
| Agent phases skipped | `META_API_KEY`/`META_MODEL_ID` missing or invalid; deterministic scan/drill results are unaffected |

## 9. Verify the deployment

```bash
bash scripts/check.sh                                  # quality gate
go test ./...                                          # full test suite
./handoffproof drill --drill fresh_deployment_baseline # live sandbox proof
./handoffproof eval                                    # regenerate docs/RESULTS.md
```

Then open the console, run one fault drill from Rehearse, and confirm the
new evidence appears in Evidence and the score moves.
