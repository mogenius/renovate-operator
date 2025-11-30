# Extra Volumes

The Renovate Operator allows you to mount additional volumes in the Renovate job pods. This is useful for providing custom configuration files, credentials, or other resources that Renovate needs to access.

## Default Volume

By default, the operator automatically creates and mounts a volume named `tmp` to `/tmp` in all Renovate job pods. This temporary volume is used by Renovate for its working directory and cache.

## Adding Extra Volumes

You can add additional volumes and volume mounts to your `RenovateJob` using the `extraVolumes` and `extraVolumeMounts` fields in the spec.

### Example: Mounting a ConfigMap with Renovate Configuration

This example shows how to mount a ConfigMap containing a custom Renovate configuration file:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: renovate-config
  namespace: renovate-operator
data:
  config.js: |
    module.exports = {
      platform: 'gitlab',
      gitAuthor: 'Renovate Bot <renovate@example.com>',
      onboarding: false,
      requireConfig: 'optional',
      // Add your custom Renovate configuration here
    };
---
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-with-config
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  image: renovate/renovate:41.43.3
  secretRef: "renovate-secret"
  parallelism: 1
  extraVolumes:
    - name: renovate-config
      configMap:
        name: renovate-config
  extraVolumeMounts:
    - name: renovate-config
      mountPath: /config
  extraEnv:
    - name: RENOVATE_CONFIG_FILE
      value: /config/config.js
```

## Important Notes

- The volume name in `extraVolumes` must match the name referenced in `extraVolumeMounts`.
- Both discovery jobs and renovate execution jobs will have the same extra volumes mounted.
- The default `tmp` volume is always present and cannot be overridden.
- Make sure the `mountPath` in your volume mount does not conflict with existing paths used by Renovate.

## Use Cases

Common use cases for extra volumes include:

- **Custom Renovate configuration files** — mount a `config.js` or JSON config file
- **SSH keys** — provide authentication for private repositories
- **Custom CA certificates** — trust additional certificate authorities
- **Plugin or preset files** — provide custom Renovate plugins or shared presets
- **Cache directories** — speed up renovate runs with persistent caching

## Troubleshooting

If your volume mount is not working:

1. Check that the ConfigMap/Secret exists in the same namespace as the RenovateJob.
2. Verify the volume name matches between `extraVolumes` and `extraVolumeMounts`.
3. Check the operator logs for any errors related to pod creation.
4. Use `kubectl describe pod <renovate-pod-name>` to see volume mount details.
5. Ensure file permissions are correct when mounting secrets (use `defaultMode`).
