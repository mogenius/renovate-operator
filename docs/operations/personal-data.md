# Personal Data

This page lists the personal data the operator processes, where it ends up, and what bounds its retention. It is meant as input for your record of processing activities; the legal basis and retention periods are decisions of whoever runs the installation. Unless a section names a platform, it applies to every supported Git platform.

## Repository paths

Repository paths identify a person when the repository lives in a personal namespace: a user account on GitHub, Gitea or Forgejo, a personal namespace on GitLab, or a personal Bitbucket workspace. The path is the operator's key for a project, so it appears in operator logs (webhook events, webhook sync, run status), metric labels and span attributes, RenovateJob status, and the Valkey keys and S3 object keys of stored run logs. Retention of each follows the store it is in, as described below.

Restrict discovery to organization or group namespaces (see [Autodiscovery](../configuration/autodiscovery.md)) if personal repositories should not be processed at all.

## Operator logs

The operator writes structured logs to stdout. Their retention is set by your log pipeline.

| Data                                          | Where                                                                    | Level           | Purpose                       |
|-----------------------------------------------|--------------------------------------------------------------------------|-----------------|-------------------------------|
| Email                                         | Login completion, OIDC failures                                          | Info, Error     | Authentication audit          |
| GitHub login                                  | GitHub OAuth user lookup                                                 | Info, Error     | Authentication audit          |
| Email, remote address                         | Access denied, suspend set from the UI                                   | Info            | Authorization audit           |
| Email, remote address                         | Every resolved access decision                                           | V(1)            | Authorization debugging       |
| Remote address, user agent                    | Unauthenticated request rejected, GitHub login initiated                 | V(1), Info      | Security monitoring           |
| Email, group memberships                      | OIDC claims and userinfo groups, GitHub groups fetched                   | V(1)            | Group mapping debugging       |
| GitHub installation account                   | RenovateJob created for a GitHub App installation                        | Info            | Operations                    |

V(1) lines are only emitted at increased verbosity. Display names are not logged. The UI authenticates through OIDC or GitHub OAuth; users of GitLab, Gitea, Forgejo or Bitbucket sign in through OIDC. Incoming webhooks from every platform are logged with event, action and repository path only; sender identities are not logged.

## Sessions

With [Valkey](./valkey.md) configured, sessions are stored server-side; otherwise they live in the browser cookie. A session holds email, display name, username, group memberships and the identity provider's access and ID tokens. Server-side sessions are AES-GCM encrypted and expire after 24 hours; logout deletes them.

## Renovate run logs

With `config.logStorage.mode` set, the last run's Renovate output per project is kept for the UI. Renovate's output can contain personal data from the repositories it processes, for example commit author emails or assignee and reviewer names.

| Mode     | Retention                                                                                                    |
|----------|--------------------------------------------------------------------------------------------------------------|
| `memory` | Until the operator restarts                                                                                  |
| `valkey` | 30 days after the last write                                                                                 |
| `s3`     | Indefinite unless a bucket lifecycle rule expires it, see [S3 log retention](./s3.md#log-retention)          |

Each run overwrites the previous log of the same project in every mode.

## Renovate repository cache

The cache forwarded to executor jobs (Valkey or `s3.cachePrefix`) is written and expired by Renovate itself. It holds repository and pull request metadata. An S3 cache has no expiry unless a lifecycle rule covers its prefix.

## Kubernetes resources

- **RenovateJob status** stores parsed run results: PR branch names, numbers and titles, and up to 256 characters of each warning or error message Renovate logged. It is overwritten on every run and removed with the RenovateJob.
- **RenovateJob spec** may name users in access rules (`readerUsers`/`adminUsers`), and `GitHubAppInstallationSet` derives RenovateJob names from installation accounts, which are personal GitHub logins for user installations.
- **Executor pod logs** contain the same Renovate output as the run logs above. Their retention follows `config.jobTTLSecondsAfterFinished` and your cluster's log collection.

## Metrics and traces

Metric labels and span attributes carry namespace, RenovateJob and project names, where project names are repository paths (see [Repository paths](#repository-paths)). Their retention is set by your metrics and tracing backends.
