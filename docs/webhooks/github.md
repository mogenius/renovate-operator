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
