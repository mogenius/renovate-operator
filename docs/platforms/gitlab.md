# GitLab using PAT

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-group1
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  discoveryFilters: 
    - "Group1/*"
  image: renovate/renovate:43.104.1 # renovate
  secretRef: "renovate-secret"
  provider:
    name: gitlab
    endpoint: ""  # optional, defaults to https://gitlab.com/api/v4
  extraEnv: []
    # - name: ENV_NAME
    #   value: "EV_VALUE"
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

**Secret Configuration for GitLab**

```yaml
kind: Secret
apiVersion: v1
type: Opaque
metadata:
  name: renovate-secret
  namespace: renovate-operator
data:
  RENOVATE_TOKEN: GITLAB_RENOVATE_TOKEN_VALUE_BASE64_ENCODED
```
