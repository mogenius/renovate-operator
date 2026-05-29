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

| Environment variable           | Helm value                  | Default    | Description                                                                                            |
|--------------------------------|-----------------------------|------------|--------------------------------------------------------------------------------------------------------|
| `VALKEY_URL`                   | —                           | `""`       | Full connection URL, e.g. `redis://:password@valkey:6379/0`. Takes precedence over host/port/password. |
| `VALKEY_HOST`                  | —                           | `""`       | Valkey hostname. Used when `VALKEY_URL` is not set.                                                    |
| `VALKEY_PORT`                  | —                           | `6379`     | Valkey port. Used when `VALKEY_URL` is not set.                                                        |
| `VALKEY_PASSWORD`              | —                           | `""`       | Valkey password. Used when `VALKEY_URL` is not set.                                                    |
| `VALKEY_FORWARD_CACHE_TO_JOBS` | `config.forwardCacheToJobs` | `true`     | Forward the Renovate cache URL to executor jobs. Requires Valkey to be configured.                     |
| `LOG_STORE_MODE`               | `config.logStorage.mode`    | `disabled` | Log storage backend: `disabled`, `memory`, or `valkey`.                                                |

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

## High availability

The upstream Helm chart currently [does not support](https://github.com/valkey-io/valkey-helm/issues/18) Valkey clustering. Once this is delivered, we will have to update the protocol from `redis://` to `redis+cluster://`.
