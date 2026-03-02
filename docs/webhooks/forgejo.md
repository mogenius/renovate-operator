# Forgejo Webhook Integration

The Forgejo webhook integration allows the Renovate Operator to automatically trigger Renovate runs when specific actions occur on Forgejo pull requests or issues.
This is particularly useful for responding to Renovate's "rebase" checkbox interactions and Dependency Dashboard updates.

## Background

On platforms like GitHub, "GitHub Apps" can install webhooks across an entire organization automatically.
Forgejo does not have an equivalent mechanism — webhooks must be added to each repository individually.
This makes manual webhook setup impractical when managing many repositories.

The operator solves this with **automatic webhook sync**: after each autodiscovery run, it queries the Forgejo API for candidate repos, checks that the sync user has admin permission (required for managing webhooks), and creates or removes webhooks accordingly.
Repos are discovered using a configurable topic search, but the sync only acts on repos where all prerequisites are met — the topic is just the discovery filter.

## Manual webhook setup

If you prefer to add webhooks to individual repos yourself (or only have a few repos), skip the sync configuration and set up webhooks manually:

1. Go to your Forgejo repository settings
2. Navigate to **Webhooks** -> **Add webhook** -> **Forgejo**
3. Set the **Target URL** to: `https://your-webhook-host/webhook/v1/forgejo?namespace=renovate-operator&job=my-renovate-job`
4. Set **Content type** to `application/json`
5. If using authentication, set **Authorization Header** to `Bearer YOUR_TOKEN_HERE`
6. Select individual events:
   - **Pull requests** (for PR checkbox interactions, close, and reopen events)
   - **Issues** (for Dependency Dashboard interactions)
7. Ensure **Active** is checked

## Supported events

### Issues (Dependency Dashboard)

The webhook triggers a Renovate run when a Dependency Dashboard issue is edited and a checkbox is checked.
Only issues containing Renovate's HTML comment markers (e.g., `<!-- manual job -->`, `<!-- rebase-all-open-prs -->`) are processed; all other issue events are ignored.

### Pull Requests

The webhook triggers a Renovate run for the following pull request actions:

- **edited**: When a Renovate PR body is edited and a checkbox is checked (e.g., the "rebase" checkbox)
- **closed**: When a Renovate PR is closed
- **reopened**: When a Renovate PR is reopened

Only pull requests containing Renovate's HTML comment markers (e.g., `<!-- rebase-check -->`) are processed; all other PR events are ignored.

## Automatic webhook sync

Automatic sync removes the need to add webhooks to every repo by hand.
After each autodiscovery cycle, the operator queries the Forgejo API for candidate repos, verifies the sync user has admin permission, and ensures a webhook exists on each eligible repo.

When a repo is no longer eligible (e.g. the topic was removed, or the sync user lost admin access), the operator removes the webhook.

Sync runs as part of the regular discovery schedule — there is no separate schedule to configure.

### Configuration

Add the `forgejo.sync` section to the `webhook` block in your RenovateJob:

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: my-renovate-job
  namespace: renovate-operator
spec:
  # ... other configuration ...
  webhook:
    enabled: true
    authentication:
      enabled: true
      secretRef:
        name: renovate-webhook-token
        key: token
    forgejo:
      sync:
        enabled: true
        forgejoURL: https://forgejo.example.com
        webhookURL: https://your-webhook-host/webhook/v1/forgejo
        topic: renovate
        tokenSecretRef:
          name: forgejo-api-token
          key: token
        authTokenSecretRef:
          name: renovate-webhook-token
          key: token
```

### Sync fields

| Field                | Required | Default                        | Description                                                                                                                                                                                                                                     |
| :------------------- | :------: | :----------------------------- | :---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `enabled`            |   yes    |                                | Enable or disable automatic webhook sync.                                                                                                                                                                                                       |
| `forgejoURL`         |   yes    |                                | Base URL of the Forgejo instance (e.g. `https://forgejo.example.com`).                                                                                                                                                                          |
| `webhookURL`         |   yes    |                                | Externally reachable URL of the operator's Forgejo webhook endpoint. The operator appends `?namespace=...&job=...` query parameters automatically.                                                                                              |
| `topic`              |    no    | value of `spec.discoverTopics` | Forgejo topic used as the search filter for discovering candidate repos.                                                                                                                                                                        |
| `events`             |    no    | `["issues", "pull_request"]`   | Forgejo webhook event types to subscribe to.                                                                                                                                                                                                    |
| `tokenSecretRef`     |   yes    |                                | Reference to a Kubernetes secret containing a Forgejo API token. This token must belong to a user with admin access on the target repos.                                                                                                        |
| `authTokenSecretRef` |    no    |                                | Reference to a Kubernetes secret containing the bearer token placed in each webhook's `authorization_header`. This authenticates webhook deliveries back to the operator. Typically the same secret used in `webhook.authentication.secretRef`. |

### How sync works

Webhook sync runs automatically at the end of each autodiscovery cycle (controlled by `spec.schedule`).

1. The operator searches for repos by topic using `GET /api/v1/repos/search?topic=true&q={topic}`.
2. Repos where the sync user has **admin permission** are eligible. Repos without admin access are skipped with a log message.
3. For each eligible repo, the operator checks for an existing webhook matching the configured URL. If missing, it creates one.
4. Repos that previously had a webhook but no longer have the topic are cleaned up — the operator deletes the webhook it created.
5. If the sync user loses admin access on a repo, the operator cannot delete the webhook. This is logged as an error so you can remove the stale webhook manually.

### Permissions

The Forgejo API token must belong to a user (or bot account) with **admin permission** on every repo you want to manage.
Repos where the user only has read or write access are silently skipped — no error is raised, but a log line is emitted so you can audit which repos were skipped.

### Query parameters

- `namespace`: The Kubernetes namespace of your RenovateJob (appended automatically by sync)
- `job`: The name of your RenovateJob resource (appended automatically by sync)

## Differences from GitHub webhook

Forgejo has a dedicated endpoint rather than reusing the GitHub handler because Forgejo fires issue webhook events for all mutations (title changes, label changes, assignee changes), not just body edits.
The Forgejo handler includes additional filtering to prevent false triggers from these non-body mutations.
