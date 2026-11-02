# Decisions

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
