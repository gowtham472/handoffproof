# HandoffProof — Demo Video Script (2:30, mascot-centric)

Current cut: an animated walkthrough with recreated UI. Its on-screen results
refer to existing demo artifacts; the animation is not a recording of a live
drill or integration connection. The next cut should show actual application
footage and the recorded evidence supporting each outcome.

Mascot: Pip the owl (yellow owl from `assets/characters_and_poses.svg`), who
guides every scene. Music: "Inspiring Advertising" by Rafael Krux (CC BY 4.0).
Target: hackathon judges — show a real rehearsal, real evidence, real repair.

## 0:00 Opening (0–8s)

Pip flies in from the left and lands beside the HandoffProof wordmark as it
draws itself. Tagline appears word by word: "Rehearse the day your developer
leaves."

## 0:08 The problem (8–20s)

Pip points at the Readiness page: four planted faults, score 85. Voice line:
"Ravi is leaving Friday. What breaks on Monday? HandoffProof finds out today."

## 0:20 "Rehearse." (20–40s)

Punchline card: **Rehearse.** Cut to the Rehearse page. The cursor clicks Run
on `withdrawn_test_token`. Sandboxes spin up; the confirmation-email assertion
goes red. Pip winces — "the app looks fine, but the mail never arrives."

## 0:40 The failure (40–48s)

Evidence row appears: `confirmation_email — passed: false`. The failure is
deterministic: the same drill in Docker fails the same way every time. No
model was involved in the verdict.

## 0:48 "Prove." (48–60s)

Punchline card: **Prove.** Proof cards sweep in: **5/5 faults caught**,
**Handover Score 85**, **100,749 tokens** of model work — every number measured
from real artifacts.

## 1:00 The diagnosis (60–80s)

The Meta model reads the evidence, not the code alone. A diagnosis card types
out: withdrawn `MAIL_API_TOKEN`, with `file:line` citations —
`demo-app/src/mail.ts:2-16`. The adversarial verifier checks every citation:
verified, 5 of 5. Pip nods: "explained, and every line checked."

## 1:20 Red to green (80–95s)

The model proposes a repair diff; the controller applies it to a copy and
reruns the drill. The confirmation-email assertion flips red to green in the
Evidence table. The model proposed; the sandbox decided.

## 1:35 "Hand over." (95–115s)

Punchline card: **Hand over.** The Applications page: a real Docker Compose
app is imported, its profile validated, baseline and fault drills run, and
evidence records carry the app ID and source hash.

## 1:55 Connected (115–128s)

Slack: the rehearsal summary posts to the team's channel — counts only, never
secrets. GitHub: the repository connects and the app imports straight from
`main`. Pip stamps each integration: connected.

## 2:08 Proof (128–142s)

Final proof rail: static scan 3/5, drills 4/5, full HandoffProof 5/5 —
second fixture 3/3. Citations verified, schema retries separated from turns,
report hashes measured. "Misses reported plainly. Nothing asserted."

## 2:22 Close (142–150s)

Tagline word by word: "Rehearse the exit. Prove the handover."
`github.com/gowtham472/handoffproof`. Music credit: "Inspiring Advertising"
by Rafael Krux (CC BY 4.0). Pip waves from the corner.

Font: Plus Jakarta Sans (as in the SAAP film), loaded locally by Remotion. Music grid measured at ~117.5 BPM; cues sit on bar lines. Built on the SAAP film pattern in D:/C-projects/saap-film: beat-timed punchlines, camera push-ins, frame-driven UI twins, mascot hop-ins, versioned deliverables with poster and thumbnail.
