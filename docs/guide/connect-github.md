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

Choose GitHub App authentication for repository-scoped read access. Existing
OAuth connections continue to work; connecting an installation replaces the
current connection on this server.

## GitHub App setup (recommended)

1. Register a GitHub App in your GitHub developer settings. Give it repository
   **Contents: Read-only** and **Metadata: Read-only** permissions. No organization
   permissions or webhook subscriptions are needed; turn off the app's webhook
   if unused. Install it for only the repositories you want HandoffProof to read.
2. Generate the app's RSA private key. Supply `HANDOFF_GITHUB_APP_ID` and
   `HANDOFF_GITHUB_APP_PRIVATE_KEY` (the full multiline PEM) through the server's
   environment or secret manager. Never put the key in the repository or browser.
3. Restart the server. In **Settings -> GitHub -> GitHub App**, click **Load
   installations**. **Install on GitHub** opens installation setup if needed;
   return and load again after installation.
4. Choose an installation and click **Connect installation**. HandoffProof
   verifies that it belongs to the configured app, is not suspended, and can
   grant the required read access. Then load and save a repository and branch.

This flow uses an operator's explicit installation selection rather than an
installation callback or user OAuth grant. The console operator is trusted to
select any installation belonging to this server's configured GitHub App.
Use a separate app/server for separate tenants. The private key must be RSA
(PKCS#1 or PKCS#8 PEM), at least 2048 bits.

Only the installation ID is stored in the encrypted credential payload.
Short-lived installation tokens are minted anew for each repository operation,
requested with Contents/Metadata read permissions, and never persisted or sent
to the browser. Expired/empty tokens and revoked access stop the operation;
disconnect still works locally when remote access is revoked. Disconnect does
not uninstall the app in GitHub.

## OAuth setup (existing connection option)

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
   HandoffProof verifies the repository and the existence of the branch with
   GitHub before saving. A blank branch uses the repository's default branch.
   Changing the repository selects its default branch. Use **Refresh repositories**
   and **Load more repositories** to browse beyond the first 100 results.

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
- Repository and installation lists load 100 entries per page, with explicit
  Load more controls. Each operation requests a bounded page.
- OAuth still uses the broad `repo` scope. Migrate by connecting a GitHub App
  installation to limit access to selected repositories and read permissions.
- GitHub App access is read-only. PR creation and repository writes remain
  outside the integration; the CLI's remediation PR command is a dry run.

## Troubleshooting

- **Not configured**: set the two `HANDOFF_GITHUB_*` credential values and
  restart the server.
- **State did not match / expired**: start again from Settings; the OAuth
  state lasts 10 minutes and works once.
- **Repository not accessible**: reconnect with an account that can access
  the repository, then load repositories again.
- **Invalid branch**: use the repository's real branch name; HandoffProof
  defaults to GitHub's reported default branch.
