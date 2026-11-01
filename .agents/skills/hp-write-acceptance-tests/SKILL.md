---
name: hp-write-acceptance-tests
description: Use when writing or changing the Playwright acceptance tests in acceptance/ that independently judge registration, booking and confirmation email.
---
# Task: acceptance tests

## Inputs
The running demo app, the mail sink's `GET /messages` endpoint, and read access to the database.

## Write (Playwright + TypeScript, strict mode)
- `registration.spec.ts`: a new customer registers; the user row exists.
- `booking.spec.ts`: a customer books; the booking row exists.
- `confirmation-email.spec.ts`: after booking, the mail sink holds exactly one message for that
  customer. This catches silent failures that the UI hides.
- `documents.spec.ts` (recovery): an uploaded file exists in storage and its checksum matches.

## Rules
- Assertions use independent evidence (database, sink, storage), never the app's own success message.
- No fixed sleeps. Poll with a timeout.
- Each test creates unique data and cleans up, so reruns are repeatable.
- Never mock the system under test.

## Done when
All pass on the healthy baseline and the email test fails after the token is revoked.
Then freeze: `bash scripts/manifest.sh create` (see hp-protected-paths).
