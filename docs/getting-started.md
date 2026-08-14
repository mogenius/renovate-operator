# Getting Started

## Prerequisites

- Kubernetes ≥ 1.29
- Helm v4
- A [supported Git platform](./platforms/) with credentials ready

## Install

```sh
helm -n renovate-operator upgrade --install renovate-operator \
  oci://ghcr.io/mogenius/helm-charts/renovate-operator \
  --create-namespace --wait
```


## Create a credentials secret

The operator reads Renovate credentials from a Kubernetes Secret in the same namespace as the RenovateJob. The keys depend on your platform — here is GitHub with a PAT:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: renovate-secret
  namespace: renovate-operator
stringData:
  GITHUB_COM_USER: "<your-github-username>"
  GITHUB_COM_TOKEN: "<your-github-pat>"
  RENOVATE_TOKEN: "<your-github-pat>"
```

For other platforms see the [platform guides](./platforms/).

## Create your first RenovateJob

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate
  namespace: renovate-operator
spec:
  schedule: "0 2 * * *"       # run daily at 02:00
  secretRef: renovate-secret
  provider:
    name: github
  image: ghcr.io/renovatebot/renovate:latest
  parallelism: 3
  discoveryFilters:
    - "my-org/*"              # replace with your org or user
```

Apply it:

```sh
kubectl apply -f renovatejob.yaml
```

The operator runs a discovery job on the next cron tick, finds matching repositories, and queues them for Renovate. Open the UI to follow progress — the URL is whatever hostname you pointed at the operator.

## Next steps

| Topic                                   | Where                                                              |
| --------------------------------------- | ------------------------------------------------------------------ |
| Autodiscovery filters and topics        | [configuration/autodiscovery.md](./configuration/autodiscovery.md) |
| Authentication for the UI               | [configuration/auth.md](./configuration/auth.md)                   |
| Webhook-triggered runs                  | [webhooks/](./webhooks/)                                           |
| Self-service onboarding for repo owners | [self-service/overview.md](./self-service/overview.md)             |
| Prometheus metrics                      | [operations/metrics.md](./operations/metrics.md)                   |
| Security and policy engine              | [security/security.md](./security/security.md)                     |
