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

| Environment variable           | Helm value                                    | Default    | Description                                                                                                       |
|--------------------------------|-----------------------------------------------|------------|-------------------------------------------------------------------------------------------------------------------|
| `VALKEY_URL`                   | `externalKeyValueStore.existingSecret.urlKey` | `""`       | Full connection URL, e.g. `redis://user:password@valkey:6379/0`. Takes precedence over all host-based variables.  |
| `VALKEY_HOST`                  | `externalKeyValueStore.host` or `.existingSecret.hostKey` | `""` | Valkey hostname. Used when `VALKEY_URL` is not set.                                                          |
| `VALKEY_PORT`                  | `externalKeyValueStore.port` or `.existingSecret.portKey` | `6379` | Valkey port. Used when `VALKEY_URL` is not set.                                                            |
| `VALKEY_USERNAME`              | `externalKeyValueStore.username` or `.existingSecret.usernameKey` | `""` | Valkey ACL username. Used when `VALKEY_URL` is not set. Omit to authenticate as the `default` user. |
| `VALKEY_PASSWORD`              | `externalKeyValueStore.existingSecret.passwordKey` | `""`  | Valkey password. Used when `VALKEY_URL` is not set. Only available via secret.                                    |
| `VALKEY_TLS`                   | `externalKeyValueStore.useTls`                | `false`    | Connect with TLS (`rediss://`). Used when `VALKEY_URL` is not set; a URL carries its own scheme.                  |
| `VALKEY_FORWARD_CACHE_TO_JOBS` | `config.forwardCacheToJobs`                   | `true`     | Forward the Renovate cache URL to executor jobs. Requires Valkey to be configured.                                |
| `LOG_STORE_MODE`               | `config.logStorage.mode`                      | `disabled` | Log storage backend: `disabled`, `memory`, `valkey`, or `s3` (see [S3 Object Storage](./s3.md)).                  |

Host, port, and username can each be set as a clear Helm value or sourced from the secret; the secret key wins when both are set. The password (and the full URL) can only be provided via secret.

The server certificate of a TLS-enabled Valkey must chain to a publicly trusted CA — there is currently no option to supply a custom CA bundle. Note that the executor jobs receive the same URL, so Renovate's containers must be able to verify the certificate too.

## Deploying with the bundled Helm chart

The chart includes an optional single-node Valkey instance (via the [official Valkey Helm chart](https://github.com/valkey-io/valkey-helm)):

```yaml
valkey:
  enabled: true
```

## Connecting to an external instance

An external Redis-protocol-compatible store (Redis, Valkey, …) is configured via the top-level `externalKeyValueStore` value, which takes precedence over the bundled instance. The connection is active when `host` or `existingSecret.name` is set.

To provide the full connection URL via an existing Kubernetes secret:

```yaml
externalKeyValueStore:
  existingSecret:
    name: "my-valkey-secret"
    urlKey: "valkey-url"
```

The secret must contain the full connection URL under the configured key:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-valkey-secret
stringData:
  valkey-url: "redis://user:yourpassword@valkey.example.com:6379/0"
```

When `urlKey` is set, all other `externalKeyValueStore` values are ignored.

> **Deprecated:** `valkey.existingSecret` / `valkey.existingSecretKey` configure the same URL-via-secret connection and are superseded by `externalKeyValueStore.existingSecret`; they will be removed in a future release.

### Credentials in separate secret fields

When the credentials arrive as separate secret fields rather than a ready-made URL — for example the [Aiven operator](https://aiven.github.io/aiven-operator/)'s `ServiceUser` secret with its `SERVICEUSER_*` keys — configure the fields individually and the operator constructs the connection URL. Each of host, port, and username can come from the secret (`existingSecret.{param}Key`) or be set as a clear value (`{param}`); the password can only come from the secret:

```yaml
externalKeyValueStore:
  useTls: true
  existingSecret:
    name: my-valkey-serviceuser
    hostKey: SERVICEUSER_HOST
    portKey: SERVICEUSER_PORT
    usernameKey: SERVICEUSER_USERNAME
    passwordKey: SERVICEUSER_PASSWORD
```

## High availability

The upstream Helm chart currently [does not support](https://github.com/valkey-io/valkey-helm/issues/18) Valkey clustering. Once this is delivered, we will have to update the protocol from `redis://` to `redis+cluster://`.
