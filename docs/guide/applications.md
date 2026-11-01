# Test your own application

HandoffProof can rehearse an application you own. The application must run
with Docker Compose. HandoffProof copies it into a server-side store, boots
it in an isolated sandbox with no published ports, runs the workflows you
declare, and writes controller evidence with the application ID and source
hash.

## 1. Add the application

Open **Applications -> Add application**.

Choose:

- **Local folder on this server** for an application already on the host, or
- **GitHub repository** after connecting GitHub in Settings.

The imported source is copied. Secret-like files (`.env`, key and credential
files) and symlinks are rejected or skipped by the same secret-path rules
used by the scanner. Build and runtime folders (`node_modules`, `dist`,
`backups`, `data`) are skipped only at the application root — a nested
source folder with one of those names, such as `engine/src/data`, is
application code and is copied.

## 2. Write the profile

The wizard starts from a sample `handoffproof.json`. Edit it to describe
your application:

- `services`: Compose service names, container ports, and optional health
  paths.
- `app_service`: the main service.
- `probe_runtime`: `node`, `curl`, `wget`, or `python3`. The runtime must
  exist inside the application container; HandoffProof uses it to make
  in-sandbox HTTP checks.
- `env`: every environment variable name the workflows need. Mark secrets.
- `credentials`: a logical name mapped to one env var, for example
  `mail -> MAIL_API_TOKEN`.
- `workflows`: ordered checks. Supported steps are `http`, `sql_count`
  (a single `SELECT COUNT` query), `log_contains`, and `file_exists`.
- `data_stores` and `backup`: optional PostgreSQL/filesystem stores and
  allowlisted backup/restore commands. Backup commands may not use a shell.

Profiles contain names and structure only. Never put secret values in the
profile.

## 3. Add test environment values

Enter one `KEY=value` per line. Use sandbox/test credentials only. Values
are AES-GCM encrypted on the server and are never returned to the browser.

## 4. Validate and scan

Click **Validate** after import. HandoffProof checks the profile, every
referenced service, workflow dependencies, SQL shape, backup commands, and
the Compose file. Dangerous Compose constructs (privileged mode, Docker
socket, host mounts, host networking) are rejected.

**Scan** shows the deterministic environment/credential/data-store signals
HandoffProof found in the imported source.

## 5. Run rehearsals

- **Baseline**: every declared workflow should pass.
- **Withdraw credential**: choose a profile credential name. HandoffProof
  reruns with that credential invalidated; the workflow depending on it is
  expected to fail while the run records the result.
- **Missing configuration**: enter an env var name. HandoffProof reruns
  without it; the dependent workflow is expected to fail.
- **Incomplete backup**: requires a configured backup. HandoffProof runs
  dump, recreates the stack, restores, and reruns the workflows — then
  starts fresh once more *without* restoring and reruns them again. The
  fault is counted as detected only when a workflow that passed after the
  real restore now fails; if no workflow reads restored data, the run
  honestly reports “not detected”.

HTTP steps may send custom `headers`; reference configured environment
values with `${ENV_NAME}` so secrets stay out of the profile, for example
`"headers": { "x-api-key": "${API_KEY}" }`.

Results appear in **Evidence** with the application ID, profile version,
source hash, and fault target. Use **Insights** and the full report as usual.

## Current limits

- Docker Compose applications only.
- One server-side application store per HandoffProof server; the demo
  Control Room pages (Readiness, Rehearse, scoreboard) still describe the
  bundled demo application. BYOA runs write to the shared Evidence store.
- HTTP steps do not capture values from one response for the next step yet.
- The backup check compares workflow behaviour after restore versus after
  a skipped restore; it does not yet compare per-file hashes for arbitrary
  stores.

## Troubleshooting

- **Compose rejected**: remove privileged mode, host mounts, Docker socket
  access, devices, or host networking from the Compose file.
- **Did not become healthy**: check the health path and that the probe
  runtime exists in the container.
- **Required environment value is not configured**: add the value in the
  wizard or make the profile variable optional.
- **No workflow depends on target**: add the env var to that workflow's
  `depends_on` list.
