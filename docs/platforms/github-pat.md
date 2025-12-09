# GitHub using PAT

**RenovateJob Configuration for GitHub**

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-github
  namespace: renovate-operator
spec:
  discoveryFilter: ###GITHUB_USERNAME###/*
  extraEnv:
    - name: RENOVATE_PLATFORM
      value: github
    - name: RENOVATE_ENDPOINT
      value: https://api.github.com/
  image: renovate/renovate:41.43.3
  parallelism: 5
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
  schedule: 0 * * * *
  secretRef: renovate-secret
```

**Secret Configuration for GitHub**

```yaml
kind: Secret
apiVersion: v1
type: Opaque
metadata:
  name: renovate-secret
  namespace: renovate-operator
data:
  GITHUB_COM_USER: USERNAME_BASE64_ENCODED
  GITHUB_COM_TOKEN: GITHUB_TOKEN_VALUE_BASE64_ENCODED
  RENOVATE_TOKEN: RENOVATE_TOKEN_VALUE_BASE64_ENCODED
```

**Go to [GitHub Fine-grained PAT](https://github.com/settings/personal-access-tokens) and add a PAT with the following minimum permissions:**

![Example Screenshot of the renovate-operator UI.](/docs/github_permissions.png)
