# Image Pull Secrets

The Renovate Operator supports configuring image pull secrets at two levels: globally for all jobs via the operator configuration, and per-job via the `RenovateJob` spec. Both levels are merged and applied to every pod created by the operator.

## Important: Namespace Considerations

RenovateJobs can be deployed in **different namespaces** than the operator itself. Image pull secrets are namespace-scoped Kubernetes resources, which means each secret must exist in the **same namespace as the RenovateJob**, not the operator's namespace.

If your registry requires authentication, make sure to create the image pull secret in every namespace where you deploy `RenovateJob` resources.

## Global Configuration (Operator-level)

You can configure default image pull secrets via the Helm chart. These are passed to the operator as an environment variable and applied to **all** RenovateJob pods across all namespaces.

```yaml
# values.yaml
image:
  imagePullSecrets:
    - name: my-registry-secret
```

> **Note:** The referenced secret must exist in the namespace of each `RenovateJob`, not only in the operator's namespace.

## Per-Job Configuration (RenovateJob.spec.imagePullSecrets)

You can also configure image pull secrets on individual `RenovateJob` resources. This is useful when different jobs use images from different registries or need different credentials.

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate
  namespace: my-namespace
spec:
  schedule: "0 * * * *"
  image: my-private-registry.example.com/renovate/renovate:latest
  secretRef: "renovate-secret"
  parallelism: 1
  imagePullSecrets:
    - name: my-registry-secret
```

## Combining Both Levels

When both global and per-job image pull secrets are configured, they are **merged** and both apply to the resulting pods. This allows you to set a default registry secret globally while adding job-specific secrets as needed.

## Example: Private Registry Setup

1. Create the image pull secret in the RenovateJob's namespace:

```bash
kubectl create secret docker-registry my-registry-secret \
  --docker-server=my-private-registry.example.com \
  --docker-username=myuser \
  --docker-password=mypassword \
  --namespace=my-namespace
```

2. Reference it in your `RenovateJob`:

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate
  namespace: my-namespace
spec:
  schedule: "0 * * * *"
  image: my-private-registry.example.com/renovate/renovate:latest
  secretRef: "renovate-secret"
  parallelism: 1
  imagePullSecrets:
    - name: my-registry-secret
```
