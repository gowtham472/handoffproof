---
name: hp-build-mail-sink
description: Use when building or changing mocks/mail-sink, the local mail provider that validates a token so token withdrawal is deterministic.
---
# Task: mail sink

## Build (under 150 lines, Node or Go)
- `POST /send`: requires `Authorization: Bearer <token>`. Valid token: store the message, return 200.
  Revoked or unknown token: return 401 and store nothing.
- `GET /messages?to=<address>`: lists stored messages for assertions.
- `POST /admin/revoke` and `POST /admin/reset`: callable only with a separate admin token that
  only the controller knows. The demo app must not know it.
- Persist state in memory; reset between drills.

## Rules
- Log every request (method, path, status, token fingerprint, never the full token).
- No real network egress, no real email.

## Done when
A test shows send OK, then revoke, then send returns 401, and the message count stays unchanged.
