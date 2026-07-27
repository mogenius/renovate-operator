# Bitbucket Webhook Integration

The Bitbucket webhook integration allows the Renovate Operator to automatically trigger Renovate runs when specific actions occur on Bitbucket Cloud pull requests. This is particularly useful for responding to Renovate's "rebase" checkbox interactions.

Webhooks can be added to each repository automatically by the operator — see [Automatic Webhook Sync](./sync.md). The rest of this page covers the Bitbucket-specific receiver and manual setup.

## Configuration

Configure the Bitbucket webhook in your RenovateJob:

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

### Bitbucket webhook setup

1. Go to your Bitbucket repository settings
2. Navigate to **Webhooks** → **Add webhook**
3. Set the **URL** to: `https://your-webhook-host/webhook/v1/bitbucket`
4. If using authentication, set the **Secret** to your webhook token — Bitbucket signs each delivery with it (`X-Hub-Signature`)
5. Select the following triggers:
   - **Pull Request: Updated**
   - **Pull Request: Merged**
   - **Pull Request: Declined**
6. Ensure the webhook is **Active** and save

The operator automatically finds the RenovateJob that owns the repository by matching the incoming repository name against discovered projects. If you have multiple RenovateJobs and want to target a specific one, append `namespace` and/or `job` as query parameters:

```
https://your-webhook-host/webhook/v1/bitbucket?namespace=renovate-operator&job=my-renovate-job
```

### Query parameters

| Parameter   | Required | Description                                                                 |
| :---------- | :------: | :-------------------------------------------------------------------------- |
| `namespace` |    no    | Kubernetes namespace to restrict the job search to.                         |
| `job`       |    no    | Name of the RenovateJob to restrict the job search to.                      |

## Supported events

Renovate does not support the Dependency Dashboard on Bitbucket Cloud, so unlike the other platforms only pull request events are processed:

- **pullrequest:updated**: When a Renovate PR description is edited and a checkbox is checked (e.g. the "rebase" checkbox)
- **pullrequest:fulfilled**: When a Renovate PR is merged
- **pullrequest:rejected**: When a Renovate PR is declined

Only pull requests containing Renovate's HTML comment markers (e.g. `<!-- rebase-check -->`) are processed; all other events are ignored.

## Authentication

Bitbucket Cloud does not send custom authorization headers. Instead, the hook's **secret** is used to sign each delivery with HMAC-SHA256, sent in the `X-Hub-Signature` header (`sha256=<hmac>`). The operator validates the signature against the tokens in `webhook.authentication.secretRef`. Automatic webhook sync configures the secret for you.
