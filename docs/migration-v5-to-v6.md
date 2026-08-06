# Migrating from v5 to v6

v6 adds a policy engine to the operator. It exists because a RenovateJob is more powerful than it
looks: the operator acts on one using **its own** cluster credentials, so a few spec fields let
whoever can edit a RenovateJob read secrets they cannot otherwise read, and send them, or the job's
platform token, to a host of their choosing. [security.md](security.md) has the full reasoning.

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

Three things are **not** governed by the switch, because they are not enforced by the operator.
Check these before upgrading:

| Change | Symptom | Fix |
|---|---|---|
| the CRD rejects `hostPath`, `privileged`, `allowPrivilegeEscalation` | `kubectl apply` on such a RenovateJob is rejected | [section 6](#6-what-the-crd-rejects-outright) |
| Kubernetes 1.29 is now the floor | `helm install`/`upgrade` refuses | [section 6](#6-what-the-crd-rejects-outright) |
| `spec.securityContext` merges over the hardened defaults instead of replacing them | a partial override no longer drops the other defaults | [section 4](#4-running-as-root) |

Everything else is opt-in.

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
