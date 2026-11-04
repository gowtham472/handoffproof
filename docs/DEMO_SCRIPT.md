# HandoffProof demo v4: rehearse the handover

Local working cut duration: 2:55. The committed repository video is a separate earlier cut. Picture: 1920 × 1080, 30 fps. Local Zira narration, music, and captions.

The first 30 seconds animate a customer booking, developer-owned email access, departure, and missing confirmation. The original owl follows the dependency and rewinds the handover. The conceptual explanation is labeled.

| Time | Narration |
| --- | --- |
| 30–39s | Start by connecting the source repository. This is the real GitHub account and the HandoffProof repository. |
| 39–50s | Choose the repository and branch, then save. This gives the team a source and remediation context. It does not automatically push a repair. |
| 50–58s | Slack is connected to a dedicated private demo channel in DoodleByte Studio. |
| 58–67s | Enable completion notifications, and choose failures only. The team gets the result where they already work. |
| 67–78s | Now rehearse the application. First run a healthy baseline: registration, booking, and confirmation email. All three controller checks pass. |
| 78–88s | Next, select Withdraw token and run it. HandoffProof removes the mail credential inside an isolated sandbox. Waiting time is cut here. |
| 88–97s | The controller reports four passing assertions and one failure. Open Evidence to see exactly which workflow broke. |
| 97–105s | The confirmation email is missing. But the booking still exists. A successful booking screen alone would have hidden this failure. |
| 105–112s | Each observed result has a controller evidence ID. The model cannot turn a red result green. |
| 112–124s | Here is a real, previously recorded Meta diagnosis of the same drill. It compares the healthy email, the surviving booking, and the failed confirmation. |
| 124–137s | The explanation points to the booking and mail source files. The handover action is to replace developer-owned access, make delivery failures visible, and rerun the unchanged checks. |
| 137–148s | Back in Slack, the real notification arrives from HOP. The automatic message identifies this rehearsal and includes its evidence references. |
| 148–158s | HandoffProof also shows delivery status. GitHub supplies source context; Slack carries the outcome to the team. Neither integration changes a test verdict. |
| 158–168s | The result is a concrete handover task, backed by a recorded failure. A repair still needs a controller-observed rerun before it can be called recovered. |
| 168–175s | HandoffProof. Rehearse the handover. Keep the evidence. |

Real Cap footage shows GitHub repository selection, Slack settings, a fresh baseline and token-withdrawal rehearsal, evidence filtering, and actual automatic Slack delivery. Waiting time is cut. The Meta response is an actual earlier recording, explicitly dated and retaining its original evidence IDs. No post-repair recovery is claimed.

Benchmark caution: the historical model comparison has mixed-evidence and repair-workspace
limitations. Do not present first-try repair failures as proven recovery. Whiteboard
evidence covers one synthetic image; auto-drafter miniapp baseline passes are 0/3.
