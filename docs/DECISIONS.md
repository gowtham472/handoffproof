# Decisions

## F5: Private mode with Muse Glimmer (2026-11-03)

Decision: `HANDOFF_PRIVATE_MODE=off|local|hybrid` on top of the F1 router, in a
new unprotected package `internal/privatemode`. Local: every phase routes to the
loopback Glimmer endpoint and the HTTP client is wrapped in a guard that refuses
non-loopback hosts before dialing (logged as `security event:
private_mode_egress_blocked`). Hybrid: intake_summary, dependency_map and
diagnosis go local; any phase whose route is not loopback runs with
`Executor.DenySource` (list_files/read_file/search_code/static_scan_report
refused). Every phase log line now carries `mode=`; the Control Room shows a
"Private mode" badge from `/api/private-mode` (no key returned).
Options considered: (a) a mode field in controller evidence records — rejected:
drill records are model-independent and `internal/evidence` is protected, so the
mode is recorded in the agent's per-phase evidence log line instead; (b) a
separate executor wrapper in `cmd` — rejected: every call site would change,
while one per-phase copy in `RunPhase` covers them all; (c) only rewriting
routes without a transport guard — rejected: a stray caller (e.g. transcription)
could still leak, so local mode guards the client too.
Verified on dev.meta.ai (2026-11-03): Ollama tag `muse-glimmer` (Ollama
0.32.7+, OpenAI-compatible `/v1`); vLLM image `vllm/vllm-openai:muse-glimmer`
with `--tool-call-parser muse_glimmer --reasoning-parser muse_glimmer
--enable-auto-tool-choice`; llama.cpp build b10353+ selects the ATEM tool parser
from the template. Hardware: bf16 about 72 GB VRAM to serve; quantized GGUF
builds 17–20 GB; `KQuant-17GB-Q4_K_M` with vision projector and full 131,072
context totals 19.0 GiB (fits a 24 GiB card); `Dynamic-Q4_K_XL` (19.7 GB) is the
higher-quality, higher-memory option; Apple silicon needs 48 GB+ unified memory.
The docs do not state structured-output (`response_format` json_schema) support
for Glimmer, so schema conformance is unverified.
Limits, stated plainly: hybrid cloud phases still receive model-written outputs
of the local phases (secret-redacted, but they may paraphrase source); quality
vs Spark is NOT measured — no local runtime or GPU on the build machine, so the
F2 glimmer rows stay "not run" with the recorded reason.

## F2 follow-up: render-only regen (2026-11-04)

`--models` always executed cells, so regenerating the table meant rerunning
hours of live cells. Added `--render-only`: skips execution, renders the same
`renderModelTable` section from the cell.json files already under
`evidence/eval` (verified: fresh binary regenerated RESULTS.md in seconds with
12/12 cells + 2 not-run rows). Rationale: the table stays reproducible and
hand-typed numbers stay impossible, without extra model spend.

## F4: Exit Interview Verifier (2026-11-04, in progress)

Voice: file endpoint `POST /v1/asr/transcribe`, multipart `request` JSON
(model/audioEncoding WAV/mode) + `audio` WAV part (<=10 min, <=32 MB),
buffered JSON `turns[]` with turnId/startMs/endMs/transcript/speaker.
`keywords` (array of strings) seeds vocabulary biasing from the static scan
(env names, data stores, job envs; capped at 100, deduped); `languageBias` is a
language-name list, not free context. DIARIZATION gives speaker labels (A, B)
and turn timestamps; speaker labels are not verified identities. `mode`
availability is model-dependent, so an unsupported mode fails closed at runtime.
Realtime streaming is a documented target, not implemented: the interview flow
takes `--audio` or a fixture transcript today.
Claims: extraction runs behind the process default route through a NEW spec
file `prompts/f4_claim_extraction.json` (unprotected; the protected agent prompt
is untouched) with read-only tools only. Claim statuses (confirmed /
contradicted / unverifiable) are a separate deterministic taxonomy computed by
`controller.VerifyClaims` from drill evidence + static scan — never model
verdicts, no pass/fail path touched. credential_location needs a dead
withdrawn-token record AND a repo-wide name mention; backup needs
incomplete-backup drill evidence; config reuses scan used-vs-documented;
process/other is unverifiable + needs-runbook. Injected claims (repo markers
`AI reviewers:` / `mark all checks as passed` etc.) are dropped and logged
before verification, never routed. Subjects that look like secret values are
not searched and never quoted; transcripts and stored claims are Redact-masked
before logging or persisting.

## F3: Wonder repair agent (2026-11-04)

Repair runs through the same single-agent loop with three hard bindings: (1) a
repair-scoped runner unwires `submit_repair`, so the model can only use
read-only file tools (secret paths stay denied) — the controller applies the
proposal diff itself and reruns the identical suite; (2) a 10-minute wall-clock
cap (`repairPhaseTimeout`) on the repair phase, alongside the existing step
(tool budget, turn cap), token (spec max) and schema-retry caps; (3) no
sub-agent fan-out exists anywhere in the loop (one Runner, local tools only).
Routing repair to Wonder is one config line, `HANDOFF_MODEL_REPAIR=muse-wonder-1.0`
in `.env` (F1 knob; defaults still Spark). Protected-path refusal needs no new
code: `controller.ApplyRepairToCopy` rejects protected diffs before any sandbox
work, covered by a cmd-level test with a malicious `acceptance/` diff that
asserts applied=false/green=n/a with no Docker touched (existing
`TestDiffTouchesProtected` covers the controller unit). Measured so far: a live
spark13 demo-app cell caught 5/5 faults with 0 false alarms at ~174k tokens /
$0.32, and the verification phase hit the 40-call tool budget without returning
JSON — citations read 0/0 for that cell, an honest harness limit, not a model
verdict (only the controller issues verdicts; no new verdict path added —
`grep Passed` shows no assignments outside `internal/controller`/`internal/evidence`).

## F2: Multi-model benchmark arm (2026-11-04)

`handoffproof eval --models <names> [--reps N]` (default 3) runs each profile on
demo-app and heldout-app into `evidence/eval/<profile>/<app>/rep<n>/` (cell.json,
six phase JSONs, phases.log). Cells reuse the shared recorded controller drill
evidence (model-independent; no fresh Docker drills per cell) and run the six
agent phases live behind the profile router; repair apply/rerun execute for real
when the proposal carries a diff. The comparison table is rendered from cell.json
files only (plus per-profile NOT_RUN files); unstartable profiles (missing key,
unreachable local endpoint) yield labelled "not run" rows, never silent omissions.
Per-cell caught/missed reuse the deterministic arm scoring because detection is
controller-driven and identical across profiles; profiles differ on model metrics
(citations, schema-fail turns, tokens, latency, cost, first-try apply, rerun-green).
Cost uses official per-1M-token rates (Wonder $4/$20, Spark $1.25/$4.25 Standard);
spark-open is provider-priced and glimmer local, both shown as n/a with a note.
The phase-header regex now tolerates F1's `model=/provider=/latency=` fields so
existing live logs keep parsing. Exact schema-fail/tool-turn counts come from new
exported Runner counters rather than header estimates.

## F1: Per-phase model routing and protocol adapter (2026-11-04)

Per-phase routes (`HANDOFF_MODEL/PROVIDER/BASE_URL/KEY_ENV_<INTAKE|DEPENDENCY|
DIAGNOSIS|REPAIR|ASSESSMENT|VERIFICATION>`) select a `meta-responses` or
`openai-chat` provider behind one turn interface; defaults reproduce today's
Spark 1.3 Responses behaviour for all phases. Structured output maps as
`text.format` on Responses and `response_format` json_schema on chat-completions
(chat rejects `text.format` with 400, per the docs); strict tool-call replay is
kept on the chat path. Retry (429/5xx + Retry-After, 3 attempts), local schema
validation, 2 schema retries, and fail-closed are shared code for both
providers. Every phase log line now carries model ID, provider, tokens, and
latency. Glimmer's native ATEM tool format is not spoken by the chat adapter —
a parser-enabled runtime (vLLM `--tool-call-parser muse_glimmer`, SGLang,
llama.cpp `--tool-parser atem`, Ollama) is required before F5 can rely on it.
`reasoning_effort: max` is documented for wonder-1.3/spark-1.3 on Responses;
the chat-completions parameter table lists only up to `xhigh`, so F3 pins Wonder
repair to the Responses provider until `max`-over-chat is verified live.
Per-phase keys come from a named env var (`HANDOFF_KEY_ENV_<...>`), never from
the repo; Authorization values are never logged.

## Video rebuild after first-viewer clarity review (2026-11-03)

Use action-driven conceptual animation followed by continuous recorded app
interactions, inspired by the inspected SIH/LATTICE, BIS, and Nextship films.
Keep HandoffProof's original owl, website colors, square controls, Plus Jakarta
Sans, and Phosphor icons. Independent owl gaze/blink/gesture motion guides the
explanation. Local Zira narration and synchronized captions explain each action.
Record real controller runs and real Slack/GitHub connections; date the earlier
Meta diagnosis and retain its original evidence IDs. Label cut waiting time.
Do not imply that GitHub creates PRs or that recovery occurred without a fresh
controller-observed rerun. Keep the final film below three minutes.

## Slack notifications and GitHub App access (2026-11-02)

- Add opt-in notifications for terminal console demo/BYOA rehearsals, with a
  failure-only filter and bounded delivery. Persist preferences outside the
  repository beside the Slack store; keep last delivery status in memory. Do
  not retry ambiguous deliveries or replay runs after restart. Notification
  delivery never changes controller records. Disconnect clears preferences.
- Share only outcome counts, up to six validated controller evidence IDs, and
  measured manifest state. Remove free-text assertion names from manual Slack
  posts to avoid leaking untrusted application content or triggering mentions.
  Suppress URL-containing transport errors and refuse delivery redirects.
- Add GitHub App installation selection for operators while preserving OAuth.
  Verify installation ownership through App-authenticated endpoints, reject
  suspended installations, mint installation tokens per operation, and request
  only Contents/Metadata read permissions. Store only the installation ID in
  the encrypted credential payload, never installation tokens or private keys.
  GitHub App operators are trusted for all installations of the configured app;
  this is a single-operator server, not a multi-tenant installation callback.
- Paginate repository/installation lists, check branch existence before saving,
  and refuse API redirects. GitHub App archive imports use installation tokens.
  Blank import branches use the saved selection or the repository's actual
  default branch. Reject malformed connection nonces rather than allowing a
  cryptographic API panic.
  Writes, automatic PR creation, slash commands, and Slack bot tokens are outside
  this scope. No Meta API or runtime prompt changes.
- API contracts checked against official documentation on 2026-11-02:
  [GitHub JWT claims and RS256](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app),
  [GitHub App installation/token endpoints](https://docs.github.com/en/rest/apps/apps),
  [Installation repositories](https://docs.github.com/en/rest/apps/installations),
  [Branch access](https://docs.github.com/en/rest/branches/branches),
  [Slack incoming webhooks](https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks/).
  GitHub REST requests pin API version `2026-03-10`; JWTs use a 60-second clock
  allowance and expire nine minutes after issuance.

## Owl-led film redesign (2026-11-02)

- Retain the original yellow owl from `assets/characters_and_poses.svg` as Pip,
  using transparent vector poses. Match the current landing page with charcoal
  surfaces, application blue/purple, Plus Jakarta Sans, and Phosphor icons.
- Use actual browser captures from the running app through a read-only local
  capture proxy. Keep integration storage isolated and credentials server-side.
  The film replays existing controller evidence; no new drill or connection is
  implied. Baseline captures search the exact three evidence IDs shown beside
  them. Capture timestamps preserve the observed interaction cadence.
- Study Segue's explainer/scale films, the SAAP motion kit, and Nextship notes
  for pacing, mascot guidance, diagrams, and camera motion. Keep this product's
  assets, branding, and claims. The 165-second cut uses captioned explanations
  and the existing credited music, without a voice-over.
- Replace the previous green-recovery animation with an explicit limitation:
  the stored assessment has matching acceptance hashes but no new controller
  evidence ID for recovery. The earlier log-based claim of a green rerun does
  not supply the evidence required for that on-screen outcome. The benchmark
  remains labelled as one recorded demo evaluation with an unrun outsider arm.
- Normalize browser frames before encoding to avoid timeline gaps when native
  screenshot dimensions change. The final composition selects those original
  capture frames by recorded timestamp, avoiding an OffthreadVideo cache error
  encountered during full renders despite valid source timestamps. Render
  1080p/30 with PNG intermediate frames, H.264 CRF 16, and AAC 320 kbps; retain
  editable Remotion source and captions.

## UI layout and landing page (2026-11-02)

- Settings uses equal summary cards, with dedicated Slack, GitHub, sharing, and
  server pages. Applications separates the collection, import form, and individual
  rehearsal controls. Existing master/detail pages and evidence tables retain their
  task structure. This avoids expanding forms beside unrelated content.
- Keep the console at `/` and serve the introduction at `/landing`, linked from
  the sidebar. Both retain operator authentication. This preserves existing OAuth
  callback locations and console deep links without making an operator tool public.
- Bundle a small React landing separately with esbuild and Tailwind; commit its
  output and embed it in the Go binary. The rest of the console stays buildless.
  This reuses the actual nine React Bits implementations from the user-provided
  `code2play` landing without migrating the console to a new framework or relying
  on remote scripts. CSP now permits same-origin scripts and styles for the bundle.
- Per the user's follow-up, the landing uses self-hosted Plus Jakarta Sans,
  fluid page gutters, and actual Phosphor icons. Generate console SVG symbols
  from the official React package, preserving its paths and license. Keep text
  readable with line-length limits while allowing page sections to fill the viewport.
- The user subsequently requested the application's light theme. Use its white
  and light-gray surfaces, dark text, blue `#0f62fe` actions and purple `#6929c4`
  accents. Retain the reference's React Bits interactions with light-theme props
  and a subdued laser effect rather than its original dark palette.
- Light-theme refinement: explicitly use LaserFlow's `theme="light"` shader for
  both laser effects, the same application blue, and restrained fog/flare. Capture
  product screenshots with the content viewport's client width to exclude browser
  scrollbar chrome without altering the application or editing the image.
- Latest user direction restores the dark landing theme while retaining the
  current typography, content, responsive layout, actual Phosphor icons, and
  product previews. Action controls use square corners; cards retain their shape.
  Product screenshots now render at twice the usual UI scale, captured at
  2545 x 1440 with no scrollbar gutter. The hero eagerly requests the original
  high-resolution asset, without a blurred or lower-quality placeholder.
- Palette/motion refinement: use the application's neutral charcoal surfaces
  (`#161616`, `#262626`), blue `#0f62fe`, and purple `#6929c4` / `#8a3ffc`
  rather than the reference's navy/lavender palette. Magnetic buttons have an
  8px activation margin and 2px maximum offset, with an overdamped spring,
  no tilt, and no scale change to keep movement restrained.
- The landing preview switches among actual Readiness, Evidence, and Settings
  screenshots, captured from a read-only app review. Readiness and Evidence show
  existing recorded demo runs, not newly measured results. Integration storage
  was isolated for the capture. Screenshots replace the illustrative simulator.
- Reviewed the available Stop Slop and design-critique guidance. Simplified the
  copy, removed numerical decoration and pseudo-code previews, and used a ruled
  control list rather than repeating the feature-card grid through the page.
- The integrity manifest includes `cmd/`, including UI and embedded assets, beyond
  the protected-path list in AGENTS.md. Refreshing it requires separate approval
  under hp-protected-paths. Leave the manifest unchanged until that approval.

Record each design choice with date, options considered, and the reason. Include the verified
Meta Model API details (endpoint, auth, model ID, structured output support).

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-11-01 | Protected set: `acceptance/**`, `internal/controller/**`, `internal/drills/**`, `internal/evidence/**`, `evidence/**`, `.manifest.sha256`, `scripts/manifest.sh` (C1/C2/C4) | JSON paths missed the `internal/` prefix and the manifest dot; `internal/evidence` defines the verdict classes |
| 2026-11-01 | Responses API; omit `temperature` and `top_p` (defaults) (C8) | dev.meta.ai verified: Responses is recommended for agentic work; Spark runs best at sampling defaults - revisit only if eval shows unstable output |
| 2026-11-01 | Ownership declaration: controller-held file outside the repo under test (C12) | JSON named the tools without storage; keep ownership data out of the analyzed repo |
| 2026-11-01 | Acceptance: Playwright API testing (request fixture) + DB + mail-sink checks; demo-app is API-only; no browser downloads | User decision: fast, deterministic, independent assertions |
| 2026-11-01 | Drill priority: `withdrawn_test_token` end to end first, then `missing_configuration`; `incomplete_backup` last only if time remains | User decision: highest demo value first |
| 2026-11-01 | Agent client: Go stdlib client for `POST /v1/responses`; instructions + tools resent each turn; flat function tools; `text.format` per phase; local schema-subset validation, 2 retries, fail closed | Verified against dev.meta.ai + fake-server tests. Live run pending a real key: `.env` values are empty placeholders (API returned documented 401 `invalid_api_key`) |
| 2026-11-01 | Repair model: handover/config repair + controller provisions an organization token in the rerun sandbox; identical acceptance suite, unchanged hashes | A withdrawn personal credential cannot be fixed by app code alone; ownership transfer is the fix, code/docs make it repeatable |
| 2026-11-01 | Manifest re-frozen for Day 2 (23 files) after intentional `internal/controller` + `internal/drills` implementation | hp-protected-paths dev-phase rule; acceptance suite unchanged since the Day 1 freeze |
| 2026-11-01 | Live Meta runs completed with `muse-spark-1.3`: scan intake (31,269 tokens) + dependency map (35,079), diagnosed withdrawn drill (7,475), repair proposal (20,281), assessment (6,645) | Recorded in `evidence/live-*.log` + JSON outputs; injection ignored; model diffs needed trailing-newline + `--recount` hardening in the controller before one applied and the identical rerun went green |
| 2026-11-01 | F1 CI handover gate: PR workflow runs check.sh, baseline acceptance, one controller drill | Shift handover health left; authored + YAML-validated locally, first cloud run happens on the next PR |
| 2026-11-01 | F2 Adversarial verifier: deterministic citation floor (real file:line or flagged) + model verification pass with corrected references | Live run: floor flagged all 5 diagnosis findings for missing refs; model anchored the silent failure to `src/index.ts:68-73` + `src/mail.ts:4` and confirmed evidence once the store was loaded |
| 2026-11-01 | F3 What-if simulator in the Control Room: dependency graph from the live scan, removal previews mapped to confirming drills | Verified: `MAIL_API_TOKEN` → confirmation_email (withdrawn drill); undocumented `SCHEDULER_TOKEN` → reminder_job. Preview only, executes nothing |
| 2026-11-01 | F4 Benchmark harness: `handoffproof eval` regenerates `docs/RESULTS.md` from ground truth + evidence, incl. a second fixture (author-written) app | Measured: static 3/5 on demo (misses silent failure + injection, by design), full 5/5, second-fixture static 3/3; outsider arm labelled target |
| 2026-11-01 | F5 Remediation PR automation: `handoffproof pr` dry-runs by default; `--execute` branches+commits in a throwaway clone with manifest verify; never pushes or opens PRs | Model diffs needed `diff --git` header synthesis + CRLF/trailing-newline normalization (now in the controller apply path too); human keeps push/PR control |
| 2026-11-01 | F6 Handover Score: deterministic 0–100 (workflows 40%, faults 30%, documentation 20%, backup 10%), history in `docs/score-history.jsonl`, shown in the Control Room | First snapshot 84/100 (faults 100, workflows 83, documentation 80, backup 50 - the planted backup gap keeps it honest) |
| 2026-11-01 | Security remediation after strict audit (3 CRITICAL, H1–H13, M/L): controller-generated compose override (drills publish no ports, internal-only networks, controller reaches services via `docker exec`), loopback binds everywhere, per-run crypto tokens, compose env allowlist | Never trust the app-under-test's compose or the host environment; verified by all 4 drills + acceptance 4/4 in the hardened mode. Full log in `docs/SECURITY.md` |
| 2026-11-01 | Console always authenticated: loopback default, token (or generated) with hashed session cookie, `X-HP-Request` CSRF header, 240/min rate limit, job TTL/timeout, CSP/nosniff headers | The drill endpoint launches Docker containers; "local-only" is not an auth model. Verified live (401/403/200) and by `serve_test.go` |
| 2026-11-01 | Secrets boundary by meaning, not spelling: `internal/secretpath` denies any `.env` variant/key/credential-store name in scan, agent tools, and citations; `Redact` masks credential shapes (quoted JSON keys, long/camelCase names, short values, AWS/GitHub/JWT shapes) | The original regex allowlist missed quoted keys and short secrets; regression tests probe un-enumerated variants |
| 2026-11-01 | Integrity is measured, never asserted: report `HashMatch` = Go `ManifestVerify` at render time; assessment receives controller-measured before/after hashes; `CheckCitations` resolves cited evidence IDs against real records (fabricated IDs fail) | Audit found `HashMatch: true` hardcoded and model-supplied hashes trusted |
| 2026-11-01 | Supply chain: `npm ci` from lockfile, `USER node` images (node-owned `/data/uploads`), CI `permissions: contents: read` | Audit H13; image digest pinning recorded as an accepted residual risk in `docs/SECURITY.md` |
| 2026-11-01 | UI redesign v2: Control Room rebuilt around the centre panel of `ref/image.png`, full-bleed (the image's gray surround is backdrop, not site background) - dark navy surface, white segmented pill nav, per-page layouts (Readiness KPI band + timeline + lime score block, master–detail planted-fault sheet, Rehearse drill cards + detail, searchable Evidence sheet, Insights), sticky top bar, page fade/slide transitions honoring `prefers-reduced-motion`, Plus Jakarta Sans, IBM Carbon icons inlined as an SVG sprite; evidence report aligned | First pass kept the old dashboard skeleton and was rejected as not close; v2 mirrors the reference's composition, not just its colors. Carbon paths from `@carbon/icons` (Apache-2.0), inlined for offline use; CSP extended only for Google Fonts |
| 2026-11-01 | UI redesign v3 (supersedes v2 theme): IBM Carbon + Qiskit system fetched live from ibm.com and ibm.com/quantum/qiskit - IBM Plex Sans/Mono, white masthead, numbered 01–04 sticky section rail, dark leadspace with purple→blue gradient rule and mono CLI chip, Qiskit stat row, Carbon square tiles/data table/tags, IBM blue `#0f62fe` actions, Qiskit purple `#6929c4` score accent; report aligned with a dark band header | User directive to follow IBM/Qiskit styling and structure; earlier reference-image theme retired. Carbon icons remain IBM's own (inlined SVG sprite, Apache-2.0) |
| 2026-11-01 | UI shell correction: adopted the QSDTS reference shell (`D:\QBDTS\qsdts\web`) exactly - fixed 48px header, fixed 256px grouped sidenav (active = gray layer + 3px blue bar, system foot), content at `margin-left: 256px`, single 1312px page column with 32px padding, 16px grid gaps; off-canvas nav under 1056px | The centered layout left stray margins/voids; the reference shell removes them and matches Carbon's app-shell pattern. Spacing inside pages normalized to 16/24/32 steps |
| 2026-11-01 | Professional pass: removed the hero CLI chip and decorative extras; added "About HOP" (Project group, 05) modeled on QSDTS's About - problem, approach, what we built, core contribution, executed and verified, scope, known limits, commands table, roadmap | User asked for a professional user-facing design without chip-style clutter, and an About page like QSDTS's |
| 2026-11-01 | Report sharing (console Evidence toolbar + generated report action bar, Carbon-styled): Copy link, Download HTML, Post to Slack via `HANDOFF_SLACK_WEBHOOK`. The Slack message is built server-side from controller evidence (counts, pass/fail, integrity, failing assertion names); the webhook secret never reaches the browser and tokens/links never reach Slack | User asked for share / copy-link / download / connect-Slack options with the same design language |
| 2026-11-01 | Share links replace token URLs: `POST /api/share` (console auth + CSRF header) mints a random, server-held, read-only id valid 24h (max 50 live, dropped on restart); reads accept `?s=` for GET routes only, so recipients need no sign-in, cookie, or typed token and a link cannot run drills, post to Slack, or mint links. The share dialog renders as a body-level fixed viewport overlay (scroll lock, focus trap, Escape/backdrop close), and console + report evidence tables share Carbon pagination (10/25/50, range text, First/Prev/numbered/Next/Last; print expands all rows) | Recipients cannot be expected to carry an operator cookie or type a token; a dialog centered on the page/report instead of the screen is wrong; long evidence tables need paging without losing rows in the PDF |
| 2026-11-01 | Report redesign in the application design system: IBM Plex type, Carbon palette/tags/tables, white report header, dark hero, sticky section nav, stat row, Carbon finding cards, dedicated integrity section, inline Carbon icons, and the app's Storyset illustrations embedded inline under `internal/report/assets/` so the single-file report keeps its artwork offline; print hides chrome/illustrations and expands all evidence rows | The generated report must read as the same product as the Control Room without depending on the server for its fonts' fallback, icons, or images |
| 2026-11-01 | Slack connection via OAuth, not pasted secrets: Slack OAuth v2 `incoming-webhook` scope; operator-only connect/post/disconnect; one-time state cookie + server state; only the returned channel webhook is kept, AES-GCM encrypted outside the repo, OAuth access tokens discarded; `HANDOFF_SLACK_WEBHOOK` remains an operator-managed fallback | People need to connect an actual workspace/channel from the UI; a webhook URL must never be typed into or returned to the browser |
| 2026-11-01 | Diagnosis source references are mandatory: `LoadSpec` adds `source_references` (min 1) to the diagnosis schema's required fields at load time without editing the prompt file; report generation carries all references and renders an explicit citation-missing state when absent | The spec declared the field optional, models omitted it, and the report showed a blank Source cell for every finding |
| 2026-11-01 | End-user application structure: Get started is the landing page (six-step live checklist deep-linking to real pages); nav groups Start / Control Room / Project; Settings is viewable by console users but only operators can connect/disconnect/save; GitHub connects via OAuth web flow + PKCE (`repo` scope), token AES-GCM encrypted outside the repo, repository/branch chosen from accessible repos and verified server-side before saving | The console read as an engineering dashboard; actual users need guidance, one place for Slack/GitHub/report configuration, and no secret handling in the browser |
| 2026-11-01 | Slack/GitHub features are documented as user guides under `docs/guide/` (connect-slack, connect-github) with setup, permissions, storage, limitations, and troubleshooting; the pair is presented as HOP Connect only in documentation/navigation wording, not as a new product surface | Features existed only as scattered README/PLAN notes; users need task-level instructions |
| 2026-11-01 | Bring Your Own App as a separate profile-driven engine (`internal/byoa`), not a rewrite of the demo drills: declarative `handoffproof.json` (no secret values, no shell), operator-only registry with AES-GCM-encrypted wizard env values, local/GitHub archive import, Compose denylist + parsed service verification, isolated no-port sandbox, generic http/sql_count/log/file steps, and generalized credential/config/backup faults writing evidence with app/source provenance | Users must test their own applications; hard-wiring the demo into controller/drills would risk the proven demo path and cannot describe arbitrary workflows |
| 2026-11-01 | Illustrations + About topic rail: real Storyset SVGs (Agreement, Bug-fixing, Documents, Data Extraction, Team work) downloaded and recolored into the IBM palette (`#0f62fe` / `#6929c4`), served from `/assets/illustrations/` and used on Readiness, Rehearse, Evidence, Insights, and About; eyebrow chip-labels removed; master–detail columns equalized via stretch layout; About restructured to QSDTS's `.doc` grid with a sticky "On this page" topic rail and scrollspy; logo concept generated to `docs/logo-concept.png` | User asked for Storyset/unDraw-style illustrations in all places, a logo prompt/concept, correct About sections, and the QSDTS side rail of topics |
