# Connect GitHub

## Purpose

Connect a GitHub account so HandoffProof can remember the repository and
branch used for remediation work and application onboarding. The GitHub
token stays on the server and is never returned to the browser.

## Who can do this

- A **console operator** can connect, list repositories, save a repository,
  and disconnect.
- Other console viewers see status only.
- Read-only share recipients cannot use GitHub controls.

## Before you start

1. Register an OAuth app at
   <https://github.com/settings/applications/new>.
2. Set the Authorization callback URL to:

   `https://<your-public-host>/api/integrations/github/callback`

   For local use, GitHub also accepts a loopback callback such as
   `http://127.0.0.1:8787/api/integrations/github/callback`.
3. Set these server environment variables and restart HandoffProof:

```text
HANDOFF_GITHUB_CLIENT_ID=<client id>
HANDOFF_GITHUB_CLIENT_SECRET=<client secret>
HANDOFF_PUBLIC_URL=https://<your-public-host>
```

## Connect

1. Open **Settings -> GitHub -> Connect GitHub**.
2. Approve in GitHub. HandoffProof uses OAuth with PKCE and the `repo`
   scope.
3. Click **Load repositories**.
4. Choose the remediation repository and branch, then **Save repository**.
   HandoffProof verifies the repository with GitHub before saving it.

## What HandoffProof does with access

Today the connection stores the account, selected repository, and branch
for remediation and Bring Your Own App onboarding. HandoffProof does **not**
automatically push branches, open pull requests, approve, or merge. The
remediation PR command remains a dry run unless a human runs its printed
commands.

## What is stored

GitHub access and refresh tokens are AES-GCM encrypted in a server-side file
outside the repository. Control it with `HANDOFF_GITHUB_STORE` and
`HANDOFF_GITHUB_ENCRYPTION_KEY`. Status responses expose only account,
repository, branch, and scope.

## Disconnect

**Disconnect GitHub** deletes the stored tokens from this server. It does
not revoke the OAuth app in GitHub; revoke that in GitHub settings.

## Limitations

- One GitHub account, repository, and branch per HandoffProof server.
- Repository loading is capped at 100 repositories.
- The `repo` scope is broad. A fine-grained GitHub App is the planned
  least-privilege migration before write automation scales.

## Troubleshooting

- **Not configured**: set the two `HANDOFF_GITHUB_*` credential values and
  restart the server.
- **State did not match / expired**: start again from Settings; the OAuth
  state lasts 10 minutes and works once.
- **Repository not accessible**: reconnect with an account that can access
  the repository, then load repositories again.
- **Invalid branch**: use the repository's real branch name; HandoffProof
  defaults to GitHub's reported default branch.
