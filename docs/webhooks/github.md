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
   - **Pushes** (only with [`triggerOnPush`](#push-events-opt-in))
7. Ensure **Active** is checked

GitHub App `installation` and `installation_repositories` events sent to this endpoint go to `GitHubAppInstallationSet`s first, see [GitHub App — Every Installation](../platforms/github-app-installation-set.md#immediate-resync-via-webhook). An `installation` event also starts [repository lifecycle discovery](#repository-lifecycle-events-automatic-discovery), whether or not a set verifies it. An `installation_repositories` event no set verifies is answered `200` and ignored, since it may belong to another App sharing the URL.

The operator automatically finds the RenovateJob that owns the repository by matching the incoming repository name against discovered projects. If you have multiple RenovateJobs and want to target a specific one, append `namespace` and/or `job` as query parameters:

```text
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

### Push events (opt-in)

A RenovateJob can also run Renovate for a project when a commit is pushed to the project's
default branch, for example a direct push that changes `renovate.json` or a dependency file. Merged
pull requests already trigger a run; this covers pushes that do not go through a pull request.

Enable it per job with `triggerOnPush`:

```yaml
spec:
  webhook:
    enabled: true
    triggerOnPush: true
```

When webhook sync is enabled, the operator adds the **Pushes** event to the webhooks it manages
for this job, and removes it again when `triggerOnPush` is turned off. For manually configured
webhooks or GitHub Apps, subscribe to **Pushes** yourself.

Only pushes to the repository's default branch schedule a run. Pushes to other branches (including
Renovate's own `renovate/*` branches), tags, and branch deletions are ignored. Matching, the
`namespace` and `job` query parameters, and authentication work as for the checkbox events, but
only jobs with `triggerOnPush` are candidates. A push that no such job owns is acknowledged with
`200 OK`, so a subscription broader than the opted-in jobs does not produce failed deliveries.

`triggerOnPush` is supported for the `github` provider only. The API rejects it on a RenovateJob
or template whose provider is set to anything else, and the operator refuses a RenovateJob whose
effective spec (after merging its template) sets it without `provider.name: github`.
