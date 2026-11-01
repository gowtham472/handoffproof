# Decisions

Record each design choice with date, options considered, and the reason. Include the verified
Meta Model API details (endpoint, auth, model ID, structured output support).

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-11-01 | Protected set: `acceptance/**`, `internal/controller/**`, `internal/drills/**`, `internal/evidence/**`, `evidence/**`, `.manifest.sha256`, `scripts/manifest.sh` (C1/C2/C4) | JSON paths missed the `internal/` prefix and the manifest dot; `internal/evidence` defines the verdict classes |
| 2026-11-01 | Responses API; omit `temperature` and `top_p` (defaults) (C8) | dev.meta.ai verified: Responses is recommended for agentic work; Spark runs best at sampling defaults — revisit only if eval shows unstable output |
| 2026-11-01 | Ownership declaration: controller-held file outside the repo under test (C12) | JSON named the tools without storage; keep ownership data out of the analyzed repo |
| 2026-11-01 | Acceptance: Playwright API testing (request fixture) + DB + mail-sink checks; demo-app is API-only; no browser downloads | User decision: fast, deterministic, independent assertions |
| 2026-11-01 | Drill priority: `withdrawn_test_token` end to end first, then `missing_configuration`; `incomplete_backup` last only if time remains | User decision: highest demo value first |
