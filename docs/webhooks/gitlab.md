# GitLab Webhook Integration

The GitLab webhook integration allows the Renovate Operator to automatically trigger Renovate runs when specific actions occur on GitLab merge requests or issues. This is particularly useful for responding to Renovate's "rebase" checkbox interactions.

## Configuration

Configure the GitLab webhook in your RenovateJob:

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

### GitLab webhook setup

1. Go to your GitLab project or group settings
2. Navigate to **Webhooks** → **Add new webhook**
3. Set the **URL** to: `https://your-webhook-host/webhook/v1/gitlab?namespace=renovate-operator&job=my-renovate-job`
4. Add your webhook secret token as the bearer token in the **Secret token** field
5. Select the following triggers:
   - **Merge request events**
   - **Issue events**
6. Ensure **Enable SSL verification** is checked (if using HTTPS)
7. Click **Add webhook**

### Query parameters

- `namespace`: The Kubernetes namespace of your RenovateJob
- `job`: The name of your RenovateJob resource
