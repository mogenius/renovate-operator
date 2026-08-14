# Migrating from v5 to v6

v6 brings two independent changes: a policy engine, and a rewritten UI access model.

The policy engine exists because a RenovateJob is more powerful than it looks: the operator acts on
one using **its own** cluster credentials, so a few spec fields let whoever can edit a RenovateJob
read secrets they cannot otherwise read, and send them, or the job's platform token, to a host of
their choosing. [security.md](security.md) has the full reasoning.

The access model replaces the single `spec.allowedGroups` boolean with reader and admin roles. Unlike
the policy engine it ships **on**, and it changes what an existing install shows in the UI, so read
[section 8](#8-ui-access-is-now-two-role-and-fails-closed) before upgrading if UI authentication is
enabled. It does have a master switch, `authorization.enabled: false`, which keeps the login and
makes every user who passes it an admin. If you run no auth provider at all, none of this affects
you.

**The policy engine ships off.** It is opt-in so that a fresh install works straight away: you get
Renovate running first and secure the operator afterwards. A useful side effect for you: upgrading
halts no RenovateJob, and nothing in sections 1-5 below applies until you turn enforcement on.

The flip side is that a stock v6 install is no better protected than v5, and the operator says so on
every start:

```
WARNING: the policy engine is NOT ENABLED -- this operator is running in an UNSECURED mode.
```

Every RenovateJob also reports the state, so it is visible without reading logs:

```console
$ kubectl get renovatejobs -o wide
NAME     SCHEDULE    PROVIDER   ACCEPTED   REASON
my-job   0 * * * *   gitlab     True       PolicyDisabled
```

Treat enabling it as the point of the upgrade, not an optional extra. On a cluster where RenovateJob
write access reaches beyond the people who already hold cluster-wide secret access, leaving it off
leaves the original exposure in place.

The built-in engine is not the only way to close that exposure, though: restricting who may write
RenovateJobs addresses all of it, and an external admission policy engine such as Kyverno or a native
`ValidatingAdmissionPolicy` can express the same constraints and some the built-in engine cannot. If
you take one of those routes, leave `policy.enabled: false` and skip to
[section 6](#6-what-the-crd-rejects-outright), which applies either way. See
[security.md](security.md#choosing-a-mitigation) for the trade-offs and a coverage table.

## What breaks on upgrade regardless

These are **not** governed by the policy switch. Check them before upgrading:

| Change | Symptom | Fix |
|---|---|---|
| the CRD rejects `hostPath`, `privileged`, `allowPrivilegeEscalation` | `kubectl apply` on such a RenovateJob is rejected | [section 6](#6-what-the-crd-rejects-outright) |
| Kubernetes 1.29 is now the floor | `helm install`/`upgrade` refuses | [section 6](#6-what-the-crd-rejects-outright) |
| `spec.securityContext` merges over the hardened defaults instead of replacing them | a partial override no longer drops the other defaults | [section 4](#4-running-as-root) |
| a RenovateJob with no access configuration is hidden once UI auth is enabled | the dashboard is empty for everyone after upgrade | [section 8](#8-ui-access-is-now-two-role-and-fails-closed) |
| `spec.allowedGroups` and `spec.access` are mutually exclusive | `kubectl apply` on a spec with both is rejected | [section 8](#8-ui-access-is-now-two-role-and-fails-closed) |
| group rules with GitHub OAuth need `auth.github.orgGroups`, or `adminUsers` instead | the dashboard shows "Access rules cannot be enforced" | [section 8](#8-ui-access-is-now-two-role-and-fails-closed) |

The last three only apply when UI authentication is enabled. Everything else is opt-in.

## Turning the policy engine on

```yaml
policy:
  enabled: true
```

Do it after working through the sections below, or enable it in a non-production cluster first and
let the operator tell you what is missing; every refusal names the field and the value to add.

Two things enabling does not change:

- It does not affect what the CRD rejects; see [section 6](#6-what-the-crd-rejects-outright).
- Configuration *validation* runs either way. A malformed `policy.allowedHosts` entry fails at
  startup even while enforcement is off, so a value you fix now will not surprise you later.

### Finding out what to configure

The fastest way to enumerate the work is to set `policy.enabled: true` in a non-production cluster
and read what the operator refuses. Every refusal names the field and the value to add:

```console
$ kubectl get renovatejobs -A -o wide
NAMESPACE   NAME      SCHEDULE    PROVIDER   ACCEPTED   REASON
renovate    my-job    0 * * * *   gitlab     False      DestinationNotAllowed

$ kubectl describe renovatejob -n renovate my-job | grep -A3 Accepted
    Type:     Accepted
    Status:   False
    Reason:   DestinationNotAllowed
    Message:  spec.provider.endpoint: host "gitlab.internal" is not allowed;
              add it to policy.allowedHosts (allowed: api.github.com, github.com, ...)
```

The UI shows the same message on the job's card. Denials also increment
`renovate_operator_policy_denials_total{check}`.

## 1. Allowed destination hosts

Every URL a RenovateJob points the operator at must be listed in `policy.allowedHosts`:
`spec.provider.endpoint`, `spec.provider.publicEndpoint`, and the webhook delivery base URL from
`spec.webhook.baseUrl` or `WEBHOOK_BASE_URL`.

The chart ships the public platform hosts, so add:

- **your platform host**, if self-hosted
- **the webhook host of any job that overrides `spec.webhook.baseUrl`**. The chart already appends
  its own webhook host (`webhook.baseUrl`, or the host derived from
  `webhook.ingress`/`webhook.route`) when the webhook server is enabled, so a deployment-wide
  delivery URL needs no entry.

```yaml
policy:
  allowedHosts:
    # keep the defaults you use
    - api.github.com
    - github.com
    # add yours
    - gitlab.internal
    - renovate-operator.renovate-operator.svc.cluster.local
```

Entries are bare hostnames, matched exactly: no wildcards, no subtree form. Ports are ignored, so
`gitea.internal` covers `https://gitea.internal:3000`.

Webhook *removal* is never gated, so a hook left behind by an earlier misconfiguration can still be
cleaned up.

## 2. Secret references must opt in

Three spec fields name a secret **and the key to read from it**, and the operator reads them with its
own credentials. Each target secret now needs a label:

```yaml
metadata:
  labels:
    renovate-operator.mogenius.com/allow-ref: "true"
```

Applies to `spec.webhook.sync.secretRef`, `spec.webhook.authentication.secretRef` and
`spec.githubAppReference`. Only the exact value `"true"` opts in.

Watch out for the webhook authentication secret: the webhook server reads it on **every incoming
delivery**, so an unlabelled secret means deliveries start being rejected rather than failing
visibly at reconcile time.

`spec.secretRef` is unaffected: it is only read at Renovate's own well-known token key names.

To relax this one control without disabling the whole engine:

```yaml
policy:
  requireSecretRefOptIn: false
```

## 3. Service accounts

`spec.serviceAccount.name` must be listed in `policy.allowedServiceAccounts`. Leaving the field unset
uses the namespace default and needs no configuration.

```yaml
policy:
  allowedServiceAccounts:
    - renovate-workload-identity
```

This is the control that closes the sharpest escalation in v5: naming a ServiceAccount let a
RenovateJob run its pods as another workload's identity, including the operator's own.

## 4. Running as root

`runAsUser: 0` or `runAsNonRoot: false` on `spec.securityContext` now needs:

```yaml
policy:
  allowRootUser: true
```

Related but separate: `spec.securityContext` now **merges** over the operator's hardened defaults
instead of replacing them. If you previously set one field and relied on the others being dropped,
you will now get the hardened value for everything you did not set. Setting a field explicitly still
wins.

## 5. Images

`spec.image` must match a `policy.allowedImages` entry exactly. Matching is literal (no prefix or
subpath form) and the tag or digest is ignored, so only the repository has to match.

```yaml
policy:
  allowedImages:
    - renovate/renovate
    - docker.io/renovate/renovate
    - ghcr.io/renovatebot/renovate
    # add yours
    - registry.internal:5000/mirror/renovate
```

Because the match is literal, **each spelling you use must be listed**. Both Docker Hub forms ship by
default; add `index.docker.io/…` if that is the form your specs use.

## 6. What the CRD rejects outright

These are enforced by the Kubernetes API server through CRD validation rules, so `kubectl apply`
fails and **`policy.enabled: false` does not help**:

| Field | Why |
|---|---|
| `spec.extraVolumes[*].hostPath` | mounts the node's filesystem, and with it every credential on that node |
| `spec.securityContext.container.privileged` | equivalent to root on the host |
| `spec.securityContext.container.allowPrivilegeEscalation` | the same, one step removed |

This is the one part of v6 that applies whether or not the policy engine is on, so if a RenovateJob
uses any of them, edit it **before** upgrading. An existing object keeps working until something
tries to update it, at which point the write is rejected.

Replace `hostPath` with `configMap`, `secret`, `emptyDir`, a PVC or a generic ephemeral volume. If you
have a use case that genuinely needs one of these three, open an issue: they were made absolute
because no documented Renovate workflow needs them, and that is a claim worth correcting if wrong.

v6 also requires **Kubernetes 1.29 or newer**, because those validation rules use CEL, which is GA
from 1.29. On an older API server they would be silently ignored, so the chart declares the floor and
the install fails instead.

## 7. RBAC

The operator's ClusterRole no longer grants `list`, `watch` or `delete` on secrets, only
`get`, `create` and `update`. Nothing to do on your side; it is listed here because it changes the
blast radius of an operator compromise, and because Secrets now bypass the informer cache: the
operator no longer holds every secret in the watched scope in memory, at the cost of one API read per
secret access.

## 8. UI access is now two-role and fails closed

Unrelated to the policy engine, and **not** covered by `policy.enabled`. It has its own switch,
`authorization.enabled`. It only concerns the web UI, so nothing here affects whether Renovate
runs.

**If you have no auth provider configured, skip this section.** With no identity to evaluate, every
request is treated as an admin and access rules are ignored, exactly as in v5.

v5 had one control: `spec.allowedGroups`, which granted everything or nothing. v6 splits that into
readers and admins, adds anonymous read for public dashboards, and gates the unredacted Renovate logs
separately. [auth.md](auth.md#access-control) is the reference; this section is only what changes on
upgrade.

The chart keeps the two concepts apart. `auth` configures **authentication**, the identity providers
and their sessions. The new top-level `authorization` block configures **what an identity may do**:

```yaml
auth:                      # who you are
  oidc: {...}
  github: {...}

authorization:             # what you may do
  enabled: true
  defaults:
    readerGroups: []
    adminGroups: []
    readerUsers: []
    adminUsers: []
    anonymousRead: false
    anonymousReadLogs: false
```

The corresponding environment variables are `AUTHORIZATION_ENABLED` and `AUTHORIZATION_DEFAULT_*`.
The one exception is the deprecated `auth.defaultAllowedGroups` (`DEFAULT_ALLOWED_GROUPS`), which
keeps its v5 name and location.

### The empty dashboard

This is the one that bites. In v5, a job without `allowedGroups` was visible to **every authenticated
user** whenever `auth.defaultAllowedGroups` was also empty. In v6 a job with no access configuration
anywhere is hidden from everyone.

If UI auth is enabled and you never configured groups, the dashboard goes empty on upgrade.

Where per-user access is not required, [Opting out of authorization
entirely](#opting-out-of-authorization-entirely) resolves this without configuring any of the
following.

**Single user, GitHub OAuth as a gate on a public domain?** Name yourself. This needs no org, no team
and no extra OAuth scope:

```yaml
authorization:
  defaults:
    adminUsers:
      - octocat         # your GitHub login
      - me@example.com  # or your email, either matches
```

An entry matches the session's email or username, case-insensitively. Use the login if your GitHub
email is private, because the operator then only knows a synthesized `<login>@github`.

Otherwise, the closest equivalent to what v5 did is to name the groups that should hold full access:

```yaml
authorization:
  defaults:
    adminGroups:
      - team-devops       # full access: view, trigger, cancel, discovery
    readerGroups:
      - team-platform     # view only, including logs
```

Or, if the dashboard was effectively public before because everyone who could sign in could see
everything, keep it visible to everyone and restrict only the actions:

```yaml
authorization:
  defaults:
    anonymousRead: true       # anyone may view, no session needed
    anonymousReadLogs: false  # but not stream Renovate logs
    adminGroups:
      - team-devops
```

Per-job `spec.access` takes the same six fields and inherits each one from the default when unset. A
field it does set **replaces** the default for that field instead of adding to it, so a job listing
one group drops every default group, and `anonymousRead: false` revokes an enabled default. A per-job
`spec.access` can therefore narrow access, not only widen it.

Set `anonymousReadLogs` deliberately. Renovate logs are passed through unredacted and can expose
private registry URLs, internal dependency names and branch names, which is why it is a second opt-in
rather than part of read access.

### Opting out of authorization entirely

Authorization can be disabled while authentication stays in place. This suits deployments where the
login exists to keep strangers out rather than to distinguish between users:

```yaml
authorization:
  enabled: false   # AUTHORIZATION_ENABLED
```

Every user who can log in then holds admin access on every RenovateJob, matching v5 behaviour when no
groups were configured. Compared with leaving the group lists empty, this states the intent
explicitly rather than relying on a default.

What it does and does not touch:

- `readerGroups`, `adminGroups`, `readerUsers` and `adminUsers` are ignored, per-job and operator-wide
- `anonymousRead` and `anonymousReadLogs` **still apply**, so a public read-only dashboard is
  unaffected. They govern what a visitor without a session may see, which authentication does not
  determine
- the empty dashboard described above cannot occur, and the unenforceable-rules banner is never shown
- `auth.github.orgGroups` is no longer required, avoiding the `read:org` re-consent it forces on
  every user
- the startup log reports the state as `authorizationEnabled=false`
- the policy engine is unaffected. It is a separate switch and continues to govern what the operator
  acts on

The default `true` applies wherever different users require different access. The flag is global and
has no per-job override, so a single RenovateJob cannot remain restricted while the rest are open.

### Everyone is signed out on upgrade

v6 records the account's username and whether its email is verified on the session, and v5 sessions
carry neither. Rather than let a still-valid v5 session resolve against fields it never had, which
would look like an empty dashboard for up to 24 hours, the operator rejects it and the user is sent
through login again. Nothing to configure; expect one round of sign-ins after the upgrade.

### allowedGroups is deprecated

`spec.allowedGroups` and `auth.defaultAllowedGroups` still work and now mean **admin** groups, so an
install that used them keeps working with no change. Migrate at your convenience:

Before:

```yaml
spec:
  allowedGroups: [team-devops]
```

After:

```yaml
spec:
  access:
    adminGroups: [team-devops]
```

Setting both on one spec is rejected by a CRD validation rule. An existing object carrying both keeps
working until something tries to update it, at which point the write fails, so drop one before you
edit such a job. If an un-upgraded CRD lets both through, the operator hides the job rather than
guessing which surface wins.

### GitHub OAuth needs orgGroups

Only needed if you want to grant access by org or team; `adminUsers` above covers naming people.

GitHub OAuth supplied no groups at all in v5, so `allowedGroups` on a GitHub install could never
match. If you use GitHub OAuth **and** configure any group, enable:

```yaml
auth:
  github:
    enabled: true
    orgGroups: true
```

This maps org and team membership to `org` and `org/team` at login. It adds the `read:org` scope, so
**every user has to re-consent** on their next sign-in, and an org that restricts OAuth apps has to
approve yours. Membership is captured at login, so a change on GitHub takes effect the next time the
user signs in.

### When the rules cannot be enforced

Group rules against a provider that supplies no groups can never match, so the operator would serve an
empty dashboard with nothing to explain it. Instead the UI reports the state:

```
Access rules cannot be enforced
Access rules are configured against groups, but the identity provider supplies none, ...
```

While that box is showing, **every** RenovateJob is hidden and every per-job endpoint answers `404`,
including jobs that only use `anonymousRead`: an unenforceable rule set cannot tell an authorized
request from an unauthorized one. The operator keeps reconciling and Renovate keeps running; only the
UI is affected. `GET /api/v1/access/status` reports the same thing for scripting, and the operator log
names the RenovateJobs involved.

The check runs per request, so fixing the values clears it without a restart. Fix it by enabling
`orgGroups`, or by replacing the group lists with `adminUsers` / `readerUsers`. Configuring only user
rules never trips it.

### API changes

If you script against the UI API: `GET /api/v1/renovatejobs` gains `role` (display only) and
`permissions[]` per job, holding any of `logs`, `trigger`, `triggerAll`, `cancel`, `discovery`. Gate on
`permissions`, not on `role`, because two readers can differ on log access. Unreadable jobs answer
`404` rather than `403`, so a job's existence is not disclosed.

## Audit an installation that ran v5

The destination allowlist stops new damage; it cannot undo what an unbounded `spec.webhook.baseUrl`
already wrote. On a cluster that ran v5 with RenovateJob write access spread beyond cluster admins:

1. List the webhooks on your repositories and look for delivery hosts you do not recognise. The
   operator's own hooks carry `namespace=` and `job=` query parameters.
2. Rotate any webhook authentication token that was ever synced to GitLab, Gitea or Forgejo; those
   platforms store it in cleartext on the hook, so it was delivered to whatever host the hook
   pointed at.
3. Rotate the platform token in `spec.secretRef` if `spec.provider.endpoint` was ever set to
   something unexpected.

## Rolling back

If enabling the policy engine causes trouble, set `policy.enabled: false` again; that is a values
change, not a rollback, and it keeps the fixes that are not policy: the merged securityContext
defaults, the narrowed RBAC, and the CRD invariants.

For a full downgrade to v5, the v6 chart is clean apart from the CRD: the `Accepted` status condition
and the two `spec.provider` URL patterns are additive, and v5 ignores the condition. Reinstalling the
v5 chart leaves a stale condition on each RenovateJob, which is harmless and disappears when the
field is dropped. The exception is section 6: a RenovateJob you edited to drop a `hostPath` volume
stays edited.

Two things to watch when downgrading with the access model in use. v5 knows nothing about
`spec.access`, so it reads a job configured only that way as having no group restriction at all and
shows it to **every authenticated user**: a rollback widens access rather than preserving it. You
cannot pre-empt this by populating both fields, since v6 rejects a spec that sets `allowedGroups` and
`access` together. Either stay on `spec.allowedGroups`, which both versions honour, or treat restoring
it as a step of the downgrade, once the v5 CRD has replaced the v6 one and dropped that rule. Also
drop the whole `authorization` block and `auth.github.orgGroups` from your values before reinstalling
the v5 chart, whose values schema rejects keys it does not know.
