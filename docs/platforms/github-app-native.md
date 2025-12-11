# GitHub App — Native Integration

> [!IMPORTANT]
> GitHub App native integration is in Beta.

The native approach lets the operator manage the GitHub App token lifecycle directly — no External Secrets Operator required. The operator reads your App credentials from a Kubernetes Secret, generates a short-lived installation access token via the GitHub API, and injects it as `RENOVATE_TOKEN` into each Renovate Job automatically.

If you haven't created your GitHub App yet: [GitHub App Setup](./github-app-setup.md)

Looking for the ESO-based approach instead? [GitHub App — External Secrets Operator](./github-app-eso.md)

---

## 1. Create the credentials Secret

Store your GitHub App credentials in a Kubernetes Secret:

```yaml
kind: Secret
apiVersion: v1
type: Opaque
metadata:
  name: github-app-credentials
  namespace: renovate-operator
data:
  PEM: <base64-encoded private key>
  APP_ID: <base64-encoded App ID>
  INSTALL_ID: <base64-encoded Installation ID>
```

## 2. Create the RenovateJob

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-github
  namespace: renovate-operator
spec:
  discoveryFilters:
    - <your-github-org>/*
  provider:
    name: github
    endpoint: ""  # optional, defaults to https://api.github.com
  image: renovate/renovate:43.104.1
  parallelism: 5
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
  schedule: 0 * * * *
  githubAppReference:
    secretName: github-app-credentials
    appIdSecretKey: APP_ID
    installationIdSecretKey: INSTALL_ID
    pemSecretKey: PEM
```

> **Note:** `secretRef` is not required when using `githubAppReference` — the operator generates the token and injects it automatically. You can still add a `secretRef` if you need additional environment variables (e.g. `RENOVATE_EXTRA_FLAGS`).

## How it works

On each reconcile loop the operator:

1. Reads `APP_ID`, `INSTALL_ID`, and `PEM` from the referenced Secret
2. Signs a JWT and exchanges it with the GitHub API for a short-lived installation access token
3. Writes the token as `RENOVATE_TOKEN` into an auto-managed Secret named `<job-name>-github-app-<hash>`
4. Mounts that Secret into every discovery and renovate Job via `envFrom`

The token is refreshed automatically before it expires (>30 min TTL check on each reconcile).
