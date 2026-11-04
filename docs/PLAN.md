# HandoffProof - 3-day build plan (Step 1)

## Standalone landing deployment (2026-11-04, user requested)

Prepare a static-only Vercel build rooted at landing, package all public assets,
and route standalone calls to the demo, repository and recorded validation.
Preserve the embedded Go build, protected files, model results and local video.
Validate typecheck, static build and the project gate before publishing setup.

## Final stabilization (2026-11-04, user-approved autonomous mission)

No new features. Audit baseline and security, bound Compose subprocesses, diagnose saved
benchmark artifacts, disclose harness and model failures, align documentation, commit
in structured groups, publish main and v1.0.2, verify CI and a fresh clone.
No live model calls. Windows/Docker failures must remain explicit.

Current validation (2026-11-04): the fresh Ubuntu WSL2 checkout passed the full
quality gate, Go vet, all 13 Go packages with zero skipped tests, and the controller
baseline for registration, booking and confirmation email. The isolated Docker
fixture passed in 13.84 seconds. See [the recorded validation](VALIDATION.md) for
the tested revision, commands and controller evidence IDs. The original Windows
network stall's single root cause remains unconfirmed.

Documentation follow-up, approved by the user: publish the WSL validation record
and its controller evidence sample, add reproducible WSL checks to the README,
replace the demo placeholder with the supplied hosted link, run the quality gate,
and commit and push only the documentation paths. Keep model benchmarks unchanged.

The dated build notes below are historical, including earlier Windows policy blocks
and pending approvals. They do not describe the current test or commit status.

## README refresh (2026-11-04, implemented; documentation checks complete)

Scope: update README.md against the current implementation and existing guides.
Preserve unrelated local edits, protected paths, the runtime agent prompt,
recorded results, and both video-production trees. Use plain professional wording
without em dashes. No new API/model calls or new runtime-result claims.

Audit findings confirmed in source:

- Exit interviews, local/hybrid private mode, profile auto-drafting, and
  experimental whiteboard claim checking appear only in the limitations tail;
  the main feature overview and user workflows omit them.
- CLI table omits `interview`, `draft`, and `whiteboard`. It also describes
  `repair` incorrectly: this command takes a diff and applies/reruns it;
  model repair proposals are part of `drill --full`.
- The claim that every model call uses one Meta Responses endpoint is false
  for per-phase provider routing, local/hybrid mode, and audio/image operations.
- Auto-drafting is a CLI workflow; the wizard loads a previously accepted
  candidate. It does not initiate a model drafting run from the load button.
- Prerequisites omit the go.mod version, Compose v2, Git Bash/bash, and Node
  requirements for the demo/acceptance workflow. Static scan can be introduced
  separately from cloud-model setup.
- The modified local demo is 2:55.02, while the tracked remote master is 2:30.02.
  Local videos/out/film/v5 is another 2:58 cut. `film/` and `videos/` are ignored,
  so links into them will not work for a fresh GitHub visitor. Do not overwrite
  the already-modified demo master or claim that v5 is the linked public asset.
- Documentation table is broken by a paragraph inserted between its rows.
  Model role/status text is duplicated, the layout omits newer packages, and
  broad "implemented and verified" wording hides measured feature limits.

Implementation plan:

1. Rewrite the introduction, feature overview and model-role explanation around
   the current product: controller-owned outcomes; explicit cloud/default,
   per-phase, local and hybrid routes; interview transcription; draft generation;
   experimental image claims. Keep build-time assistants separate from runtime.
2. Add concise practical workflows for exit interviews, loading CLI-generated
   profile candidates, local/hybrid configuration, and experimental whiteboard
   input. Show only flags and environment names confirmed in source; explain
   which actions are CLI-only and which results appear in the console.
3. Correct quick start, CLI reference, architecture/layout, integration details,
   demo metadata/public links, documentation navigation, and build/team credits.
   Keep the linked demo master unchanged; distinguish it from local video cuts.
4. Consolidate status and limitations next to relevant features. Link measured
   results instead of copying volatile totals or claiming new verification.
   State auto-draft, local-runtime, synthetic-whiteboard, and BYOA limits plainly.
5. Validate relative links and section anchors; check command/flag/env examples
   against implementation without invoking models or changing evidence. Run
   `bash scripts/check.sh`, report actual results (including any existing Docker
   fixture timeout), and verify protected-file integrity. Record any substantive
   documentation decisions in docs/DECISIONS.md.

README rewrite complete. Validated all 46 Markdown links, relative targets in
tracked files, section anchors, and coverage of all 12 implemented CLI commands.
No em dashes in the README. Isolated checkout skill and protected-manifest checks
passed. Full `bash scripts/check.sh` completed gofmt/vet, then exited 1 because
`TestRunnerPythonFixture` timed out after 10 minutes during Docker Compose startup;
other listed Go packages passed. A no-credentials `go run ... scan` attempt was
blocked by Windows Application Control before execution, not reported as passed.
Commit and push explicitly authorized. Publish this task's documentation only.

## Reference motion + sound refinement (2026-11-04, v5 exported; Docker fixture timeout)

Read-only analysis completed: reference decoded at 30 fps into timestamped
consecutive-frame sheets under `videos/out/review/reference-deep/`. Audio has
657,532 decoded signed-16-bit channel samples, all zero: there is no reference
effect to extract. Detailed motion timings: `videos/REFERENCE-ANALYSIS.md`.

1. Replace decorative repeated pops, taps, bleeps and word-by-word hits with a
   sparse sound design: short soft air sweeps following horizontal phrase travel
   and zoom transitions, a light air puff on Tony's smoke exit, and a restrained
   final settle. Leave holds quiet; reserve quiet mouse clicks for visible UI
   actions. Choose/trim actual suitable effects and verify their source/license.
2. Rework kinetic choreography to match the reference's distinct stages:
   entrance scale change, short hold, phrase-level horizontal track, emphasis
   push-in, and an intentional reset into the next phrase. Use clean brief blur
   during fast movement. Avoid shrinking every accumulating sentence to fit.
   Keep final reading holds fully readable and naturally spaced.
3. Preserve the approved bright multipoint logo scenes, white closing card,
   three-character opening to the outro, Tony's smoke, and centered final pair.
   Refine transition and lockup timings so sound follows the actual movement.
4. Render short opening/punch/outro previews with new sound first, inspect
   consecutive frames and cue timings, then export v5 retaining v4. Run video
   typecheck and required repository checks; report actual results and any
   Docker-test timeout. Validate final audio/video duration and full-file decode.

Implementation and export completed: `videos/out/film/v5/handoffproof-film.mp4`
(178 seconds, 1080p/30 fps). Details: `videos/REVIEW-v5.md`. Typecheck,
skill formatting, protected manifest, and full movie decode succeeded. Full
repository check exited 1 on the existing Docker fixture's 10-minute timeout.

## Video corrections (2026-11-04, video exported; repository gate has Docker fixture timeout)

Scope: `videos/`, using `videos/out/film/v3/handoffproof-film.mp4` as the current
render. Preserve existing unrelated edits in `film/`, `demo/`, and elsewhere.
Reference: user-supplied 7.45-second clip, sampled every 0.1 seconds across the
clip (75 samples); inspect consecutive frames around entrances during editing.

1. Rebuild shared kinetic typography to follow the reference's word-by-word
   composition, smooth horizontal tracking, scale changes, and emphasis holds.
   Replace character-count width estimates with actual loaded-font measurements;
   use consistent natural word spacing. Apply to the introduction, title text,
   punch scenes, and closing tagline; audit other animated text throughout.
2. Remove every concentric ripple use, including buttons, logo stages, feature
   accents, and the closing HOP badge. Substitute a restrained one-time accent
   sweep with a short lift/settle and compact soft shadow; no expanding rings.
3. Give the dedicated Docker, Meta, GitHub, Slack and two-integrations logo
   scenes bright moving multipoint gradients, white animated marks, and contrasting light
   text. This request applies to the user-supplied scene screenshots; the
   closing card keeps its white background.
4. Rebuild the closing composition around a centered three-character group.
   All three appear first; Tony glances back, does a playful anticipation hop with soft smoke puffs,
   and exits. The controller and model ease into a centered pair. Center the
   title/HOP lockup, naturally spaced tagline, repository link, technology
   badges, and footer with a stable final reading hold.
5. Source a lighter instrumental music bed from a free-to-use music library,
   verify the track's actual license, and record source/credit requirements.
   Favor gentle piano/plucks, soft rhythm, no vocals or heavy bass. Mix quietly
   beneath narration, with smooth fades and speech ducking; review transition
   effects for harshness as part of the new mix.
6. Validate each implementation step with `bash scripts/check.sh` and the video
   project's typecheck. Render short motion previews and closely spaced frame
   sheets for text entrances and Tony's exit; inspect all ripple locations and
   text spacing across the film. Export to a new `videos/out/film/v4/` directory,
   retaining v3. Review the full export for timing, narration clarity, audio
   levels, readability, and duration under three minutes. Record actual checks
   and design decisions; make no new product/evidence claims.

Completed export: `videos/out/film/v4/handoffproof-film.mp4` (178 seconds,
1080p/30 fps). See `videos/REVIEW-v4.md` for actual validation and the existing
Docker fixture timeout from the full repository check. Protected manifest OK;
runtime agent prompt and protected paths unchanged.

## F6 + F7 (2026-11-04, code DONE — see RESULTS.md for measured numbers)

F6 BYOA auto-drafter: `internal/draft/` (new, unprotected, +test),
`prompts/f6_profile_draft.json` (new), `cmd/handoffproof/draft.go` (+test,
`/api/drafts/latest`), wizard button in `cmd/handoffproof/web/index.html`,
`.env.example`. Measured: `handoffproof draft` on miniapp and demo-app, 3 reps each,
rendered by `draft --render`. F7 whiteboard (experimental):
`internal/agent/media.go` (+test; image gen, SAM 3.1, image-input extraction),
`internal/whiteboard/` (new, +test), `prompts/f7_image_claims.json` (new),
`cmd/handoffproof/whiteboard.go` (+test), fixture
`eval/fixtures/whiteboard-synthetic.{png,label.json}`. Live run:
`evidence/whiteboard-20261103-184027.json`. Protected changes awaiting manifest
approval: `internal/agent/media*.go`, `cmd/handoffproof/*`, `prompts/*`.
Open decision: claim polarity in controller config/backup verification.

## F5: Private mode with Muse Glimmer (2026-11-03, code DONE — quality vs Spark not measured)

`HANDOFF_PRIVATE_MODE=local|hybrid` (default off) routes source-reading phases
to a loopback Glimmer endpoint via F1. Local mode guards the HTTP client
(loopback only) and refuses cloud transcription; hybrid denies source tools to
cloud-routed phases. Files: `internal/privatemode/` (new, unprotected, +test),
`internal/agent/routing.go` (IsLoopbackURL/Host), `internal/agent/tools.go`
(DenySource), `internal/agent/loop.go` (Mode, SourceLocalOnly, per-phase deny
copy), `internal/agent/private_test.go` (new), `cmd/handoffproof/main.go`
(wiring, `mode=` in the log line, repair runner keeps the guard),
`cmd/handoffproof/interview.go`, `cmd/handoffproof/serve.go` (`/api/private-mode`),
`cmd/handoffproof/web/index.html` (badge), `cmd/handoffproof/privatemode_test.go`
(new), `.env.example`. Egress acceptance: `TestLocalModeNoEgressToMetaAPI`.
Protected (awaiting manifest-refresh approval): `internal/agent/*`, `cmd/handoffproof/*`.
Target, not result: Glimmer quality row in the F2 table (no local runtime here).
F6/F7: not started (F7 is gated on F5 being measured, which it is not).

## F4: Exit Interview Verifier (2026-11-04, DONE 2026-11-03 — cmd suite now executes and passes)

Update 2026-11-03: Application Control no longer blocks the test binaries. The
first real cmd run found a broken fake-server envelope in
`TestInterviewOffline` (stray quote, `usage` outside the object); fixed in the
test only. `go test ./cmd/handoffproof/` now passes, including
TestInterviewOffline (C1/C2 confirmed, C3 contradicted, C5 unverifiable +
needs runbook, injected C4 dropped), TestInterviewLatest*, and the F2/F3 tests.


`handoffproof interview --audio h.wav [--transcript fixture.json]` transcribes
(vocabulary biased from the scan), extracts claims via
`prompts/f4_claim_extraction.json`, drops injected claims with a log line,
verifies deterministically, persists a redacted record to
`evidence/interview-*.json`, prints a panel. Files:
`internal/agent/transcribe.go` (+test), `prompts/f4_claim_extraction.json`
(new, unprotected), `internal/controller/claims.go` (protected),
`cmd/handoffproof/interview.go` (+test), `eval/fixtures/interview-transcript.json`
+ `interview-sample.wav`, Control Room claims panel (`serve.go`, `web/index.html`),
HTML report interview section (`internal/report/report.go` +test). Verified by
execution: report suite green (incl. TestRenderInterviewSection), agent suite
green (incl. TestTranscribeFileDiarizedTurns/NoRetryOn400). Blocked: cmd suite
(interview e2e, endpoint, F2/F3 tests) cannot execute — Windows Application
Control denies all fresh test binaries; will rerun in an allow window. Then
STOP for protected-path approval (cmd, controller, agent, report, prompts).
Live benchmark for F2 complete (12/12 cells: spark13 6/6, wonder 6/6;
spark-open/glimmer labelled not run with reasons); table regenerated via
`eval --models ... --render-only` (renders from cell.json files only).

## F3: Wonder repair agent (2026-11-04, implemented — with F2 below)

`HANDOFF_MODEL_REPAIR=muse-wonder-1.0` in `.env` routes repair_proposal to
Wonder via the F1 knob (config, not code). Repair runs on a scoped runner
(`submit_repair` unwired → read-only file tools) under a 10-minute wall-clock
cap, with steps/tokens/retries already capped by the loop and no sub-agent
fan-out. Files: `cmd/handoffproof/main.go` (scoped runner + timeout in
`runRepairLoop`), `cmd/handoffproof/eval_models.go` (same for cells),
`cmd/handoffproof/eval_models_test.go` (read-only + protected-refusal tests),
`.env` (gitignored local routing; defaults unchanged). No protected-path edits;
no new verdict path. Wonder-vs-Spark repair metrics come from the F2 table's
wonder/spark13 rows (historical cells; mixed-evidence and repair-workspace limitations apply).

## F2: Multi-model benchmark arm (2026-11-04, code complete; historical benchmark recorded with harness limitations)

`handoffproof eval --models spark13,wonder,spark-open,glimmer [--reps 3]` runs
each profile on demo-app + heldout-app, 3 reps per cell, into
`evidence/eval/<profile>/<app>/rep<n>/` (cell.json + phase JSONs + phases.log).
Cells reuse recorded controller drill evidence (model-independent) and run the
six agent phases live behind the profile router; repair apply/rerun execute for
real when a diff is proposed. The comparison table is generated from cell.json
files only; a profile that cannot start (missing key / unreachable local
endpoint) yields a labelled "not run" row. Files: `cmd/handoffproof/eval_models.go`
(new), `cmd/handoffproof/eval_models_test.go` (new), `cmd/handoffproof/eval.go`
(flags + phase-header regex now tolerates F1's `model=/provider=/latency=` fields),
`internal/agent/loop.go` (exact SchemaRetries/ToolTurns counters), regenerated
`docs/RESULTS.md`. cmd/ is manifest-hashed: refresh + separate commit after.
Assumption: per-cell caught/missed reuse the deterministic arm scoring (detection
is controller-driven and identical across profiles); profiles differ on model
metrics (citations, retries, tokens, latency, cost, apply/green rates).

## F1: Per-phase model routing (2026-11-04, DONE — commits a1065f2 + 1d536f5)

Implementation complete and green (see the F1 row in docs/DECISIONS.md).
STOPPED before any manifest refresh/verify: awaiting explicit approval because
`internal/agent/*` and `cmd/handoffproof/main.go` are manifest-hashed. Nothing
committed yet. `bash scripts/check.sh` cannot run end-to-end in this shell
(pre-existing Git Bash socket error); its Go gates were run directly and pass:
gofmt clean, `go vet` clean, `go build ./...` ok, `go test` ok for
`internal/agent`, `cmd/handoffproof`, `internal/scan`, `internal/evidence`,
`internal/report`. Next after approval: refresh manifest (separate commit), then F2.

Goal: route each of the six agent phases to a configured model/provider
without changing default behaviour (Spark 1.3 via Responses for all phases).
Files to touch: `internal/agent/routing.go` (new), `internal/agent/provider.go`
(new), `internal/agent/loop.go`, `internal/agent/spec.go`, `internal/agent/client.go`,
`internal/agent/routing_test.go` (new), `cmd/handoffproof/main.go` (router wiring
+ log line), `.env.example`, `docs/DECISIONS.md`. Protected (needs manifest-refresh
approval after the change): `internal/agent/*`, plus `cmd/handoffproof/main.go`
which `scripts/manifest.sh` also hashes. No changes to `acceptance/`,
`internal/controller/`, `internal/drills/`, `internal/evidence/`, `evidence/`,
prompts, or `scripts/manifest.sh`. Assumption: per-phase env names
`HANDOFF_MODEL_<INTAKE|DEPENDENCY|DIAGNOSIS|REPAIR|ASSESSMENT|VERIFICATION>`
(model ID), `HANDOFF_PROVIDER_<...>` (`meta-responses`|`openai-chat`),
`HANDOFF_BASE_URL_<...>`, `HANDOFF_KEY_ENV_<...>` (name of env var holding the key).

## Video rebuild after clarity review (2026-11-03)

Status: Full v4 exported and validated on 2026-11-03. The 175-second master,
poster, 21 caption cues, script, and measured QA report are saved under demo/ and docs/.
Full video/audio decode passed; 13 extracted frames were reviewed. TypeScript,
skill checks, and rehearsal integrity passed. The complete project gate remains
limited by Windows Application Control blocking two test executables.
The user rejected the prior cut's static screen holds
and unclear explanation. This supersedes the completed 2026-11-02 video plan.

Reference research covers other projects only. A D-drive inventory and sampled
rendered sequences were inspected; detailed findings and the proposed 2:55
storyboard are in `docs/VIDEO_REBUILD.md`. The SIH/LATTICE film is the primary
reference for explanation, mascot direction, recorded interactions, and camera
focus. BIS supplies additional character/action timing; Nextship supplies
progressive information reveals. Keep HandoffProof's dark application palette,
original owl, Plus Jakarta Sans, Phosphor icons, and square controls.

1. Build a narrated, action-driven story: customer books, developer-owned email
   access disappears, confirmation stops, HandoffProof rehearses and exposes it.
   Animate the dependency and the owl's attention/gestures, then transition into
   a continuous real browser demonstration. First review a 20–30 second motion
   sample before paying for the full render.
2. Record the current application and actual integration workflows: connect
   GitHub, select repository/branch, import a supported application; connect
   Slack to the designated demo channel, enable completion/failure-only delivery,
   show a real notification and delivery status. Inspect exact authorization
   screens before any new access grant; keep credentials outside recordings.
3. Capture an actual rehearsal and explain its observed result, Meta diagnosis,
   source reference, and handover action in plain language. Respect manifest
   verification before/after drills. Resolve any manifest mismatch through the
   separately required approval, never bypass it for filming.
4. Compose under 3 minutes with continuous captured actions, measured click
   timing, deliberate camera targets, concise narration/captions, and sound
   accents. Preserve the previous master until the replacement is reviewed.
5. Check first-viewer comprehension, UI/result legibility, integration claims,
   audio, timing, full video decode, TypeScript, and required project checks.

Capture prerequisites found without displaying secret values: Slack OAuth
credentials exist but HANDOFF_PUBLIC_URL is HTTP; Slack needs a configured HTTPS
callback. GitHub OAuth credentials exist; GitHub App ID/private key are absent.
The demo repository and Slack workspace/channel need to be identified before
connection changes or posting. The user authorized choosing a GitHub repository,
selected DoodleByte Studio for Slack, and approved the integrity refresh.
The 30-second animated opening has rendered; full recording/editing is in progress.

## Slack and GitHub follow-up (2026-11-02)

Status: APPROVED by the user's follow-up to implement both planned features,
complete existing connections, and add useful integration features. Additional
scope: Slack failure-only filtering and delivery status; GitHub repository
pagination and server-side branch validation. The request is
to implement the planned Slack and GitHub features. Existing OAuth connections,
manual Slack posting, repository selection, and GitHub application imports are
already implemented. The guides explicitly identify the following future work.

1. Slack: add opt-in automatic completion notifications for console-triggered
   demo and BYOA rehearsals. Reuse the channel connection and server-built
   summary, cite controller evidence IDs, and keep delivery bounded. Show
   notification settings and delivery errors without changing drill results.
   Do not send messages during development except to local test servers.
2. GitHub: add installation authentication using a GitHub App with repository
   selection and minimum read permissions for current onboarding operations.
   Keep private keys in server environment configuration, mint short-lived
   installation tokens server-side, and handle expiry and revoked access.
   Preserve existing OAuth connections during migration. Do not add repository
   writes, PR creation, approval, or merging to this scope.
3. Confirm API contracts against official Slack/GitHub documentation before
   implementation. Update Settings, guides, deployment/security documentation,
   and docs/DECISIONS.md to describe the implemented behavior and permissions.
4. Test authorization, opt-in behavior, notification failures, installation
   access boundaries, token expiry, and secret handling using local HTTP test
   servers. Run `bash scripts/check.sh` after each implementation step and
   review the console changes in a browser. Report live-account checks as
   unavailable unless actually performed with authorized connections.

No protected-path or runtime-agent-prompt edits are proposed. Preserve the
concurrent film work and existing documentation changes. If console assets are
covered by the manifest, prepare the changes and request the required manifest
refresh approval after the implementation is reviewable.

Implementation: notification preferences and failure filter, delivery status,
GitHub App installation selection/read tokens, repository pagination, branch
validation, and default-branch-aware archive imports are in place. Existing
OAuth flows remain supported. Regression tests use local fake services and
synthetic controller records; they do not supply new drill evidence IDs.
`bash scripts/check.sh` passes Go formatting, vet, tests, and skills validation;
its remaining gate is the manifest refresh requiring explicit approval.
Browser review with isolated synthetic settings confirmed preference saving,
persisted failure filtering, GitHub App setup/error controls, and the existing
OAuth repository settings. Console JavaScript syntax and diff whitespace checks
also completed. No real Slack message or GitHub write was performed.

## Video refresh (2026-11-02)

Status: COMPLETED on 2026-11-02 after the user's approved direction: retain the owl as the central guide,
use real application footage, completely redesign the visuals in the website's
theme/colors, and study the Segue and other local film references.

Implementation: a 2:45 composition using actual application captures and
recorded controller evidence. The original SVG owl is the central guide, with
transparent vector poses, the website's charcoal/blue/purple palette, local
Plus Jakarta Sans, Phosphor icons, animated diagrams, camera moves, music, and
burned-in explanations. The 165-second master is exported and checked, with
poster, SRT, editable source, and a validation record in `docs/VIDEO_VALIDATION.md`.

| Time | Content |
| --- | --- |
| 0:00-0:20 | Owl introduction, current landing page, developer-handover problem |
| 0:20-0:35 | Baseline: registration, booking, confirmation email |
| 0:35-0:58 | Withdrawn token; booking persists while the email assertion fails |
| 0:58-1:10 | Animated comparison of the two controller outcomes |
| 1:10-1:32 | Actual Insights, cited source files, recorded Meta diagnosis |
| 1:32-1:48 | Missing configuration and the reminder-job failure |
| 1:48-2:06 | Repair proposal, matching recorded hashes, explicit missing recovery evidence |
| 2:06-2:26 | Applications and focused integration settings |
| 2:26-2:45 | Measured comparison, evaluation limits, repository link |

Before rendering: match each on-screen outcome to its controller evidence ID;
label recorded/replayed material; show a real Meta response rather than a staged
typing effect; do not present an integration as connected or a message as posted
unless the capture supports it. Correct the script's four-fault wording to match
the five-fault evaluation. Keep token usage out of the main proof sequence.

Validation so far: `npm run typecheck` passed. The first project check reported
`check OK` / `manifest OK`; the later check passes Go formatting, vet, tests, and
skills but stops at the manifest because concurrent integration work changed
`cmd/handoffproof`. Leave those files and the manifest untouched by this film
task. Six displayed evidence IDs are checked against existing controller
records. No new drill, Meta call, connection, or Slack post ran for this capture.
The stored assessment has no new recovery evidence ID, so the film explicitly
leaves recovery unverified.

Deliverables: revised script/captions, real UI captures, a checked 1080p MP4 under
three minutes, poster/thumbnail, and a brief validation record.

## UI layout and landing page refresh (2026-11-02)

Status: UI implemented and reviewed on 2026-11-02. User authorized commit/push
finalization, including the previously requested integrity-manifest refresh.

Validation: landing TypeScript check and production build completed; browser
review covered Settings connection states, integration detail pages and More
actions, application search/import/detail navigation, product screenshot tabs,
font loading, and narrow/wide page bounds. The required check script completed
gofmt, go vet, Go tests, skills checks, and manifest verification after the user
authorized finalization. `bash scripts/check.sh` reports `check OK`. No new drill
ran; screenshots use existing recorded demo evidence.

Follow-up scope explicitly requested by user: use official Phosphor icons rather
than text arrows, Plus Jakarta Sans on the landing page, fluid responsive page
widths, actual product screenshots in previews, and review anti-slop guidance.
Applied the available Stop Slop and design-critique skills; no separately named
anti-slop design file was found in the installed skills.
Latest direction: use the application's light theme and blue/purple palette on
the landing page, superseding the reference's dark colors.
Follow-up refinement: correct both lasers to use the light shader and application
blue; recapture all product screenshots using content-only viewport clips.
Latest user direction supersedes the light theme: restore the dark landing theme,
use square action buttons, preserve the current layout and content, and capture
the product screens at twice their display scale for sharper hero rendering.
Palette/motion refinement requested by user: use the application's charcoal,
blue, and purple tokens, and cap button movement at 2px with a damped spring.

Goal: keep overview pages compact and move setup, configuration, and detailed actions
into dedicated pages. Add a HandoffProof landing page using the landing design and
the actual React Bits component implementations from `D:/H-projects/code2play`.

1. Settings: replace expanding integration forms with consistent summary cards
   showing connection status and Connect / Manage actions. Provide dedicated Slack
   and GitHub pages with back navigation, configuration, connection details,
   repository/channel actions, and disconnect controls. Give report/sharing and
   server settings their own detail pages where appropriate. Place technical
   setup guidance in expandable troubleshooting sections.
2. Applications: make the main page a searchable application list with an Add
   application action. Move import/setup to a separate page; move per-application
   configuration, validation, rehearsal actions, and results into a detail page.
   Review the other console pages for the same overview/detail separation; keep
   their existing workflow and evidence behavior intact.
3. Landing: reproduce the reference landing's dark grid, indigo/teal palette,
   large type, laser hero, interactive panels, and animated calls to action with
   HandoffProof content and branding. Reuse its actual React Bits source components
   (LaserFlow, ShinyText, Magnet, StarBorder, CountUp, SpotlightCard, GlareHover,
   ClickSpark, and DecryptedText where used), retaining attribution/license details.
   Bundle the landing's React dependencies locally and serve its compiled assets
   through the existing Go application. Add an Open Control Room entry and a
   return link from the console; retain existing console deep links and OAuth
   callbacks. Keep current authentication boundaries; public hosting is outside
   this change.
4. Responsive/accessibility: consistent page headers, constrained form widths,
   clear primary actions, loading/empty/error states, keyboard focus, mobile
   layouts, and reduced-motion handling. Maintain existing console branding.
5. Record routing/build decisions in docs/DECISIONS.md. After each implementation
   step run `bash scripts/check.sh`; build the landing bundle and check desktop
   and mobile views plus integration and application navigation in a browser.
   Report the actual checks and any unavailable runtime verification.

Scope: console UI, landing assets/build setup, Go serving routes as needed, and
design documentation. Do not edit the runtime agent prompt or protected paths.
The reference project is read-only. Preserve unrelated existing workspace changes.

Status: APPROVED 2026-11-01. Day 2 closed live. Power-ups F1–F6 done (CI gate, verifier, what-if, benchmark, remediation PRs, Handover Score). Next: Day 3 (video, submission). Decisions in `docs/DECISIONS.md`.
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
- The Meta model does real work on screen (dependency map, diagnosis from
  controller evidence, repair proposal) and never issues a verdict. Drills
  run in a fixed controller order; the prompt's `drill_plan` phase is
  defined but not invoked by the current code.
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
| Responses | `POST /v1/responses` - recommended default for new agentic/coding work |
| Chat Completions | `POST /v1/chat/completions` - drop-in; no reasoning carryover; no search grounding / tool search |
| Messages | `POST /v1/messages` - Anthropic-compatible; point SDK at base host `https://api.meta.ai`; stateless |
| Auth | `Authorization: Bearer $MODEL_API_KEY`; official env var `MODEL_API_KEY`, key from the Model API dashboard |
| Models | `muse-spark-1.3` (latest, "recommended for new work", agentic-tuned); `muse-wonder-1.0` (most capable, same requests as Spark); `muse-spark-1.2` / `1.1` previous; `-contributor` variants discounted |
| Unknown model | `404` + `code: model_not_found`; a `503` on model lookup is inconclusive - retry |
| Structured output | Chat: `response_format` with `type: "json_schema"` + `json_schema`. Responses: `text.format`. A param from the other endpoint does not configure structured output (`text.format` on Chat returns 400). Decoding is constrained, output guaranteed to conform. Recursive `$ref` schemas rejected |
| Sampling | `temperature` 0–2, default 1.0. `top_p` in (0,1], default 1.0, `0` returns 400. Spark is tuned for the defaults; adjust one, not both; prefer clearer instructions over lowering temperature |
| Output limits | Responses: `max_output_tokens`, min 16, covers reasoning + visible output, shares the context window. Chat: `max_completion_tokens` (`max_tokens` alias accepted). `max_output_tokens` is rejected on Chat |
| Reasoning | `reasoning.effort` (Responses) / `reasoning_effort` (Chat): `none`, `minimal`, `low`, `medium`, `high`, `xhigh`. On by default. `none` returns 400 on Muse Spark |
| Tools | `function` or `custom` tools (`custom` is Responses-only). Responses: flat format, `function_call` items + `function_call_output` with `previous_response_id`. Chat: nested `function` key, `tool_calls` array + `tool` messages. Names must match `^[a-zA-Z0-9_.-]+$` with at most one dot. `strict` defaults false. `parallel_tool_calls` defaults true. `max_tool_calls` caps built-in tools only - bound function calls in our own loop. `tool_choice` must be `"auto"`. Always validate tool args client-side |
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
caps (model-dependent - handle 400s and read `GET /v1/models` at runtime); pricing and
quota numbers; hackathon submit-page rules (re-read at submission per
hp-submission-checklist).

## Agent/controller interface

CLI (`cmd/handoffproof`): `scan` = static scan + intake + dependency proposal (JSON
phases 1–2); `drill` = fixed-order execution + diagnosis + repair proposal + rerun
(the `drill_plan` phase is defined in the prompt but not invoked); `report` = static
HTML evidence report.

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
| C2 | - | `scripts/manifest.sh` hashes `acceptance internal/controller internal/drills scripts/manifest.sh` - omits `evidence/`; `AGENTS.md` invariants omit `scripts/manifest.sh` | Keep `evidence/` out of the manifest (runtime output, gitignored) but the controller still rejects repair diffs touching it. Add `scripts/manifest.sh` to the `AGENTS.md` list for consistency (Day 1, with approval) |
| C3 | Manifest rule implies hashing `evidence/` | `.gitignore` ignores `evidence/*` except `.gitkeep` | Manifest covers committed files only; runtime evidence integrity comes from controller-written records plus the report's hash footer |
| C4 | - | `internal/evidence/` (defines `Record` + classes) is in no protected list | Add `internal/evidence/**` to the protected set - altering it would break verdict integrity (Day 1, with approval) |
| C5 | R7 + example: report injections as `prompt_injection_attempt` | Output schemas and `internal/evidence/evidence.go` have only the 4 classes | Do not shoehorn into the 4 classes. Log as a separate controller security event and surface it in the report (schema defined Day 2) |
| C6 | Workflows: registration, booking, confirmation_email | hp-write-acceptance-tests adds `documents.spec.ts`; scaffold adds `POST /uploads` | Uploads/storage is drill support for `incomplete_backup` assertions, not a 4th customer workflow. Keep 3 workflows + recovery spec |
| C7 | Baselines: `static_scan_only`, `fresh_deployment_only`, `checklist_only` | hp-eval-results-table arms drop `checklist_only` | Run the 2 automated arms + full; `checklist_only` only if time permits and as a labeled target |
| C8 | Params mix protocols: `temperature 0.1` + `top_p 0.9` (docs: one, not both; prefer 1.0 for Spark) and `max_output_tokens` (Responses-only) + `response_format` (Chat-only) | hp-meta-api-client: temperature 0.1 | Responses API; omit temperature and top_p (defaults); `max_output_tokens 4096`; schemas via `text.format`. Revisit only if eval shows unstable output |
| C9 | `model_id: ${META_MODEL_ID}` | Official key env var is `MODEL_API_KEY`, repo uses `META_API_KEY` | Read `META_API_KEY`, fall back to `MODEL_API_KEY`; document in `docs/DECISIONS.md` |
| C10 | Example `EV-0412` | `evidence.NewID` yields `EV-` + 8 hex | Code is normative; the example is illustrative |
| C11 | `request_drill` / `drill_plan` include `fresh_deployment_baseline`, but `supported_scope_v1.drills` lists only 3 | hp-implement-drill: baseline is the gate | No change - baseline gates the 3 drills, it is not counted as one |
| C12 | Ownership tools named, storage not specified | No ownership file in the layout | Day 2 decision: controller-held file outside the repo under test; record in `docs/DECISIONS.md` |
| C13 | "Fresh sandbox copy of the repo" (repair target ambiguous) | - | Repairs apply to a `demo-app` copy only; the controller never patches itself at runtime |

## 3-day build order

Always-on constraint skills: hp-verdict-integrity, hp-protected-paths,
hp-untrusted-input, hp-secrets-and-safety, hp-hackathon-rules. After every step run
`bash scripts/check.sh` and report the command + result. Priority when short on time
(hp-hackathon-rules): working E2E demo with the Meta model doing real work, then video,
then one honest results table; cut extra drills, polish, second app.

Day 1 - deterministic core, no model needed.

| Step | Skill | Output | Done when |
| --- | --- | --- | --- |
| 1.1 Mail sink | hp-build-mail-sink | `mocks/mail-sink`: `POST /send` (Bearer), `GET /messages`, `POST /admin/revoke\|reset` (controller-only admin token), request logging without full tokens | Test: send OK, revoke, send 401, message count unchanged |
| 1.2 Demo app | hp-scaffold-demo-app | `demo-app` Node 20 + Express (TS), Compose PostgreSQL, users/bookings/documents, `POST /register\|/bookings\|/uploads`, `GET /health`, `MAIL_API_TOKEN` via sink, `SCHEDULER_TOKEN` job, `/data/uploads`, DB-only `backup.sh`, incomplete `HANDOVER.md`, silent-failure booking handler, one injection comment, `eval/ground_truth.json` outside the agent's reach | `docker compose up`; 3 workflows pass by hand |
| 1.3 Static scan | hp-static-scan | `internal/scan`: env read-vs-documented, jobs, upload paths, backup coverage, hardcoded hosts, `ask <name>` phrases; stable sorted JSON | Fixture tests pass; demo-app missing var detected; scan-only baseline recorded |
| 1.4 Acceptance + freeze | hp-write-acceptance-tests, hp-protected-paths | `acceptance/`: registration, booking, confirmation-email, documents specs; independent assertions, polling, unique data | Green on baseline; email test fails after revoke; `manifest.sh create`; folder frozen |

Day 2 - controller, drills, agent loop (Meta API goes live).

| Step | Skill | Output | Done when |
| --- | --- | --- | --- |
| 2.1 Agent client | hp-meta-api-client | `internal/agent`: Responses client, prompt render, tool loop with budget, schema validation, usage logging, 429/5xx backoff; `docs/DECISIONS.md` updated | Recorded run: intake, dependency map, drill plan, diagnosis from real evidence |
| 2.2 Drills | hp-implement-drill | `internal/drills` + controller runner: baseline gate + drills in order withdrawn_token, missing_configuration, incomplete_backup (last only if time remains); isolated internal Docker network, limits, teardown-always; controller-written evidence | Param-validation tests; one real run yields evidence records with IDs |
| 2.3 Repair + rerun | hp-repair-and-rerun | Diff review (protected reject), sandbox-copy apply, hash check, identical-suite rerun, before/after IDs | One fault red to green, same acceptance hashes |
| 2.4 Report | hp-evidence-report | `internal/report`: single offline HTML file, class badges, per-finding evidence/log/source/impact/confidence, inferred/untested sections, integrity footer; escaped + redacted | Sample run renders; each finding links to its record |

Day 3 - honest numbers, video, submission.

| Step | Skill | Output | Done when |
| --- | --- | --- | --- |
| 3.1 Results | hp-eval-results-table | `docs/RESULTS.md`: per-arm faults injected/caught/missed, false alarms, confirmed/refuted risks, tokens/run; date, commit, evidence IDs | Table contains measured numbers only; one miss reported plainly if present |
| 3.2 Video | hp-demo-video | `docs/DEMO_SCRIPT.md` + recorded take: problem, baseline, token drill, evidence view with model diagnosis, docs drill, repair + green rerun, results | Safe take recorded (<3:00, readable, no secrets); captions present |
| 3.3 Submit prep | hp-submission-checklist | Public repo, README event section + setup + video link, secrets sweep incl. history, form fields, indie track, contributors | All 7 checklist items verified; official rules re-read |

Nov 4 morning is contingency only; submit by ~3:00 PM ET.

## Report sharing (2026-11-01, done; revised same day)

The Evidence page's **Full report** button opens a share dialog from a
body-level fixed overlay, so it centers on the viewport - not on the page or
report beneath it - with scroll locked, focus trapped, and backdrop/Escape
close. Copy link mints a server-held, read-only share link (`POST /api/share`
-> `?s=...`, 24h, max 50 live, GET-only): recipients open the report with no
sign-in, cookie, or token typing, and the link cannot run drills, post to
Slack, or mint links. Download PDF uses the generated report's themed print
stylesheet (“Save as PDF”); Post to Slack sends a server-built counts-only
summary via `HANDOFF_SLACK_WEBHOOK`. Covered by `TestShareLinks` and
`TestSlackIntegration`; rules documented in `docs/SECURITY.md`.

## Report redesign (2026-11-01, done)

The generated report now uses the Control Room design system end to end:
IBM Plex Sans/Mono, the Carbon palette and tags, a white 48px report header,
the dark Qiskit-style hero with the purple-to-blue rule, the app's Storyset
illustrations embedded inline (evidence, insights, handover), inline Carbon
icons, a sticky section nav, the app's stat row, Carbon finding cards with
evidence links, and a dedicated acceptance-integrity section. Print/PDF keeps
the theme, hides navigation/actions/pagination/illustrations, and expands
every evidence row. Covered by the report render test's design assertions
and verified with browser screenshots.

## HOP Connect documentation (2026-11-01, done)

Slack and GitHub features are documented as user guides:
`docs/guide/connect-slack.md` (OAuth scope, HTTPS callback, storage,
disconnect, troubleshooting), `docs/guide/connect-github.md` (PKCE/`repo`,
repository selection, encrypted token storage, limits), and
`docs/guide/applications.md` (BYOA profile, validation, rehearsals, limits).
README links all three.

## Bring Your Own App onboarding (2026-11-01, done)

`internal/byoa` adds a declarative `handoffproof.json` profile (services,
health, probe runtime, env names, credentials, workflows with http /
sql_count / log_contains / file_exists steps, data stores, allowlisted
backup), fail-closed validation, secret-path/symlink/size-safe source copy,
Compose denylist checks, `docker compose config` service verification, a
generated no-published-ports internal-network override, and a sandbox runner
for baseline, withdrawn-credential, missing-configuration, and backup
rehearsals. Evidence records gain optional `app_id`, `profile_version`,
`source_sha256`, and `fault_target`. The server registry
(`/api/applications...`) imports local folders or GitHub archives with the
connected account, AES-GCM-encrypts wizard environment values, and is
operator-only. The console gains an **Applications** page with an import
wizard, profile editor, validate/scan/run/delete, and per-kind fault target.
Proven by profile/compose unit tests, the encrypted-registry API test, and a
Docker e2e rehearsal on a non-demo Python fixture
(`internal/byoa/testdata/miniapp`). Limits are documented in the guide.

## Application structure, onboarding, Settings, GitHub (2026-11-01, done)

The console is structured as an end-user application: the sidenav is grouped
**Start** (Get started), **Control Room** (Readiness, Rehearse, Evidence,
Insights), and **Project** (Settings, About HOP), with one shared page-head
pattern (title, plain-language purpose, Storyset illustration) and a header
shortcut to Settings. The console opens on **Get started**, a six-step
illustrated checklist whose done states come from live score/evidence/
integration data and whose buttons open the page where each step happens.

**Settings** is viewable by console users; only operators can change
anything. It covers Slack (status/connect/post/disconnect), GitHub, report
defaults (rows per page, saved in the browser and shared with the generated
report), and server/safety status. GitHub uses OAuth web flow with PKCE and
the `repo` scope: callback state is one-time and cookie-bound, the token is
AES-GCM encrypted outside the repo and never returned to the browser, and
operators pick the remediation repository/branch from the repositories the
connection can access (`/user/repos`, verified via `/repos/{owner}/{repo}`).
Covered by `TestGitHubOAuthFlow`; setup in the README.

## Slack connection (2026-11-01, done)

Real Slack connection is Slack OAuth v2 with the `incoming-webhook` scope:
console operators click **Add to Slack** in the Full report dialog (or the
generated report), approve in Slack, and choose the channel. The callback
(`GET /api/integrations/slack/callback`) is protected by a one-time,
10-minute, HttpOnly `hp_slack_state` cookie plus a server-side state record;
it exchanges the code at `oauth.v2.access`, validates the returned
`hooks.slack.com/services/...` webhook, and stores only that webhook
(AES-GCM encrypted, outside the repo; `HANDOFF_SLACK_STORE` /
`HANDOFF_SLACK_ENCRYPTION_KEY` control the store). OAuth bot/user tokens are
discarded. Status, post, and disconnect are operator-only; share links stay
read-only. Covered by `TestSlackOAuthFlow` (fake Slack exchange, encrypted
store, state replay rejection, disconnect) plus the existing post test.
Setup is documented in the README's "Connect a real Slack workspace".

## Report source references (2026-11-01, fixed)

Diagnosis findings could reach the report with no `source_references` (the
spec declared the field but did not require it), so the report's Source cell
rendered blank. `LoadSpec` now requires at least one `source_references`
entry per diagnosis finding before a diagnosis can be saved; report
generation carries every reference into the report; and a finding that still
has none renders "Not provided — citation missing" instead of an empty cell.
The current withdrawn-token diagnosis was backfilled with verified
`src/index.ts` / `src/mail.ts` references; `verify --model=false` reports all
five citations verified. Covered by
`TestDiagnosisSchemaRequiresSourceReferences` and the report render test.

## Evidence pagination (2026-11-01, done)

Console Evidence and the generated report use the same Carbon pagination:
10/25/50 rows per page, “Showing X–Y of Z records”, First/Previous/numbered/
Next/Last controls. Console drill/result/search changes reset to page 1;
report summary links jump to the page holding the cited evidence record;
print/PDF output expands every row and hides the pager. Covered by the report
render test's pagination assertions.

## Post-audit security remediation (2026-11-01, done)

Strict multi-agent audit found 3 CRITICAL + H1–H13 + M/L findings. All remediated
one by one; the finding-by-finding log, threat model, and residual risks live in
`docs/SECURITY.md`. Headlines: controller-generated compose override (drill
sandboxes publish no ports, internal-only networks, `docker exec` health checks),
loopback binds, per-run crypto tokens, env allowlist, always-authenticated
console, secret-path deny-by-meaning + fail-closed redaction, measured (never
asserted) integrity, `npm ci` + non-root images + CI permissions. Verified:
`bash scripts/check.sh` OK, full Go suite green, 4 drills correct, acceptance
4/4, console 401/403/200 probes. Remaining before Day 3: video + submission prep.

## Validation Plan

- Every step: `bash scripts/check.sh` (gofmt, vet, tests, skills format, manifest).
- Sandbox: `docker network inspect` shows `Internal: true`; no `--privileged`, no socket/home mounts; teardown verified.
- Secrets: `git grep -n -i -E "(api[_-]?key|secret|token)\s*=\s*['\"][A-Za-z0-9]{16,}"` empty; pre-submit `git log -p` sweep.
- Verdict integrity: `grep -rn "Passed" internal --include=*.go | grep -v internal/controller | grep -v internal/evidence` shows no assignments; every observed claim carries an `evidence_id`.
- Untrusted input: unit test with injection string + fake key asserts delimiters, `[REDACTED:...]`, unchanged verdicts.
- E2E: healthy baseline green, token drill red (email assertion), diagnosis cites evidence, repair rerun green with unchanged hashes.
- Highest risk: Day 2.1 - first real Meta API run (auth, model ID, `text.format`, tool loop). Validate with a minimal live call before wiring all phases.

## Risks / Open Questions

- R1: Default sampling may vary. Mitigation: eval watches for unstable output; revisit temperature/top_p only if eval shows instability.
- R2: Docker Desktop + Playwright + Node 20 setup on Windows 11/Git Bash may eat Day 1. Mitigation: sink + scan first (no Docker needed for scan unit tests).
- R3: API key availability/quota unknown. Mitigation: Day 2 starts with one minimal live call; keep a recorded-evidence replay path so controller/report work continues offline.
- Q1: Held-out fault needs someone outside the implementation - who, and when? (Else eval runs on planted faults only, labeled as such.)
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
