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
  discoveryFilters:
    - "Group1/*"
  image: renovate/renovate:43.104.11
  secretRef: "renovate-secret"
  parallelism: 1
  webhook:
    enabled: true
```

Trigger a run for a specific project using curl (URL-encode the project path):

```sh
curl -X POST \
  "http://webhook.example.com/webhook/v1/schedule?project=yourOrg%2FyourProject"
```

The operator will find the RenovateJob that has this project in its status and schedule it.
You can also scope the lookup with optional `namespace` and `job` parameters:

```sh
curl -X POST \
  "http://webhook.example.com/webhook/v1/schedule?job=renovate-unsecure&namespace=renovate-operator&project=yourOrg%2FyourProject"
```

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
  discoveryFilters:
    - "Group1/*"
  image: renovate/renovate:43.104.11
  secretRef: "renovate-secret"
  parallelism: 1
  webhook:
    enabled: true
    authentication:
      enabled: true
      secretRef:
        name: renovate-api
        key: token
---
apiVersion: v1
kind: Secret
metadata:
  name: renovate-api
  namespace: renovate-operator
  labels:
    renovate-operator.mogenius.com/allow-ref: "true"
stringData:
  token: your-webhook-token
```

Call the webhook passing the token in the Authorization header:

```sh
curl -X POST \
  "https://webhook.example.com/webhook/v1/schedule?project=yourOrg%2FyourProject" \
  -H "Authorization: Bearer YOUR_TOKEN_HERE"
```

## Trigger every project of a job

Add `all=true` to schedule every project of one RenovateJob in a single call, the same way the
UI's "Trigger All" button does. A typical caller is the release pipeline of a shared library: once
the new version is published, it asks Renovate to look at every repository that may depend on it.

```sh
curl -X POST \
  "https://webhook.example.com/webhook/v1/schedule?all=true&job=renovate-secure&namespace=renovate-operator" \
  -H "Authorization: Bearer YOUR_TOKEN_HERE"
```

- `job` and `namespace` are required and name the RenovateJob exactly; `project` must not be set.
- The job must have webhook authentication enabled. On a job without it, the request is rejected
  with `401 Unauthorized`, so an unauthenticated endpoint cannot be used to start every run of a job.
- Every project that is not currently running is scheduled at the same priority as a single
  `/schedule` call. A project that is running is left alone, as it is for a single call; its next
  scheduled run picks up the change.
- A `200 OK` means the projects were queued. They start as the job's `parallelism` allows, and
  not at all while the job is suspended.
- On `500 Internal Server Error` some projects may already be queued. Repeating the request is safe.

## Job resolution

All webhook endpoints (`/schedule`, `/github`, `/gitlab`, `/forgejo`, `/gitea`) use the same resolution logic to find the target RenovateJob:

1. List all RenovateJobs with `webhook.enabled: true`.
2. Filter by `namespace` and `job` query parameters if provided.
3. Filter to jobs that have the incoming project in their discovered project list.
4. If authentication is enabled on a job, validate the request credential against it.
5. The first job that passes authentication (or has authentication disabled) is used.

`/schedule?all=true` does not search: it uses the job named by `namespace` and `job`, as described in
[Trigger every project of a job](#trigger-every-project-of-a-job).

If multiple jobs match, authentication is tried on each — the first to authenticate wins. If none authenticate, the request is rejected with `401 Unauthorized`. If no job contains the project at all, the request is rejected with `404 Not Found`.

Providing `namespace` and `job` narrows the search and is useful when multiple RenovateJobs could own the same project.

## Notes and best practices

- Prefer HTTPS for the webhook ingress and restrict access to trusted networks when possible.
- Use single-purpose tokens and rotate them periodically.
- The `project` parameter should be URL-encoded (for example `group/repo` becomes `group%2Frepo`).
- Projects must have been discovered at least once before webhooks can trigger them — the operator matches the incoming project name against the job's discovered project list.
- A trigger for a project that is already running is not lost: the project is queued again, at the trigger's priority, as soon as that run ends. This covers the merge events Renovate's own automerge sends while it is still running. Several triggers during one run add a single follow-up run, and cancelling the run drops it. A one-off run with its own options (custom base branch, dry run, debug) requested from the UI during a run is still ignored.
