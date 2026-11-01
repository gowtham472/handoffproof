---
name: hp-scaffold-demo-app
description: Use when asked to build or modify the demo booking application under demo-app/. Produces a small Node.js + PostgreSQL app with deliberately planted handover faults.
---
# Task: build the demo app

## Inputs
None. Output goes in `demo-app/`. Keep it under about 400 lines of source.

## Build
- Node 20 + Express (TypeScript preferred), PostgreSQL via `docker-compose.yml`.
- Tables: users, bookings, documents.
- Endpoints: `POST /register`, `POST /bookings` (sends a confirmation email), `POST /uploads`,
  `GET /health`.
- Confirmation email goes through an HTTP mail provider using `MAIL_API_TOKEN` (the mail sink mock).
- A scheduled reminder job using `SCHEDULER_TOKEN` from the environment.
- Uploaded files stored under `/data/uploads`, a volume outside the database.

## Plant these faults (ground truth)
1. `.env.example` omits one variable the code reads (missing configuration).
2. The booking handler catches the mail provider error and still returns success (silent failure).
3. `backup.sh` dumps only the database, not `/data/uploads` (incomplete backup).
4. `docs/HANDOVER.md` says "ask Ravi for the mail key" (documentation gap, an untested dependency).

## Ground truth location
Write the planted faults to `eval/ground_truth.json`, outside `demo-app/` and excluded from anything
the agent can read. Never mention them in code comments or docs inside `demo-app/`.

## Done when
`docker compose up` brings the app up, and the three workflows work by hand.
