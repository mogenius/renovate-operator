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
3. Set the **URL** to: `https://your-webhook-host/webhook/v1/gitlab`
4. Add your webhook secret token as the bearer token in the **Secret token** field
5. Select the following triggers:
   - **Merge request events**
   - **Issue events**
6. Ensure **Enable SSL verification** is checked (if using HTTPS)
7. Click **Add webhook**

### Authentication methods

GitLab can authenticate webhook deliveries to the operator in two ways. Both validate against the token(s) in `authentication.secretRef` (comma-separate the secret value to allow several at once):

- **Secret token** (used in step 4 above) — GitLab sends the value verbatim in the `X-Gitlab-Token` header. Store the plain token in the secret.
- **Signing token** (recommended by GitLab) — GitLab signs each delivery with HMAC-SHA256 (the [Standard Webhooks](https://www.standardwebhooks.com/) scheme) and sends `Webhook-Id`, `Webhook-Timestamp` and `Webhook-Signature` headers. Store the `whsec_…` signing key that GitLab generates in the secret. The operator recomputes the signature over `id.timestamp.body` and rejects any delivery whose timestamp is more than 5 minutes from the current time, mitigating replay.

If a request carries both, the signature takes precedence over the secret token.

The operator automatically finds the RenovateJob that owns the project by matching the incoming project path against discovered projects. If you have multiple RenovateJobs and want to target a specific one, append `namespace` and/or `job` as query parameters:

```
https://your-webhook-host/webhook/v1/gitlab?namespace=renovate-operator&job=my-renovate-job
```

### Query parameters

| Parameter   | Required | Description                                                                 |
| :---------- | :------: | :-------------------------------------------------------------------------- |
| `namespace` |    no    | Kubernetes namespace to restrict the job search to.                         |
| `job`       |    no    | Name of the RenovateJob to restrict the job search to.                      |
