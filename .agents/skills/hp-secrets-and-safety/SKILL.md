---
name: hp-secrets-and-safety
description: Use when handling API keys, env files, Docker configuration, sandbox networking or test data. Keeps secrets out of the repo and the sandbox isolated.
---
# Secrets and sandbox safety (constraint)

## Secrets
- Read `META_API_KEY`, `META_MODEL_ID` from the environment. Never print, log or commit them.
- `.env` and `auth.json` stay gitignored. Run `git log -p | grep -i "api_key"` before submitting.
- Use only synthetic credentials and synthetic customer data in the demo app and the sandbox.

## Sandbox
- One isolated Docker network per drill, internal only (no internet egress).
- No `--privileged`. Never mount the Docker socket or host home directories into a sandbox container.
- Mount the repository copy read-only unless the step needs writes, and write only to a scratch volume.
- Set CPU, memory and time limits on every container. Tear everything down after each drill.

## Verify
- `docker network inspect` shows `Internal: true` for the drill network.
- `git grep -n -i -E "(api[_-]?key|secret|token)\s*=\s*['\"][A-Za-z0-9]{16,}"` returns nothing.
