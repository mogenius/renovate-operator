# GitHub App using native Implementation


**Secret Configuration for GitHub App credentials**
```yaml
kind: Secret
apiVersion: v1
type: Opaque
metadata:
  name: github-app-credentials
  namespace: renovate-operator
data:
  PEM: PEM_BASE64_ENCODED
  APP_ID: GITHUB_APP_ID_BASE64_ENCODED
  INSTALL_ID: GITHUB_INSTALL_ID_BASE64_ENCODED
```

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
  githubAppReference:
    secretName: github-app-credentials
    appIdSecretKey: APP_ID
    installationIdSecretKey: INSTALL_ID
    pemSecretKey: PEM
```

**This will automatically create and inject the following secret**
```yaml
kind: Secret
apiVersion: v1
type: Opaque
metadata:
  name: renovate-github-github-app-xxxxxxxx
  namespace: renovate-operator
data:
  GITHUB_COM_USER: USERNAME_BASE64_ENCODED
  GITHUB_COM_TOKEN: GITHUB_TOKEN_VALUE_BASE64_ENCODED
  RENOVATE_TOKEN: RENOVATE_TOKEN_VALUE_BASE64_ENCODED
```
