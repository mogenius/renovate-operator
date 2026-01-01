# GitHub Webhook Integration

The GitHub webhook integration allows the Renovate Operator to automatically trigger Renovate runs when specific actions occur on GitHub pull requests or issues. This is particularly useful for responding to Renovate's "rebase" checkbox interactions.

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
3. Set the **Payload URL** to: `https://your-webhook-host/webhook/v1/github?namespace=renovate-operator&job=my-renovate-job`
4. Set **Content type** to `application/json`
5. Add your webhook secret as the bearer token
6. Select individual events:
   - **Pull requests** (for PR checkbox interactions)
   - **Issues** (for Dependency Dashboard interactions)
7. Ensure **Active** is checked

### Query parameters

- `namespace`: The Kubernetes namespace of your RenovateJob
- `job`: The name of your RenovateJob resource
