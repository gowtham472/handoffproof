# Evidence sample (sanitized)

A sanitized copy of one real HandoffProof run, kept in the repository so the
benchmark's model-quality numbers can be reproduced without the local
`evidence/` directory (which is git-ignored).

Source run: `withdrawn_test_token` drill run
`run-1793544564914230200` against the bundled demo app, with the live
diagnosis and model verification produced from it.

| File | What it is |
| --- | --- |
| `run-1793544564914230200-withdrawn_test_token.json` | Controller evidence records for the run |
| `diagnosis-withdrawn_test_token.json` | Model diagnosis for that drill |
| `verification.json` | Model adversarial-verification verdicts for the diagnosis |
| `live-full.log` | Live phase log (diagnosis, repair proposal, assessment) with token headers |
| `live-scan.log` | Live scan log (intake summary, dependency map) with token headers |

Sanitization: email addresses were replaced with `user@example.test`, and
credential-shaped strings (API keys, bearer tokens, Slack webhook URLs) were
replaced with `[REDACTED]`. Evidence IDs, file:line citations, token counts,
and verdicts are unchanged, so the numbers are real; anything that could
identify a person or a secret is not.

Reproduce the model-quality section against this sample:

```bash
handoffproof eval --evidence-dir docs/evidence-sample
```

Note this overwrites `docs/RESULTS.md`; rerun `handoffproof eval` with no
flag afterwards to regenerate it from the full local evidence.
