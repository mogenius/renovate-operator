# Valkey

The operator uses Valkey as an optional shared datastore. When configured, it enables:

- **Session storage** — persists authenticated sessions across operator replicas and restarts (required for multi-replica deployments)
- **Renovate cache** — forwards a Redis-compatible cache URL to each Renovate executor job, allowing Renovate to reuse dependency metadata between runs
- **Log storage** — retains the last run's log output per project, queryable through the UI (alternative to the in-memory store)

Without Valkey, sessions are stored in cookies, no cache is forwarded to jobs, and log storage falls back to `memory` or `disabled`.

## Database assignment

The operator uses three logical Valkey databases, each serving a distinct purpose:

| Usage                | DB (host-based) | Purpose                                       |
|----------------------|-----------------|-----------------------------------------------|
| `UsageSessionStore`  | 0               | Session encryption store                      |
| `UsageRenovateCache` | 1               | Renovate job cache forwarded to executor jobs |
| `UsageRenovateLogs`  | 2               | Log storage for completed Renovate runs       |

### Predefined URL with explicit database

When `VALKEY_URL` includes an explicit database index, the operator treats that index as the **base** and adds each usage as an offset. This allows pointing at any starting database without conflicting with other workloads on the same Valkey instance.

Example — `VALKEY_URL=redis://valkey:6379/5`:

| Usage                | Resulting DB |
|----------------------|--------------|
| `UsageSessionStore`  | 5 (5 + 0)    |
| `UsageRenovateCache` | 6 (5 + 1)    |
| `UsageRenovateLogs`  | 7 (5 + 2)    |

If `VALKEY_URL` has no database component (e.g. `redis://valkey:6379`), the base is `0` and databases `0`, `1`, `2` are used — the same as the host-based default.

## Configuration

Valkey is configured via environment variables (or the equivalent Helm values):

| Environment variable           | Helm value                  | Default    | Description                                                                                                       |
|--------------------------------|-----------------------------|------------|-------------------------------------------------------------------------------------------------------------------|
| `VALKEY_URL`                   | —                           | `""`       | Full connection URL, e.g. `redis://user:password@valkey:6379/0`. Takes precedence over all host-based variables.  |
| `VALKEY_HOST`                  | —                           | `""`       | Valkey hostname. Used when `VALKEY_URL` is not set.                                                               |
| `VALKEY_PORT`                  | —                           | `6379`     | Valkey port. Used when `VALKEY_URL` is not set.                                                                   |
| `VALKEY_USERNAME`              | —                           | `""`       | Valkey ACL username. Used when `VALKEY_URL` is not set. Omit to authenticate as the `default` user.               |
| `VALKEY_PASSWORD`              | —                           | `""`       | Valkey password. Used when `VALKEY_URL` is not set.                                                               |
| `VALKEY_TLS`                   | —                           | `false`    | Connect with TLS (`rediss://`). Used when `VALKEY_URL` is not set; a URL carries its own scheme.                  |
| `VALKEY_FORWARD_CACHE_TO_JOBS` | `config.forwardCacheToJobs` | `true`     | Forward the Renovate cache URL to executor jobs. Requires Valkey to be configured.                                |
| `LOG_STORE_MODE`               | `config.logStorage.mode`    | `disabled` | Log storage backend: `disabled`, `memory`, `valkey`, or `s3` (see [S3 Object Storage](./s3.md)).                  |

The server certificate of a TLS-enabled Valkey must chain to a publicly trusted CA — there is currently no option to supply a custom CA bundle. Note that the executor jobs receive the same URL, so Renovate's containers must be able to verify the certificate too.

## Deploying with the bundled Helm chart

The chart includes an optional single-node Valkey instance (via the [official Valkey Helm chart](https://github.com/valkey-io/valkey-helm)):

```yaml
valkey:
  enabled: true
```

To use an external Valkey instance, provide the URL via an existing Kubernetes secret:

```yaml
valkey:
  existingSecret: "my-valkey-secret"
  existingSecretKey: "valkey-url"  # default key name
```

The secret must contain the full connection URL under the configured key:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-valkey-secret
stringData:
  valkey-url: "redis://:yourpassword@valkey.example.com:6379/0"
```

### External Valkey with credentials in separate secret fields

When an external Valkey's credentials arrive as separate secret fields rather than a ready-made URL — for example the [Aiven operator](https://aiven.github.io/aiven-operator/)'s `ServiceUser` secret with its `SERVICEUSER_USERNAME` and `SERVICEUSER_PASSWORD` keys — use the host-based variables via the chart's `extraEnv` instead of templating a URL yourself. The operator constructs the connection URL:

```yaml
valkey:
  enabled: false

extraEnv:
  - name: VALKEY_HOST
    valueFrom:
      secretKeyRef:
        name: my-valkey-serviceuser
        key: SERVICEUSER_HOST
  - name: VALKEY_PORT
    valueFrom:
      secretKeyRef:
        name: my-valkey-serviceuser
        key: SERVICEUSER_PORT
  - name: VALKEY_USERNAME
    valueFrom:
      secretKeyRef:
        name: my-valkey-serviceuser
        key: SERVICEUSER_USERNAME
  - name: VALKEY_PASSWORD
    valueFrom:
      secretKeyRef:
        name: my-valkey-serviceuser
        key: SERVICEUSER_PASSWORD
  - name: VALKEY_TLS
    value: "true"
```

`extraEnv` entries are appended after the chart's built-in environment variables, so they also take precedence over any Valkey settings the chart emits itself.

## High availability

The upstream Helm chart currently [does not support](https://github.com/valkey-io/valkey-helm/issues/18) Valkey clustering. Once this is delivered, we will have to update the protocol from `redis://` to `redis+cluster://`.
