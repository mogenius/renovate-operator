# Custom Renovate Configuration

`spec.renovateConfig` provides a Renovate configuration file to every discovery and
executor pod of a `RenovateJob`. The operator mounts the file read-only under
`/etc/renovate/` and sets `RENOVATE_CONFIG_FILE` to it.

Exactly one of `inline` or `configMapRef` must be set.

## Inline Configuration

The operator writes the configuration into a ConfigMap it owns. The ConfigMap is
updated when the spec changes and garbage-collected with the `RenovateJob`.

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-with-config
  namespace: renovate-operator
spec:
  # ... other configuration ...
  renovateConfig:
    inline: |
      module.exports = {
        gitAuthor: 'Renovate Bot <renovate@example.com>',
        onboarding: false,
        requireConfig: 'optional',
      };
```

`fileName` (default `config.js`) controls the mounted file name; the extension tells
Renovate how to parse it, so use `config.json` for plain JSON:

```yaml
spec:
  renovateConfig:
    fileName: config.json
    inline: |
      {
        "onboarding": false
      }
```

## Referencing an Existing ConfigMap

Point at a key in a ConfigMap you manage yourself (e.g. deployed via Helm, Kustomize
or GitOps). The file is mounted under the key's name, so the key needs a supported
extension (`.js`, `.cjs`, `.mjs`, `.json`, `.json5`):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: renovate-config
  namespace: renovate-operator
data:
  config.js: |
    module.exports = {
      onboarding: false,
    };
---
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-with-config
  namespace: renovate-operator
spec:
  # ... other configuration ...
  renovateConfig:
    configMapRef:
      name: renovate-config
      key: config.js
```

The ConfigMap must live in the same namespace as the `RenovateJob`.

## Notes

- A `RENOVATE_CONFIG_FILE` entry in `spec.extraEnv` still has precedence over the generated
  one, so existing setups keep working unchanged.
- The config volume is named `renovate-config-file`. An `extraVolumes` entry with
  the same name would conflict.
- Don't put credentials in the config file; ConfigMaps are not Secrets!
