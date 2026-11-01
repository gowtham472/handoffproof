---
name: hp-meta-api-client
description: Use when writing or changing code that calls the Meta Model API, renders the agent prompt from the JSON spec, or runs the agent tool loop in internal/agent.
---
# Task: Meta API client and agent loop

## First
Read the official docs (dev.meta.ai). Record endpoint, auth header, model ID and structured-output
support in `docs/DECISIONS.md`. Do not guess any of them.

## Build (internal/agent)
- Config from `META_API_KEY`, `META_MODEL_ID`, optional `META_API_BASE_URL`.
- Render the system prompt by joining the arrays in `prompts/handoffproof_agent_prompt.json`.
- Tool loop: model returns a tool call, Go validates it against the allowlist and budget, executes it,
  returns the result as untrusted data. There is no tool that sets a verdict.
- Validate every response against the JSON schemas. On failure retry with the error, up to 2 times,
  then fail closed.
- Log token usage per run. Timeout and retry with backoff on HTTP 429 and 5xx.

## Rules
Follow hp-untrusted-input and hp-secrets-and-safety. Temperature 0.1.

## Done when
A recorded run: intake, dependency map, drill plan, and diagnosis produced from real controller
evidence, with a token usage summary.
