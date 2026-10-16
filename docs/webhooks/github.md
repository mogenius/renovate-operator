# GitHub Webhook Integration

The GitHub webhook integration allows the Renovate Operator to automatically trigger Renovate runs when specific actions occur on GitHub pull requests or issues. This is particularly useful for responding to Renovate's "rebase" checkbox interactions.

Webhooks can be added to each repository automatically by the operator — see [Automatic Webhook Sync](./sync.md). The rest of this page covers the GitHub-specific receiver and manual setup.

## Configuration

Configure the GitHub webhook in your RenovateJob:

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
```

### GitHub webhook setup

1. Go to your GitHub repository or organization settings
2. Navigate to **Webhooks** → **Add webhook**
3. Set the **Payload URL** to: `https://your-webhook-host/webhook/v1/github`
4. Set **Content type** to `application/json`
5. Add your webhook secret as the bearer token
6. Select individual events:
   - **Pull requests** (for PR checkbox interactions)
   - **Issues** (for Dependency Dashboard interactions)
7. Ensure **Active** is checked

The operator automatically finds the RenovateJob that owns the repository by matching the incoming repository name against discovered projects. If you have multiple RenovateJobs and want to target a specific one, append `namespace` and/or `job` as query parameters:

```
https://your-webhook-host/webhook/v1/github?namespace=renovate-operator&job=my-renovate-job
```

### Query parameters

| Parameter   | Required | Description                                                                 |
| :---------- | :------: | :-------------------------------------------------------------------------- |
| `namespace` |    no    | Kubernetes namespace to restrict the job search to.                         |
| `job`       |    no    | Name of the RenovateJob to restrict the job search to.                      |

### Repository lifecycle events (automatic discovery)

Besides pull request and issue interactions, the endpoint also responds to repository lifecycle
events and starts a discovery run for the matching RenovateJob(s). Add these to the webhook's
event subscriptions:

- **Repositories** — `created` and `deleted` actions. A repository appearing or disappearing
  changes what a job's autodiscovery would return, so a fresh discovery reconciles the project
  list without waiting for the next scheduled run.
- **Installation** (GitHub Apps) — the `created` action, sent when the App is installed.

Matching differs from the checkbox events: a newly created repository is not yet part of any
job's project status, so the operator matches on discovery intent instead. A job is a candidate
when its webhook is enabled, its provider is `github`, and its `spec.discoveryFilters` match the
repository's full name. Installation payloads match every embedded repository; payloads without
a repository list fall back to matching the account as `<account>/*`. Jobs without
`discoveryFilters` match every repository. The optional `namespace` and `job` query parameters
and the configured webhook authentication apply as usual.

The endpoint answers `202 Accepted` and lists the RenovateJobs whose discovery was started.
Events that match no job, a job whose discovery is already running, or a suspended job are
acknowledged with `200 OK` so GitHub does not mark the delivery as failed and retry it.
