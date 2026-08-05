# UI Authentication

The operator's web UI can be protected with an authentication provider. Two providers are supported: **OIDC** (OpenID Connect) and **GitHub OAuth**. If neither is configured, the UI is publicly accessible.

Only one provider can be active at a time. OIDC takes precedence over GitHub OAuth.

---

## OIDC

Compatible with any OIDC-compliant identity provider (Keycloak, Dex, Google, Azure AD, Okta, etc.).

### Helm Configuration

```yaml
auth:
  oidc:
    enabled: true
    issuerUrl: "https://accounts.google.com"   # OIDC provider issuer URL
    clientId: "your-client-id"
    existingSecret: "oidc-secret"              # Kubernetes secret name
    secretKey: "client-secret"                 # Key inside the secret
    sessionSecretKey: ""                       # Optional: key for session encryption secret
    redirectUrl: ""                            # Optional: auto-detected from ingress
    redirectScheme: ""                         # Optional: http or https, overrides the auto-detected scheme
    insecureSkipVerify: false                  # Do not use in production
    logoutUrl: ""                              # Optional: auto-discovered via OIDC metadata
    allowedGroupPrefix: ""                     # Optional: only accept groups with this prefix
    allowedGroupPattern: ""                    # Optional: only accept groups matching this regex
    additionalScopes: []                        # Optional: extra OIDC scopes (e.g., ["groups"])
    fetchUserInfoGroups: false                   # Optional: fetch groups from userinfo endpoint
```

The redirect URL is auto-detected from the chart's `route.hostnames[0]` or `ingress.host`, using `https` only when `ingress.tls` is set. When TLS terminates outside the cluster (external load balancer, Gateway API listener), set `redirectScheme: https` to correct the scheme — or set `redirectUrl` to a full URL, which takes precedence.

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: oidc-secret
stringData:
  client-secret: "<your-oidc-client-secret>"
```

### OIDC Provider Setup

Register a confidential OAuth client with your identity provider and set the callback URL to:

```
https://<your-operator-host>/auth/callback
```

Required scopes: `openid`, `email`, `profile`

#### Additional Scopes

By default, only the standard OIDC scopes (`openid`, `email`, `profile`) are requested. Some providers support additional custom scopes — for example, Keycloak supports a `groups` scope to include group membership in the ID token.

To request extra scopes, set `additionalScopes`:

```yaml
auth:
  oidc:
    additionalScopes:
      - groups
```

**Azure AD / Entra ID**: Do **not** add `groups` here. Azure AD does not support `groups` as an OIDC scope and will reject the request with `AADSTS650053`. Instead, configure the `groups` claim in **App Registration → Token Configuration → Add groups claim**. The operator will read groups from the ID token regardless of whether the scope is requested.

#### Userinfo Group Fetching

Some OIDC providers (Keycloak, Auth0, custom setups) expose groups exclusively via the userinfo endpoint rather than in the ID token. To fetch groups from the userinfo endpoint and merge them with any ID token groups:

```yaml
auth:
  oidc:
    fetchUserInfoGroups: true
```

When enabled, the operator makes an additional HTTP call to the provider's userinfo endpoint during login. Groups from both sources are deduplicated and merged before validation. Userinfo failures are treated as hard errors and will block login.

---

## GitHub OAuth

Authenticates users via a GitHub OAuth App.

### Helm Configuration

```yaml
auth:
  github:
    enabled: true
    clientId: "your-github-client-id"
    existingSecret: "github-oauth-secret"     # Kubernetes secret name
    secretKey: "client-secret"                # Key inside the secret
    sessionSecretKey: ""                      # Optional session encryption key
    redirectUrl: ""                           # Optional: auto-detected from ingress
    redirectScheme: ""                        # Optional: http or https, overrides the auto-detected scheme
    orgGroups: false                          # Optional: map org and team membership to groups (needs read:org)
```

Redirect URL auto-detection and `redirectScheme` behave exactly as described for [OIDC](#helm-configuration) above.

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: github-oauth-secret
stringData:
  client-secret: "<your-github-client-secret>"
```

### GitHub OAuth App Setup

Create an OAuth App at **GitHub → Settings → Developer settings → OAuth Apps** with the callback URL:

```
https://<your-operator-host>/auth/callback
```

The operator requests the `read:user` and `user:email` scopes, plus `read:org` when
`auth.github.orgGroups` is enabled. On logout, the OAuth token is automatically revoked.

---

## Session Security

Sessions expire after **24 hours**. Two session storage modes are available:

**Cookie-based** (default): the full session is AES-256-GCM encrypted and stored directly in the browser cookie. This is stateless — sessions survive pod restarts and require no external infrastructure. However, if users belong to many groups the encrypted cookie may exceed the ~4096 byte browser limit, causing authentication failures.

**Valkey** (opt-in): only an encrypted session ID (~100 bytes) is stored in the cookie; the full session data lives server-side in Valkey. This avoids cookie size limits regardless of group count, and sessions are shared across replicas. Recommended for multi-replica deployments or when users have many group memberships.

If you run multiple operator replicas, you **must** set a static session secret so all replicas can decrypt the session cookie. Set the session secret via `sessionSecretKey` pointing to a key in your existing secret, or the operator will auto-generate one per startup.

### Valkey Session Store

To use Valkey for session storage, either deploy a Valkey instance via the bundled subchart or provide an external Valkey URL via a secret:

**Option 1: Bundled Valkey subchart**

```yaml
valkey:
  enabled: true
  auth:
    enabled: true
    aclUsers:
      default:
        password: ""
        permissions: "~* &* +@all"
  dataStorage:
    enabled: false  # sessions are ephemeral
```

**Option 2: Valkey URL from an existing secret**

```yaml
valkey:
  existingSecret: "my-valkey-secret"
  existingSecretKey: "valkey-url"   # key containing e.g. "redis://:password@valkey:6379/0"
```

For TLS connections, use the `rediss://` scheme in the secret value.

---

## Access Control

When authentication is enabled, access to each RenovateJob is resolved from the
user's group membership. There are two roles:

| Role | May do |
|---|---|
| `reader` | view the job, its projects, statuses, PR activity and dependency issues; stream Renovate logs |
| `admin` | everything a reader may do, plus trigger a project, trigger all projects, cancel a run, start discovery and change execution options |

A job the request holds no role on is not listed and answers `404`, so its
existence is not disclosed. A reader attempting a write gets `403`.

### How it works

1. **No authentication provider configured**: there is no identity to evaluate,
   so every request is an admin and `spec.access` is ignored.
2. **Authentication enabled**: the session's groups are matched against the
   job's effective access configuration.
   - a match in `adminGroups` grants `admin`
   - otherwise a match in `readerGroups` grants `reader`
   - otherwise `anonymousRead` grants `reader`
   - otherwise the job is hidden (**fail closed**)

Anonymous read is a floor, not a sessionless special case: when it is enabled
everyone gets read access, and group matches only ever add to it. Signing in can
never take access away.

> **Fail closed**: a job with no access configuration anywhere is hidden from
> everyone once authentication is enabled. Set `auth.defaultAccess.adminGroups`
> (or `spec.access` per job) or the dashboard will look empty.

### Per-job configuration

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: my-renovate-job
spec:
  schedule: "0 2 * * *"
  access:
    readerGroups:
      - team-platform
    adminGroups:
      - team-devops
    anonymousRead: false      # readable without a session
    anonymousReadLogs: false  # anonymous readers may stream Renovate logs
  # ... other fields
```

Every field falls back to the operator-wide default when unset, so a job can add
to the defaults but cannot remove them. `anonymousRead` and `anonymousReadLogs`
are three-state: unset inherits, `false` opts out of an enabled default.

### Operator-wide defaults

These apply to every job that leaves the matching field unset:

```yaml
auth:
  defaultAccess:
    readerGroups:
      - team-platform
    adminGroups:
      - team-devops
    anonymousRead: false
    anonymousReadLogs: false
```

### Public dashboards

To publish a read-only dashboard for public repositories while keeping actions
restricted to maintainers:

```yaml
spec:
  access:
    anonymousRead: true
    adminGroups:
      - my-org/maintainers
```

Anonymous visitors see the dashboard; the action controls render disabled with a
hint to sign in. Renovate logs stay closed unless `anonymousReadLogs` is also
enabled, because log output is not redacted by the operator and can expose
private registry URLs, internal dependency names and branch names.

### GitHub org and team groups

GitHub OAuth has no group concept on its own. Enable `auth.github.orgGroups` to
map membership into session groups as `org` and `org/team`:

```yaml
auth:
  github:
    enabled: true
    orgGroups: true   # requires the read:org scope
```

Group membership is captured at login, so changes on GitHub take effect the next
time the user signs in. With `orgGroups` disabled the operator cannot evaluate
any group and **refuses to start** if access groups are configured, rather than
silently hiding every job.

### OIDC group filtering

Filter which groups from your OIDC provider are accepted:

```yaml
auth:
  oidc:
    # ... other OIDC settings ...
    allowedGroupPrefix: "renovate-"              # Only accept groups starting with "renovate-"
    allowedGroupPattern: "^(team-|platform-).*"  # Only accept groups matching regex
```

This is useful when your identity provider returns many groups but you only want
to use certain ones for authorization. An OIDC provider that emits no groups at
all leaves every job hidden.

### Deprecated: allowedGroups

`spec.allowedGroups` and `auth.defaultAllowedGroups` still work and are treated
as admin groups. They are mutually exclusive with `spec.access`: setting both on
one RenovateJob is rejected by the API server, and if an older CRD without that
validation accepts it, the operator treats the job as inaccessible rather than
guessing.

```yaml
# before
spec:
  allowedGroups:
    - team-devops

# after
spec:
  access:
    adminGroups:
      - team-devops
```

### Security considerations

- **Fail closed**: jobs without any access configuration are hidden when auth is enabled
- **Group validation**: groups are normalized (lowercased, trimmed) and validated
- **Audit logging**: access decisions and denials are logged for security auditing
- **Route allowlist**: only a fixed set of read routes can be served without a session; every other route requires one, so a new endpoint is protected by default

---

## Notes

- Auth protects the **web UI only**. The webhook endpoints are unaffected.
