# GitHub App using External Secrets Operator

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

**Secret Configuration for GitHub App**

```yaml
# 1. fetch GitHub App Private Key from Secrets Manager
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: github-app-pem
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: aws-secretsmanager # update your existing (Cluster)-SecretStore
  target:
    name: github-app-pem
  data:
    - secretKey: private_key_pem
      remoteRef:
        key: path/to/github-app # change to your secret path & name
        property: private_key_pem # change to your secret key
---
# 2. create a GithubAccessToken using GitHub App Credentials
apiVersion: generators.external-secrets.io/v1alpha1
kind: GithubAccessToken
metadata:
  name: github-auth-token
spec:
  appID: "123456" # change to your GitHub App ID
  installID: "1234567" # change to your GitHub App Installation ID
  repositories: # optional
    - my-gitops-repo
  permissions: # optional
    contents: read
  auth:
    privateKey:
      secretRef:
        name: github-app-pem # reference secret from step 1.
        key: private_key_pem
---
# 3. create a secret containing the required keys
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: github-auth-token
spec:
  refreshInterval: "30m0s"
  target:
    name: renovate-secret
    template: # template secret to use  GITHUB_COM_TOKEN
      type: Opaque
      engineVersion: v2
      data:
        GITHUB_COM_TOKEN:  "{{ .token }}"
        GITHUB_COM_USER: username # replace with your base64-encoded GitHub username
        RENOVATE_TOKEN: "{{ .token }}"
        # add additional fields you need, you can specify multiple sources in dataFrom to template more values in here
  dataFrom:
    - sourceRef:
        generatorRef:
          apiVersion: generators.external-secrets.io/v1alpha1
          kind: GithubAccessToken
          name: github-auth-token # reference from step 2.
```
