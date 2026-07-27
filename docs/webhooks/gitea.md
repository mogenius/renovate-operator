# Gitea Webhook Integration

The Gitea webhook integration allows the Renovate Operator to automatically trigger Renovate runs when specific actions occur on Gitea pull requests or issues. This is particularly useful for responding to Renovate's "rebase" checkbox interactions and Dependency Dashboard updates.

Webhooks can be added to each repository automatically by the operator — see [Automatic Webhook Sync](./sync.md). The rest of this page covers the Gitea-specific receiver and manual setup.

## Configuration

Configure the Gitea webhook in your RenovateJob:

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

### Gitea webhook setup

1. Go to your Gitea repository settings
2. Navigate to **Webhooks** → **Add Webhook** → **Gitea**
3. Set the **Target URL** to: `https://your-webhook-host/webhook/v1/gitea`
4. Set **Content type** to `application/json`
5. If using authentication, set **Authorization Header** to `Bearer YOUR_TOKEN_HERE`
6. Select individual events:
   - **Pull requests** (for PR checkbox interactions, close, and reopen events)
   - **Issues** (for Dependency Dashboard interactions)
7. Ensure **Active** is checked

The operator automatically finds the RenovateJob that owns the repository by matching the incoming repository name against discovered projects. If you have multiple RenovateJobs and want to target a specific one, append `namespace` and/or `job` as query parameters:

```
https://your-webhook-host/webhook/v1/gitea?namespace=renovate-operator&job=my-renovate-job
```

### Query parameters

| Parameter   | Required | Description                                                                 |
| :---------- | :------: | :-------------------------------------------------------------------------- |
| `namespace` |    no    | Kubernetes namespace to restrict the job search to.                         |
| `job`       |    no    | Name of the RenovateJob to restrict the job search to.                      |

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
