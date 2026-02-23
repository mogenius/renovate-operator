# Forgejo Webhook Integration

The Forgejo webhook integration allows the Renovate Operator to automatically trigger Renovate runs when specific actions occur on Forgejo pull requests or issues.
This is particularly useful for responding to Renovate's "rebase" checkbox interactions and Dependency Dashboard updates.

## Configuration

Configure the Forgejo webhook in your RenovateJob:

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

### Forgejo webhook setup

1. Go to your Forgejo repository or organization settings
2. Navigate to **Webhooks** → **Add webhook** → **Forgejo**
3. Set the **Target URL** to: `https://your-webhook-host/webhook/v1/forgejo?namespace=renovate-operator&job=my-renovate-job`
4. Set **Content type** to `application/json`
5. Add your webhook secret in the **Secret** field (used for HMAC signature verification via the `X-Forgejo-Signature` header)
6. Select **Custom Events** and enable:
   - **Pull Request** (for PR checkbox interactions, close, and reopen events)
   - **Issues** (for Dependency Dashboard interactions)
7. Ensure **Active** is checked
8. Click **Add webhook**

### Query parameters

- `namespace`: The Kubernetes namespace of your RenovateJob
- `job`: The name of your RenovateJob resource

## Supported Events

### Issues (Dependency Dashboard)

The webhook triggers a Renovate run when a Dependency Dashboard issue is edited and a checkbox is checked.
Only issues containing Renovate's HTML comment markers (e.g., `<!-- manual job -->`, `<!-- rebase-all-open-prs -->`) are processed; all other issue events are ignored.

### Pull Requests

The webhook triggers a Renovate run for the following pull request actions:

- **edited**: When a Renovate PR body is edited and a checkbox is checked (e.g., the "rebase" checkbox)
- **closed**: When a Renovate PR is closed
- **reopened**: When a Renovate PR is reopened

Only pull requests containing Renovate's HTML comment markers (e.g., `<!-- rebase-check -->`, `<!--renovate-debug:...-->`) are processed; all other PR events are ignored.

## Differences from GitHub Webhook

Forgejo has a dedicated endpoint rather than reusing the GitHub handler because Forgejo fires issue webhook events for all mutations (title changes, label changes, assignee changes), not just body edits.
The Forgejo handler includes additional filtering to prevent false triggers from these non-body mutations.
