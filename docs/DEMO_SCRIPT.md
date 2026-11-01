# Demo script (3 minutes, only work built during the event)

0:00 Problem: the site looks fine, the developer is gone, confirmations silently stop.
0:20 Control Room: Handover Score 84, scoreboard 5/5, live evidence. "The model never decides pass or fail - the controller does."
0:45 Baseline drill from the console: register, book, email arrives. All green with evidence IDs.
1:10 Withdraw-token drill: booking still says confirmed, controller email assertion fails (red). Open the diagnosis: evidence-cited, alternatives rejected.
1:45 Verifier: deterministic floor flags missing citations; model pass anchors the bug to the swallowed mail error in source.
2:05 What-if: remove SCHEDULER_TOKEN, preview the reminder job breaking; missing-config drill confirms it.
2:25 Repair: model proposes, controller rejects a bad diff, applies the good one to a sandbox copy, identical acceptance rerun goes green. PR prepared (dry run) for a human.
2:45 Results table: static scan misses the silent failure by design; full HandoffProof catches 5/5; held-out fixture 3/3. Score trend closes.
2:55 Full report: page through the evidence, open a finding to show its verified source references, then copy a 24-hour read-only share link, save the themed PDF, or Add to Slack / post the counts-only summary.

Rules: show the Meta model working on screen, no secrets, captions on, keep under 3:00.
Full timing source: `.agents/skills/hp-demo-video/SKILL.md`.
