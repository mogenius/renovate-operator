#####

<div align="center">
    <img src="src/static/favicon.ico" width="90" />
    <h1 align="center">Renovate Operator</h1>
</div>

[Renovate][1] is one of, if not the leading tool for automated dependency updates.

With tools like [Mend Renovate][2] you can easily use renovate for free.

But what if you want to run renovate on your own hardware? What if you are running a private GitLab instance? Or just want better control over the scheduling of your renovate executions?

If you are already running Kubernetes, this project might be for you.

### How it works

1. At the defined time of your schedule, a renovate discovery job is started
2. After the discovery finished, you will be able to see all your discovered projects in the UI
3. All projects are now being set to be scheduled
4. Every 10 seconds the operator checks for schedules projects and starts a new renovate job
5. Only as many jobs as defined in `spec.parallelism` are getting executed at the same time

![Example Screenshot of the renovate-operator UI.](/docs/example.png)

## Installation

### Helm

```sh
helm repo add mogenius https://helm.mogenius.com/public --force-update
helm -n renovate-operator upgrade --install renovate-operator mogenius/renovate-operator --create-namespace --wait
```

## Documentation
- [Webhook API](./docs/webhook.md)


## Examples

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-group1
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  discoveryFilter: "Group1/*"
  image: renovate/renovate:41.43.3 # renovate
  secretRef: "renovate-secret"
  extraEnv:
    - name: RENOVATE_ENDPOINT
      value: "https://gitlab.company.com"
    - name: RENOVATE_PLATFORM
      value: "gitlab"
    - name: RENOVATE_ALLOW_PLUGINS
      value: "true"
  parallelism: 1
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "1Gi"
  nodeSelector:
    kubernetes.io/hostname: server-1
```

## Contributing
<a href="https://github.com/mogenius/renovate-operator/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=mogenius/renovate-operator" />
</a>

Made with [contrib.rocks](https://contrib.rocks).

## Development

### Running Tests

Run the test suite:
```sh
go test -v ./...
```

### Code Quality

Run golangci-lint locally:
```sh
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
golangci-lint run
```

### Generate CRDs

```sh
controller-gen crd paths=./src/... output:crd:dir=charts/renovate-operator/crds
```

[1]: https://github.com/renovatebot/renovate
[2]: https://docs.mend.io/renovate/latest/
