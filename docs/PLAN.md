# HandoffProof — 3-day build plan (Step 1)

Status: APPROVED 2026-11-01. Day 1 in progress. Decisions recorded in `docs/DECISIONS.md`.
Date: 2026-11-01. Deadline: Nov 4, 9:00 PM ET. Submit target: Nov 4, ~3:00 PM ET (6+ hours early).
Inputs read: `AGENTS.md`, all 16 skills in `.agents/skills/`, `prompts/handoffproof_agent_prompt.json`.
None of those files were modified.

## Goal

Build a Go controller plus a Meta-model-powered agent that rehearses "the developer
disappears" for a small Node.js + PostgreSQL + file-storage app. V1 scope: three
workflows (registration, booking, confirmation email), three drills (withdrawn test
token, missing configuration, incomplete backup), one recovery target (fresh local
environment), one evidence view.

## Success Criteria

- `bash scripts/check.sh` passes; manifest verifies before and after every drill and rerun.
- One fault goes red to green with identical acceptance hashes, shown with before/after evidence IDs.
- The Meta model does real work on screen (dependency map, drill plan, diagnosis from
  controller evidence, repair proposal) and never issues a verdict.
- `docs/RESULTS.md` holds measured numbers only; misses reported plainly.
- Demo video under 3:00, submitted on the indie track with all contributors listed.

## Verified Meta Model API details (dev.meta.ai, fetched 2026-11-01)

Decision: use the Responses API with `muse-spark-1.3` as the default model
(overridable via `META_MODEL_ID`, e.g. `muse-wonder-1.0`). Rationale: the docs call
Responses "the recommended default for new work" and prescribe it for "agentic and
coding workloads where reasoning continuity across tool turns matters"; our tool loop is
exactly that. Same models, same bearer auth, same per-token cost on all three protocols.

| Item | Verified value |
| --- | --- |
| Base URL | `https://api.meta.ai/v1` |
| Responses | `POST /v1/responses` — recommended default for new agentic/coding work |
| Chat Completions | `POST /v1/chat/completions` — drop-in; no reasoning carryover; no search grounding / tool search |
| Messages | `POST /v1/messages` — Anthropic-compatible; point SDK at base host `https://api.meta.ai`; stateless |
| Auth | `Authorization: Bearer $MODEL_API_KEY`; official env var `MODEL_API_KEY`, key from the Model API dashboard |
| Models | `muse-spark-1.3` (latest, "recommended for new work", agentic-tuned); `muse-wonder-1.0` (most capable, same requests as Spark); `muse-spark-1.2` / `1.1` previous; `-contributor` variants discounted |
| Unknown model | `404` + `code: model_not_found`; a `503` on model lookup is inconclusive — retry |
| Structured output | Chat: `response_format` with `type: "json_schema"` + `json_schema`. Responses: `text.format`. A param from the other endpoint does not configure structured output (`text.format` on Chat returns 400). Decoding is constrained, output guaranteed to conform. Recursive `$ref` schemas rejected |
| Sampling | `temperature` 0–2, default 1.0. `top_p` in (0,1], default 1.0, `0` returns 400. Spark is tuned for the defaults; adjust one, not both; prefer clearer instructions over lowering temperature |
| Output limits | Responses: `max_output_tokens`, min 16, covers reasoning + visible output, shares the context window. Chat: `max_completion_tokens` (`max_tokens` alias accepted). `max_output_tokens` is rejected on Chat |
| Reasoning | `reasoning.effort` (Responses) / `reasoning_effort` (Chat): `none`, `minimal`, `low`, `medium`, `high`, `xhigh`. On by default. `none` returns 400 on Muse Spark |
| Tools | `function` or `custom` tools (`custom` is Responses-only). Responses: flat format, `function_call` items + `function_call_output` with `previous_response_id`. Chat: nested `function` key, `tool_calls` array + `tool` messages. Names must match `^[a-zA-Z0-9_.-]+$` with at most one dot. `strict` defaults false. `parallel_tool_calls` defaults true. `max_tool_calls` caps built-in tools only — bound function calls in our own loop. `tool_choice` must be `"auto"`. Always validate tool args client-side |
| Instructions | Responses `instructions` is developer-level and per-request only; resend `instructions`, `tools`, `reasoning.effort` on every turn |
| Usage | Responses: `{input_tokens, output_tokens, total_tokens, input_tokens_details: {cached_tokens}, output_tokens_details: {reasoning_tokens}}`. Preflight: `POST /v1/responses/input_tokens` |
| Errors | `type`: `invalid_request_error` (4xx), `authentication_error` (401), `rate_limit_error` (429), `server_error` (5xx). Branch on status + `type`, not message. 429: exponential backoff with jitter, honor `Retry-After`. 500: retry with backoff. 503 (`server_shutting_down`, `service_overloaded`, `backend_unavailable`): always retryable, honor `Retry-After` |

JSON-spec parameter translation (the JSON mixes protocols, see C8): `temperature` and
`top_p` omitted (defaults; revisit only if eval shows unstable output), `max_output_tokens 4096`,
per-phase schema sent via `text.format`, `retry_on_schema_violation 2` then fail closed.
Implement over Go `net/http` (stdlib-first per `AGENTS.md`; docs show OpenAI/Anthropic
SDKs, same wire format). Config: `META_API_KEY` with fallback to official
`MODEL_API_KEY`; `META_MODEL_ID` required, never hardcoded; `META_API_BASE_URL`
overrides the base. Record all of this in `docs/DECISIONS.md` on Day 2.

Not verified in this pass (do not rely on): per-model context-window and max-output
caps (model-dependent — handle 400s and read `GET /v1/models` at runtime); pricing and
quota numbers; hackathon submit-page rules (re-read at submission per
hp-submission-checklist).

## Agent/controller interface

CLI (`cmd/handoffproof`): `scan` = static scan + intake + dependency proposal (JSON
phases 1–2); `drill` = drill plan + execution + diagnosis + repair proposal + rerun
(phases 3–7); `report` = static HTML evidence report.

- `internal/agent` (hp-meta-api-client): renders the system prompt by joining the JSON
  arrays; Meta client; tool loop. Validates every tool call against the allowlist and
  budget (40/phase, 200 total) and every response against the phase schema (2 retries,
  then fail closed). Logs token usage per run. Never imports controller verdict code.
  Read-only tools may batch; `request_drill` and `submit_repair` are sequential.
- `internal/controller` + `internal/drills` (hp-implement-drill, hp-verdict-integrity):
  only these decide pass/fail and write `evidence.Record` (ID `EV-` + 8 hex via
  `evidence.NewID`). Drill interface: Name, ValidateParams (allowlist), Setup (fresh
  sandbox), Inject, Observe, Teardown (always runs). Manifest verify before and after
  every run; abort on mismatch. Repairs apply to a fresh sandbox copy of `demo-app`
  only, never the original; rerun uses the identical suite and hashes.
- Tools (9, no verdict tool by design): `list_files`, `read_file`, `search_code`
  (read-only, sandboxed copy, size-limited, wrapped as untrusted data; `eval/` and
  secrets excluded); `static_scan_report` (deterministic ground truth, call first);
  `read_ownership_declaration`, `request_ownership_confirmation`; `request_drill`
  (controller may refuse; fresh baseline gates the rest); `read_evidence` (by
  `evidence_id`, terminal status only); `submit_repair` (unified diff, protected paths
  rejected).
- Finding classes: exactly one of `observed_failure`, `observed_pass` (both require a
  controller `evidence_id`), `inferred_risk`, `untested_dependency`. Confidence capped
  at 0.9 without runtime evidence, 0.97 with it.

## Conflicts: JSON spec vs folder layout

Nothing below was changed in Step 1. Each fix needs user approval at its build step.

| # | JSON spec says | Repo says | Resolution |
| --- | --- | --- | --- |
| C1 | `protected_paths`: `/acceptance/**`, `/drills/**`, `/evidence/**`, `/controller/**`, `/manifest.sha256` | `prompts/README.md` + skill: `acceptance/**`, `internal/controller/**`, `internal/drills/**`, `evidence/**`, `.manifest.sha256` (+ `scripts/manifest.sh` in skill and manifest) | JSON paths are wrong (missing `internal/` prefix and the manifest's leading dot). Treat the skill list as normative; fix the JSON only with approval |
| C2 | — | `scripts/manifest.sh` hashes `acceptance internal/controller internal/drills scripts/manifest.sh` — omits `evidence/`; `AGENTS.md` invariants omit `scripts/manifest.sh` | Keep `evidence/` out of the manifest (runtime output, gitignored) but the controller still rejects repair diffs touching it. Add `scripts/manifest.sh` to the `AGENTS.md` list for consistency (Day 1, with approval) |
| C3 | Manifest rule implies hashing `evidence/` | `.gitignore` ignores `evidence/*` except `.gitkeep` | Manifest covers committed files only; runtime evidence integrity comes from controller-written records plus the report's hash footer |
| C4 | — | `internal/evidence/` (defines `Record` + classes) is in no protected list | Add `internal/evidence/**` to the protected set — altering it would break verdict integrity (Day 1, with approval) |
| C5 | R7 + example: report injections as `prompt_injection_attempt` | Output schemas and `internal/evidence/evidence.go` have only the 4 classes | Do not shoehorn into the 4 classes. Log as a separate controller security event and surface it in the report (schema defined Day 2) |
| C6 | Workflows: registration, booking, confirmation_email | hp-write-acceptance-tests adds `documents.spec.ts`; scaffold adds `POST /uploads` | Uploads/storage is drill support for `incomplete_backup` assertions, not a 4th customer workflow. Keep 3 workflows + recovery spec |
| C7 | Baselines: `static_scan_only`, `fresh_deployment_only`, `checklist_only` | hp-eval-results-table arms drop `checklist_only` | Run the 2 automated arms + full; `checklist_only` only if time permits and as a labeled target |
| C8 | Params mix protocols: `temperature 0.1` + `top_p 0.9` (docs: one, not both; prefer 1.0 for Spark) and `max_output_tokens` (Responses-only) + `response_format` (Chat-only) | hp-meta-api-client: temperature 0.1 | Responses API; omit temperature and top_p (defaults); `max_output_tokens 4096`; schemas via `text.format`. Revisit only if eval shows unstable output |
| C9 | `model_id: ${META_MODEL_ID}` | Official key env var is `MODEL_API_KEY`, repo uses `META_API_KEY` | Read `META_API_KEY`, fall back to `MODEL_API_KEY`; document in `docs/DECISIONS.md` |
| C10 | Example `EV-0412` | `evidence.NewID` yields `EV-` + 8 hex | Code is normative; the example is illustrative |
| C11 | `request_drill` / `drill_plan` include `fresh_deployment_baseline`, but `supported_scope_v1.drills` lists only 3 | hp-implement-drill: baseline is the gate | No change — baseline gates the 3 drills, it is not counted as one |
| C12 | Ownership tools named, storage not specified | No ownership file in the layout | Day 2 decision: controller-held file outside the repo under test; record in `docs/DECISIONS.md` |
| C13 | "Fresh sandbox copy of the repo" (repair target ambiguous) | — | Repairs apply to a `demo-app` copy only; the controller never patches itself at runtime |

## 3-day build order

Always-on constraint skills: hp-verdict-integrity, hp-protected-paths,
hp-untrusted-input, hp-secrets-and-safety, hp-hackathon-rules. After every step run
`bash scripts/check.sh` and report the command + result. Priority when short on time
(hp-hackathon-rules): working E2E demo with the Meta model doing real work, then video,
then one honest results table; cut extra drills, polish, second app.

Day 1 — deterministic core, no model needed.

| Step | Skill | Output | Done when |
| --- | --- | --- | --- |
| 1.1 Mail sink | hp-build-mail-sink | `mocks/mail-sink`: `POST /send` (Bearer), `GET /messages`, `POST /admin/revoke\|reset` (controller-only admin token), request logging without full tokens | Test: send OK, revoke, send 401, message count unchanged |
| 1.2 Demo app | hp-scaffold-demo-app | `demo-app` Node 20 + Express (TS), Compose PostgreSQL, users/bookings/documents, `POST /register\|/bookings\|/uploads`, `GET /health`, `MAIL_API_TOKEN` via sink, `SCHEDULER_TOKEN` job, `/data/uploads`, DB-only `backup.sh`, incomplete `HANDOVER.md`, silent-failure booking handler, one injection comment, `eval/ground_truth.json` outside the agent's reach | `docker compose up`; 3 workflows pass by hand |
| 1.3 Static scan | hp-static-scan | `internal/scan`: env read-vs-documented, jobs, upload paths, backup coverage, hardcoded hosts, `ask <name>` phrases; stable sorted JSON | Fixture tests pass; demo-app missing var detected; scan-only baseline recorded |
| 1.4 Acceptance + freeze | hp-write-acceptance-tests, hp-protected-paths | `acceptance/`: registration, booking, confirmation-email, documents specs; independent assertions, polling, unique data | Green on baseline; email test fails after revoke; `manifest.sh create`; folder frozen |

Day 2 — controller, drills, agent loop (Meta API goes live).

| Step | Skill | Output | Done when |
| --- | --- | --- | --- |
| 2.1 Agent client | hp-meta-api-client | `internal/agent`: Responses client, prompt render, tool loop with budget, schema validation, usage logging, 429/5xx backoff; `docs/DECISIONS.md` updated | Recorded run: intake, dependency map, drill plan, diagnosis from real evidence |
| 2.2 Drills | hp-implement-drill | `internal/drills` + controller runner: baseline gate + drills in order withdrawn_token, missing_configuration, incomplete_backup (last only if time remains); isolated internal Docker network, limits, teardown-always; controller-written evidence | Param-validation tests; one real run yields evidence records with IDs |
| 2.3 Repair + rerun | hp-repair-and-rerun | Diff review (protected reject), sandbox-copy apply, hash check, identical-suite rerun, before/after IDs | One fault red to green, same acceptance hashes |
| 2.4 Report | hp-evidence-report | `internal/report`: single offline HTML file, class badges, per-finding evidence/log/source/impact/confidence, inferred/untested sections, integrity footer; escaped + redacted | Sample run renders; each finding links to its record |

Day 3 — honest numbers, video, submission.

| Step | Skill | Output | Done when |
| --- | --- | --- | --- |
| 3.1 Results | hp-eval-results-table | `docs/RESULTS.md`: per-arm faults injected/caught/missed, false alarms, confirmed/refuted risks, tokens/run; date, commit, evidence IDs | Table contains measured numbers only; one miss reported plainly if present |
| 3.2 Video | hp-demo-video | `docs/DEMO_SCRIPT.md` + recorded take: problem, baseline, token drill, evidence view with model diagnosis, docs drill, repair + green rerun, results | Safe take recorded (<3:00, readable, no secrets); captions present |
| 3.3 Submit prep | hp-submission-checklist | Public repo, README event section + setup + video link, secrets sweep incl. history, form fields, indie track, contributors | All 7 checklist items verified; official rules re-read |

Nov 4 morning is contingency only; submit by ~3:00 PM ET.

## Validation Plan

- Every step: `bash scripts/check.sh` (gofmt, vet, tests, skills format, manifest).
- Sandbox: `docker network inspect` shows `Internal: true`; no `--privileged`, no socket/home mounts; teardown verified.
- Secrets: `git grep -n -i -E "(api[_-]?key|secret|token)\s*=\s*['\"][A-Za-z0-9]{16,}"` empty; pre-submit `git log -p` sweep.
- Verdict integrity: `grep -rn "Passed" internal --include=*.go | grep -v internal/controller | grep -v internal/evidence` shows no assignments; every observed claim carries an `evidence_id`.
- Untrusted input: unit test with injection string + fake key asserts delimiters, `[REDACTED:...]`, unchanged verdicts.
- E2E: healthy baseline green, token drill red (email assertion), diagnosis cites evidence, repair rerun green with unchanged hashes.
- Highest risk: Day 2.1 — first real Meta API run (auth, model ID, `text.format`, tool loop). Validate with a minimal live call before wiring all phases.

## Risks / Open Questions

- R1: Default sampling may vary. Mitigation: eval watches for unstable output; revisit temperature/top_p only if eval shows instability.
- R2: Docker Desktop + Playwright + Node 20 setup on Windows 11/Git Bash may eat Day 1. Mitigation: sink + scan first (no Docker needed for scan unit tests).
- R3: API key availability/quota unknown. Mitigation: Day 2 starts with one minimal live call; keep a recorded-evidence replay path so controller/report work continues offline.
- Q1: Held-out fault needs someone outside the implementation — who, and when? (Else eval runs on planted faults only, labeled as such.)
- Q2: Approve C1/C2/C4/C8/C9 resolutions before Day 1 edits? (Step 1 changed nothing.)

## Sources

Only pages fetched with non-empty content from dev.meta.ai during this run:

- https://dev.meta.ai/
- https://dev.meta.ai/docs/overview
- https://dev.meta.ai/docs/quickstart
- https://dev.meta.ai/docs/authentication
- https://dev.meta.ai/docs/models
- https://dev.meta.ai/docs/structured-output
- https://dev.meta.ai/docs/protocols
- https://dev.meta.ai/docs/tool-calling
- https://dev.meta.ai/docs/error-handling
- https://dev.meta.ai/docs/token-counting
- https://dev.meta.ai/docs/protocols/responses
- https://dev.meta.ai/docs/protocols/chat-completions
