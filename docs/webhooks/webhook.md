# Webhook API — triggering Renovate runs

The Webhook API allows external systems (for example CI/CD pipelines, Git hooks, or automation tools)
to trigger Renovate runs managed by this operator. Use it to start a run for a specific project on demand.

Security note: treat the webhook endpoint as sensitive. Prefer HTTPS, restrict the ingress host,
and use authentication (bearer tokens) in production.

## Prerequisites

Ensure the webhook feature is enabled when installing the operator via Helm. Example values:

```yaml
webhook:
  enabled: true
  ingress:
    enabled: true
    host: webhook.example.com
```

Make sure the ingress host resolves and (ideally) serves TLS.

## Enable the webhook on a RenovateJob

Add the `webhook` section to the `RenovateJob` spec to enable webhook-triggered runs for that job.

### Example: enable webhook (no authentication)

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-unsecure
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  discoveryFilter: "Group1/*"
  image: renovate/renovate:41.43.3
  secretRef: "renovate-secret"
  parallelism: 1
  webhook:
    enabled: true
```

Trigger a run for a specific project using curl (URL-encode the project path):

```sh
curl -X POST \
  "http://webhook.example.com/webhook/v1/schedule?job=renovate-unsecure&namespace=renovate-operator&project=yourOrg%2FyourProject"
```

The `job` query parameter must match the `metadata.name` of the `RenovateJob` resource.

### Example: enable webhook with authentication

Create a secret containing a bearer token:

> [!IMPORTANT]
> If you want to create multiple tokens for one RenovateJob separate them using a `,`
> kubectl create secret generic -n renovate-operator renovate-api --from-literal=token=TOKEN1,TOKEN2

```sh
kubectl create secret generic -n renovate-operator renovate-api --from-literal=token=YOUR_TOKEN_HERE
```

Reference the secret in the `RenovateJob`:

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-secure
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  discoveryFilter: "Group1/*"
  image: renovate/renovate:41.43.3
  secretRef: "renovate-secret"
  parallelism: 1
  webhook:
    enabled: true
    authentication:
      enabled: true
      secretRef:
        name: renovate-api
        key: token
```

Call the webhook passing the token in the Authorization header:

```sh
curl -X POST \
  "https://webhook.example.com/webhook/v1/schedule?job=renovate-secure&namespace=renovate-operator&project=yourOrg%2FyourProject" \
  -H "Authorization: Bearer YOUR_TOKEN_HERE"
```

## Notes and best practices

- Prefer HTTPS for the webhook ingress and restrict access to trusted networks when possible.
- Use single-purpose tokens and rotate them periodically.
- The `project` parameter should be URL-encoded (for example `group/repo` becomes `group%2Frepo`).
