# controller

Read the root AGENTS.md first. Relevant skills: hp-verdict-integrity, hp-protected-paths, hp-implement-drill

Local constraints:
- Only this package issues verdicts and writes evidence.
- Verify the manifest before and after every run; abort on mismatch.
- The agent never imports this package's verdict-setting code.
