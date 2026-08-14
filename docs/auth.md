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
user's group membership, or from their account named directly. There are two
roles:

| Role | May do |
|---|---|
| `reader` | view the job, its projects, statuses, PR activity and dependency issues; stream Renovate logs |
| `admin` | everything a reader may do, plus trigger a project, trigger all projects, cancel a run, start discovery and change execution options |

A job the request holds no role on is not listed and answers `404`, so its
existence is not disclosed. A reader attempting a write gets `403`.

### How it works

1. **No authentication provider configured**: there is no identity to evaluate,
   so every request is an admin and `spec.access` is ignored.
2. **Authorization disabled** (`authorization.enabled: false`): authentication
   alone decides, so every session is an admin on every job. See
   [Disabling authorization](#disabling-authorization).
3. **Authentication enabled**: the session is matched against the job's effective
   access configuration.
   - a match in `adminUsers` or `adminGroups` grants `admin`
   - otherwise a match in `readerUsers` or `readerGroups` grants `reader`
   - otherwise `anonymousRead` grants `reader`
   - otherwise the job is hidden (**fail closed**)

Anonymous read is a floor, not a sessionless special case: when it is enabled
everyone gets read access, and matches only ever add to it. Signing in can never
take access away.

> **Fail closed**: a job with no access configuration anywhere is hidden from
> everyone once authentication is enabled. Set `authorization.defaults.adminUsers` or
> `authorization.defaults.adminGroups` (or `spec.access` per job) or the dashboard
> will look empty.

### Naming users instead of groups

`adminUsers` and `readerUsers` name individual accounts, so access needs no group
to exist at all. This is what a single-operator install wants: GitHub reports no
org for a personal account, so a group-only model has no way to name its owner.

An entry matches the session's **email or username**, case-insensitively. Both
are checked because a GitHub account may keep its email private, in which case
the operator synthesizes `<login>@github` for display and the login is the value
worth configuring. The username comes from the GitHub `login`, or from the OIDC
`preferred_username` claim.

> **How much you can trust these depends on your identity provider.** A user rule
> is only as strong as the provider's guarantee that the value identifies one
> account:
>
> - **Email** is matched only when the provider vouched for it. OIDC
>   `email_verified` is honoured when present, and an address explicitly marked
>   unverified never matches a rule (it is still shown in the UI). If your
>   provider omits the claim entirely, the operator cannot tell, and an IdP with
>   self-service email addresses then lets an account claim any entry.
> - **Username** has no such signal. OIDC does not promise `preferred_username`
>   is unique or stable, so on a provider where users can change it, treat it as
>   untrusted and use groups instead. The GitHub `login` is unique and stable.
>
> When in doubt on OIDC, grant by group: group membership is assigned by an
> administrator, whereas these two claims may be self-asserted.

```yaml
auth:
  github:
    enabled: true       # orgGroups not needed

authorization:
  defaults:
    adminUsers:
      - octocat         # GitHub login
      - me@example.com  # or email, either matches
```

User and group rules combine: a match in either list at a given level grants that
role, and `adminUsers` outranks `readerGroups` just as `adminGroups` does.

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
    readerUsers:
      - auditor@example.com   # individual accounts, no group needed
    adminUsers:
      - octocat
    anonymousRead: false      # readable without a session
    anonymousReadLogs: false  # anonymous readers may stream Renovate logs
  # ... other fields
```

Every field falls back to the operator-wide default when unset; a field that is
set **replaces** it for that field. Setting `readerGroups` here therefore drops every
default reader group rather than adding to it, and the fields you leave unset
still inherit. `anonymousRead` and `anonymousReadLogs` are three-state: unset
inherits, `false` opts out of an enabled default.

> A per-job `spec.access` can **narrow** access, not only widen it. Check the
> operator-wide defaults before setting a field, or the people the default
> granted will lose the job.

### Operator-wide defaults

These apply to every job that leaves the matching field unset:

```yaml
authorization:
  defaults:
    readerGroups:
      - team-platform
    adminGroups:
      - team-devops
    readerUsers: []
    adminUsers:
      - me@example.com
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

**Rate-limit it at the ingress.** `anonymousRead` makes `/api/v1/renovatejobs`
and `/api/v1/discovery/status` reachable without a session, and with
`anonymousReadLogs` so is `/api/v1/logs`, which opens a pod log stream against
the Kubernetes API server per request. The operator does not rate-limit, so a
dashboard actually exposed to the internet wants a limit in front of it, applied
by whatever terminates traffic: a Traefik `RateLimit` middleware, an Envoy Gateway
`BackendTrafficPolicy` with `rateLimit`, or the equivalent for your controller.
Gateway API has no portable rate-limit filter, so this is implementation-specific
either way.

### Disabling authorization

Authorization can be disabled while authentication stays in place. This suits
deployments where the login exists to keep strangers out rather than to
distinguish between users:

```yaml
authorization:
  enabled: false        # AUTHORIZATION_ENABLED

auth:
  oidc:
    enabled: true
    # ... provider settings
```

With this set:

- every user who can log in is an **admin on every RenovateJob**, including
  triggering, cancelling and reconfiguring runs
- `spec.access.readerGroups`, `adminGroups`, `readerUsers` and `adminUsers` are
  ignored, as are their `authorization.defaults` counterparts
- `spec.access.anonymousRead` and `anonymousReadLogs` **still apply**: they
  govern what a visitor without a session may see, which authentication does not
  determine. A public read-only dashboard is therefore unaffected
- no job is hidden from an authenticated user, so the fail-closed default does
  not apply and the misconfiguration banner never appears
- the startup log reports the state as `authorizationEnabled=false`

It has no effect when no authentication provider is configured, since every
request is already an admin in that case.

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
time the user signs in.

You only need this if you want to grant access by org or team. To give named
people access, use `adminUsers` / `readerUsers` and leave `orgGroups` off: it adds
the `read:org` scope, which forces every user to re-consent and which some orgs
restrict at the OAuth-app level.

With `orgGroups` disabled the operator cannot evaluate any group, so no group
rule can ever match. Rather than serve an empty dashboard with no explanation,
the UI treats this as an unenforceable configuration: see below.

### Unenforceable access rules

Group rules configured against an identity provider that supplies no groups
cannot be enforced. Today that means GitHub OAuth with `auth.github.orgGroups`
disabled while either `authorization.defaults.*Groups` or any job's
`spec.access.*Groups` / `spec.allowedGroups` is set. Configuring only
`adminUsers` / `readerUsers` never triggers it, since those need no groups.

While the operator is in this state:

- every RenovateJob is hidden and every per-job endpoint answers `404`,
  including jobs that only use `anonymousRead`, since an unenforceable rule set
  cannot tell an authorized request from an unauthorized one
- the dashboard renders a prominent error box explaining what to fix
- `GET /api/v1/access/status` reports `{"misconfigured": true, "reason":
  "GroupsUnsupported", "message": "..."}`. It is readable without a session,
  because the misconfiguration hides everything from everyone, and carries no
  job names or counts
- the operator logs the reason together with the affected RenovateJobs

The operator keeps running: reconciliation, scheduling and Renovate runs are
unaffected, since this is a UI access problem. The check runs per request, so
both a job created later and a fix to the configuration take effect without a
restart.

Fix it by setting `auth.github.orgGroups=true` (`GITHUB_ORG_GROUPS`), or by
replacing the group lists with `adminUsers` / `readerUsers`.

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

Both are matched case-insensitively, because group names are normalized to lowercase before filtering, so `allowedGroupPrefix: "Renovate-"` and `allowedGroupPattern: "^Team-Renovate$"` work the same as their lowercase spellings. The pattern is compiled with the case-insensitive flag rather than lowercased, so escapes keep their meaning: `\D`, `\S`, `\W`, `\B` and `\p{Lu}` are not rewritten.

When a filter is configured and a user has no group left after it, the login is refused, so watch for `WARNING: All user groups filtered out by validation` in the operator log. It reports the count after each of the three layers, which distinguishes a provider sending no groups (`original_count: 0`) from a policy that rejects them all (`after_policy: 0`).

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
