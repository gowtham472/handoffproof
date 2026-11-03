# privatemode

Read the root AGENTS.md first. Relevant skills: hp-meta-api-client, hp-secrets-and-safety

Local constraints:
- Routing and egress only. Never decides pass/fail and never writes evidence.
- Fail closed: an unknown mode or a non-loopback endpoint in local mode is an error, never a fallback to the cloud.
