# Connect Slack

## Purpose

Post a server-built HandoffProof report summary to a Slack channel: evidence
record counts, pass/fail totals, manifest-integrity state, and up to three
failing assertion names. Webhook URLs, tokens, share links, and model text
are never posted.

## Who can do this

- A **console operator** can connect, post, and disconnect Slack.
- A read-only share recipient cannot connect, post, or disconnect.

## Before you start

1. Create an app at <https://api.slack.com/apps>.
2. Turn on **Incoming Webhooks**.
3. Add the bot scope `incoming-webhook`.
4. Add this Redirect URL under **OAuth & Permissions**:

   `https://<your-public-host>/api/integrations/slack/callback`

5. Set these server environment variables and restart HandoffProof:

```text
HANDOFF_SLACK_CLIENT_ID=<client id>
HANDOFF_SLACK_CLIENT_SECRET=<client secret>
HANDOFF_PUBLIC_URL=https://<your-public-host>
```

Slack redirect URLs must use HTTPS. For a local server, use an HTTPS tunnel
to `127.0.0.1:8787` and open the console through that HTTPS address.

## Connect

1. Open **Settings -> Slack -> Add to Slack**, or **Evidence -> Full report
   -> Add to Slack**.
2. Approve in Slack and choose the channel.
3. Return to HandoffProof. Settings should show **Connected**, the workspace,
   and the channel.

## Post a summary

Use **Post to Slack** in Settings, the Full report dialog, or the generated
report. Posting is manual today; automatic drill notifications are a planned
HOP Connect feature, not a current one.

## What is stored

Only Slack's channel webhook is stored, AES-GCM encrypted in a server-side
file outside the repository. Control it with `HANDOFF_SLACK_STORE` and
`HANDOFF_SLACK_ENCRYPTION_KEY`. Slack bot/user access tokens returned during
OAuth are discarded.

## Disconnect

**Disconnect Slack** deletes the stored webhook from this server. It does
not uninstall the Slack app; remove that in Slack. A connection supplied by
the `HANDOFF_SLACK_WEBHOOK` environment variable cannot be disconnected from
the UI.

## Troubleshooting

- **Not configured**: set the two `HANDOFF_SLACK_*` credential values and
  restart the server.
- **Must be HTTPS**: set `HANDOFF_PUBLIC_URL` to an HTTPS address and
  register the matching callback in Slack.
- **State did not match / expired**: start again from HandoffProof; the
  OAuth state lasts 10 minutes and works once.
- **403**: sign in as a console operator; share recipients are read-only.
