# Generic Platform Configuration

This guide shows how to configure the Renovate Operator for any platform supported by Renovate. The operator works with all [Renovate platforms](https://docs.renovatebot.com/modules/platform/).

## Basic Configuration

Replace the platform-specific values with those for your platform:

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-myplatform
  namespace: renovate-operator
spec:
  schedule: "0 * * * *" # Hourly
  discoveryFilter: "MyOrg/*" # Optional: filter discovered repositories
  image: renovate/renovate:latest # renovate
  secretRef: "renovate-secret"
  extraEnv:
    - name: RENOVATE_ENDPOINT
      value: "https://your-platform.example.com" # Your platform API endpoint
    - name: RENOVATE_PLATFORM
      value: "your-platform" # Platform type: github, gitlab, bitbucket, azure, gitea, etc.
    # Add any additional environment variables here
  parallelism: 1
```

## Secret Configuration

Create a secret with your platform authentication token:

```yaml
kind: Secret
apiVersion: v1
type: Opaque
metadata:
  name: renovate-secret
  namespace: renovate-operator
data:
  RENOVATE_TOKEN: BASE64_ENCODED_TOKEN_VALUE
```
