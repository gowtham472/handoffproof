---
name: hp-submission-checklist
description: Use in the final hours before submitting to the hackathon. Walks through repository, video and form checks.
---
# Task: submit

Deadline: Nov 4, 9:00 PM ET (Nov 5, 6:30 AM IST). Target: submit 6+ hours early.

1. `bash scripts/check.sh` passes; manifest OK.
2. Repo is public; README has "Built during the event", setup steps, and a link to the video.
3. No secrets anywhere in history: `git log -p | grep -i -E "api[_-]?key|secret"`.
4. `docs/RESULTS.md` contains only measured numbers.
5. Demo video under 3:00, uploaded, link opens in a private window.
6. Submit on the Cerebral Valley submit page; add all contributors; choose the indie track.
7. Re-read the official rules for any field or requirement not covered here.
