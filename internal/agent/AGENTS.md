# agent

Read the root AGENTS.md first. Relevant skills: hp-meta-api-client, hp-untrusted-input, hp-secrets-and-safety

Local constraints:
- No tool may set a verdict. Enforce tool budget and allowlist in Go.
- Validate every model response against the schemas; fail closed.
