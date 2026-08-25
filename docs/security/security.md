# Security

## Trust model

The operator acts on a RenovateJob using **its own** cluster credentials, not the credentials of
whoever wrote the RenovateJob. That makes a few spec fields more powerful than they look:

- URLs in a spec are destinations the operator authenticates to, or hands to a Git platform to
  deliver events to.
- Secret references in a spec are secrets the operator reads, in the RenovateJob's namespace.

Treat `create`/`patch` on `renovatejobs` as a privileged grant, comparable to `create` on `pods` in
the same namespace. Everything below is about bounding what that grant can reach and, first,
deciding whether it needs bounding at all.

## Attack vectors

What someone who can create or edit a RenovateJob can do, absent any mitigation. "Attacker" here
means anyone holding that permission who should not be trusted with the operator's own credentials,
including a compromised CI token that commits to a GitOps repository.

| # | Vector | Spec field | What it yields |
|---|---|---|---|
| 1 | **Platform token exfiltration** | `spec.provider.endpoint` | the job's Renovate token (usually org-wide repository write) is sent as an `Authorization` header to a host of their choosing on the operator's first API call |
| 2 | **Persistent event and token exfiltration** | `spec.webhook.baseUrl` | written onto **your repositories'** webhooks as the delivery URL. The platform then posts every repository event there, and on GitLab, Gitea and Forgejo the webhook auth token too, in cleartext. It keeps working after the RenovateJob is fixed or deleted |
| 3 | **Arbitrary secret read** | `spec.webhook.sync.secretRef`, `spec.webhook.authentication.secretRef`, `spec.githubAppReference` | these name a secret **and a key**, and the operator reads them with its own credentials, so any value in the namespace, not just the ones the writer could read. Combine with 1 and it leaves the cluster |
| 4 | **ServiceAccount impersonation** | `spec.serviceAccount.name` | runs the Renovate pod as any ServiceAccount in the namespace. In the operator's own namespace that includes the operator's, which can read secrets cluster-wide |
| 5 | **Node escape** | `spec.extraVolumes[*].hostPath`, `securityContext.privileged`, `allowPrivilegeEscalation` | the node filesystem, and with it kubelet credentials and every secret on that node |
| 6 | **Arbitrary code execution** | `spec.image`, `spec.extraEnv`, `spec.renovateConfig` | any image. Note that pinning the image does not close this: the official Renovate image is a Node runtime, `spec.renovateConfig` supplies an executable JavaScript config file as a first-class field, and `RENOVATE_CONFIG_FILE` in `spec.extraEnv` can point at one from any mounted volume |
| 7 | **Namespace secret mounts** | `spec.extraEnvFrom`, `spec.extraVolumes` | mounts any secret in the namespace into the Renovate pod. This is the *documented feature set* (it is how SSH keys and CA certificates get in) and is equivalent to what `create pods` already grants |
| 8 | **UI link phishing** | `spec.provider.publicEndpoint` | attacker-controlled links rendered as dashboard and PR links under the operator's own UI |

Vectors 1-4 are the ones worth understanding properly, because they are **not** equivalent to
`create pods`: they use the *operator's* identity to reach things the caller could not reach directly,
and 2 keeps doing so after the fact. 5-7 are pod-level, and anyone who can already create a pod in
that namespace has them anyway.

## Choosing a mitigation

Three approaches. They are not alternatives so much as layers, and the right answer depends on who
can write RenovateJobs in your cluster.

### 1. Restrict who can write RenovateJobs

The most complete answer, and the one to reach for first. Every vector above collapses if
`create`/`patch` on `renovatejobs` is held only by people you would trust with the operator's
ServiceAccount, because that is effectively what the permission grants.

```yaml
# RenovateJobs are cluster infrastructure: treat the permission like any other
# privileged grant, not like an application resource.
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: renovate-job-author
  namespace: renovate-operator
rules:
  - apiGroups: ["renovate-operator.mogenius.com"]
    resources: ["renovatejobs"]
    verbs: ["get", "list", "watch"] # read-only for everyone else
```

If your RenovateJobs are applied by a GitOps controller from a repository with reviewed commits, that
review *is* your control, and you may reasonably decide you need nothing further. Note what the
threat model then rests on: anyone who can merge to that repository, and any token that can push to
it.

### 2. The operator's built-in policy engine

Covers vectors 1-4 and 8 fully and 6 partially, with a handful of values. Needs no extra component,
and reports refusals on the RenovateJob itself. Vector 5 is closed by the CRD whether or not the
engine is on, so enabling it is not what stops `hostPath`.

This is what the rest of this document describes. It suits you when you cannot restrict who writes
RenovateJobs (self-service, multi-tenant, or many teams committing to one GitOps repo), or when you
want a backstop that does not depend on getting RBAC exactly right.

It is deliberately **not** a general policy language: a fixed set of checks, configured per install
rather than per namespace or tenant, and it says nothing about vector 7.

### 3. An external admission policy engine

If you already run [Kyverno](https://kyverno.io), Gatekeeper/OPA, or Kubernetes'
[ValidatingAdmissionPolicy](https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/)
(1.30+, no extra component), you can express the same constraints there, and some the built-in
engine cannot. Advantages: rejection happens at `kubectl apply` rather than at reconcile, policies can
differ per namespace or tenant, and you get them in the same audit trail as the rest of your
admission policy.

The clearest case for reaching here is **vector 7**, which the built-in engine deliberately leaves
alone because mounting secrets is the documented feature. Sketched against Kyverno; treat it as the
shape of the rule rather than something to apply unread, since the `validate` and failure-action
syntax has moved between Kyverno versions:

```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: renovatejob-restrict-secret-mounts
spec:
  rules:
    - name: only-approved-secrets
      match:
        any:
          - resources:
              kinds: ["renovate-operator.mogenius.com/v1alpha1/RenovateJob"]
      validate:
        message: "spec.extraEnvFrom may only reference secrets named renovate-*"
        # the constraint: every extraEnvFrom entry must name an approved secret
        pattern:
          spec:
            =(extraEnvFrom):
              - =(secretRef):
                  name: "renovate-*"
```

The same constraint as a native `ValidatingAdmissionPolicy` needs no extra component on 1.30+, at the
cost of writing it in CEL:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: renovatejob-restrict-secret-mounts
spec:
  matchConstraints:
    resourceRules:
      - apiGroups: ["renovate-operator.mogenius.com"]
        apiVersions: ["v1alpha1"]
        operations: ["CREATE", "UPDATE"]
        resources: ["renovatejobs"]
  validations:
    - expression: >-
        !has(object.spec.extraEnvFrom) ||
        object.spec.extraEnvFrom.all(e,
          !has(e.secretRef) || e.secretRef.name.startsWith('renovate-'))
      message: "spec.extraEnvFrom may only reference secrets named renovate-*"
```

Using an external engine instead of the built-in one is a legitimate choice. If you do, leave
`policy.enabled: false` and be deliberate about covering vectors 1-4: those are the ones that use the
operator's identity rather than the caller's, and they are easy to overlook when writing pod-shaped
policy.

### Coverage at a glance

| Vector | Built-in policy engine | Restricting RBAC | External admission policy |
|---|---|---|---|
| 1 platform token exfiltration | `allowedHosts` | ✅ | ✅ |
| 2 webhook redirect | `allowedHosts` | ✅ | ✅ |
| 3 arbitrary secret read | `allow-ref` label | ✅ | ✅ (can also restrict names and keys) |
| 4 ServiceAccount impersonation | `allowedServiceAccounts` | ✅ | ✅ |
| 5 node escape | CRD, always on, not governed by the switch | ✅ | ✅ (redundant) |
| 6 arbitrary code execution | `allowedImages`, partial: pinning the image does not stop config-supplied code | ✅ | partial, same limit |
| 7 namespace secret mounts | ❌ by design | ✅ | ✅ (the main reason to use one) |
| 8 UI link phishing | `allowedHosts` | ✅ | ✅ |

Vector 5 is enforced by the CRD itself and is on regardless of any of these; see
[what the CRD rejects](#rejected-outright-by-the-crd).

## The policy engine is off by default

Every operator-side control on this page is governed by one switch, and it ships **off**:

```yaml
policy:
  enabled: false # the default; turn this on
```

The default is about getting started: a first install runs without you having to enumerate platform
hosts, label secrets and list images up front, which is what makes the operator usable in a homelab
or on a throwaway cluster. It is not a statement that the unenforced state is safe: while the
switch is off, every path in the trust model above is open to anyone who can write a RenovateJob.

The operator warns on every start, and each RenovateJob reports `Accepted=True` with the reason
`PolicyDisabled`, so `kubectl get renovatejobs -o wide` shows the state at a glance.

Secure it afterwards: configure the values below, then flip the switch.
[migration-v5-to-v6.md](../migration/migration-v5-to-v6.md) walks through the sequence. Leaving it off is
defensible for a homelab, a local cluster or CI; on anything shared it means the controls documented
here are not in effect.

The switch does **not** govern what the CRD rejects: `hostPath`, `privileged` and
`allowPrivilegeEscalation` are refused by the Kubernetes API server, which knows nothing about this
setting. It also does not disable configuration validation, so a malformed value fails at startup
either way rather than waiting to surprise you when you enable enforcement.

## Allowed destinations

Every externally reachable URL taken from a RenovateJob is checked against `policy.allowedHosts`
before the operator uses it:

| Spec field | What the operator does with it |
|---|---|
| `spec.provider.endpoint` | base URL of the authenticated platform API client, and `RENOVATE_ENDPOINT` in every Job pod |
| `spec.provider.publicEndpoint` | dashboard and pull-request links rendered in the UI |
| `spec.webhook.baseUrl` | the delivery URL written onto **your repositories'** webhooks |

Without this bound, anyone who can edit a RenovateJob can point `spec.provider.endpoint` at a host
they control and collect the job's Renovate platform token (usually an org-wide repository-write
credential) on the operator's first API call. Redirecting `spec.webhook.baseUrl` is worse, because
it persists: the platform keeps delivering every repository event to that host after the RenovateJob
is corrected or deleted, and on GitLab, Gitea and Forgejo the webhook authentication token is stored
in cleartext on the hook and delivered with it.

### Configuring it

```yaml
policy:
  allowedHosts:
    - api.github.com
    - github.com
    - gitlab.com
    - api.bitbucket.org
    - bitbucket.org
    - gitea.com
    - codeberg.org
```

Those are the chart defaults, so an install against a public platform needs no change. Entries are
bare hostnames (no scheme, port or path) and are matched **exactly**. Ports are ignored when
matching, so `gitea.internal` covers `https://gitea.internal:3000`.

The chart appends one host of its own: with `webhook.enabled`, the host of `webhook.baseUrl` — or of
`webhook.ingress`/`webhook.route` when the base URL is derived — is added to the list, since a URL
you configured in the chart is an approved destination by construction. A job that overrides
`spec.webhook.baseUrl` with a different host is still checked against the list as configured.

There is deliberately no subtree or wildcard form. `*.example.com` would extend trust to every name
anyone can bring up under that domain, and on an internal domain that is often a wide set of people:
a stray DNS record, a Service in another namespace, or an ingress hostname someone else controls
would all become valid exfiltration targets. Listing each host you actually talk to keeps the
allowlist a statement about real destinations. A leading-dot or wildcard entry is rejected at
startup, and by the chart's values schema at install time, rather than silently matching nothing.

You must append:

- **your platform host, if self-hosted**: e.g. `gitlab.example.com`
- **the operator's own webhook host, if `webhook.sync` is enabled**: the host of
  `webhook.ingress`/`webhook.route`, or of `spec.webhook.baseUrl`. The chart cannot infer this,
  because the delivery host is whatever your platform must be able to reach.

An empty list refuses every destination. That is also the default when the operator runs from raw
manifests rather than the chart, so a deployment that has not configured this fails closed.

### What a denial looks like

A refused RenovateJob is halted: it is not scheduled, no discovery or executor Job is created for
it, and no webhooks are written. The reason is recorded on the resource itself rather than only in
the operator log:

```console
$ kubectl get renovatejobs
NAME      SCHEDULE      PROVIDER   ACCEPTED
my-job    0 * * * *     gitlab     False

$ kubectl describe renovatejob my-job
Status:
  Conditions:
    Type:     Accepted
    Status:   False
    Reason:   DestinationNotAllowed
    Message:  spec.provider.endpoint: host "gitlab.internal" is not allowed;
              add it to policy.allowedHosts (allowed: api.github.com, github.com, ...)
```

`kubectl get renovatejobs -o wide` adds the reason column. The web UI halts the job's card with the
same message and disables its run controls, so the dashboard does not offer actions the policy will
refuse.

Condition reasons: `DestinationNotAllowed`, `InvalidDestinationURL`, `SecretRefNotOptedIn`,
`ServiceAccountNotAllowed`, `RootUserNotAllowed`, `ImageNotAllowed`, `PolicySatisfied`.
`renovate_operator_policy_denials_total{check="destination"}` increments on each denial.

Fixing the configuration clears the condition on the next reconcile (within a minute) and the job
resumes. Nothing has to be recreated.

A RenovateJob that the operator has not reconciled since the upgrade carries no condition yet; that
is reported as accepted, so a rollout does not black out every card in the UI before the first
reconcile lands.

Two deliberate asymmetries:

- **Webhook removal is never gated.** A hook written under a hostile `baseUrl` is exactly the one
  that needs deleting, and during removal the delivery URL is matching input rather than a
  destination. Only webhook *writes* require an allowlisted host.
- **Webhook sync fails open.** A denial skips the sync and is logged; it never blocks discovery or
  Renovate runs.

## Secret references must opt in

Some spec fields name a secret **and the key to read from it**. Because the operator does the
reading with its own cluster credentials, an unconstrained reference of that shape lets anyone who
can edit a RenovateJob resolve any value in the namespace (a database password, a cloud
credential) regardless of what they themselves are allowed to read.

Three fields are arbitrary-key references:

| Spec field | Keys it names |
|---|---|
| `spec.webhook.sync.secretRef` | `key` (the platform token used for webhook management) |
| `spec.webhook.authentication.secretRef` | `key` (the webhook authentication token) |
| `spec.githubAppReference` | `appIdSecretKey`, `installationIdSecretKey`, `pemSecretKey` |

A secret targeted by any of them must opt in:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: renovate-webhook-auth
  labels:
    renovate-operator.mogenius.com/allow-ref: "true"
stringData:
  token: ...
```

Only the exact value `"true"` opts in. The check runs after the secret is fetched but before any key
is read, so a refused reference never puts secret data in the operator's memory, and the refusal
never echoes the value it protected.

`spec.secretRef` is deliberately **not** covered. It is read only at Renovate's own well-known token
key names (`RENOVATE_TOKEN`, `GITHUB_COM_TOKEN`, `GITLAB_TOKEN`, `BITBUCKET_TOKEN`, `GITEA_TOKEN`,
`FORGEJO_TOKEN`), so it cannot be pointed at an arbitrary value, and requiring a label there would
break every installation for no gain.

Set `policy.requireSecretRefOptIn: false` to disable the requirement. The operator logs an error at
startup when it is off, because that restores the original exposure.

## Renovate config files and ConfigMap access

`spec.renovateConfig` supplies a Renovate configuration file to every Job pod
([renovate-config.md](renovate-config.md)).

Because renovate allows you to supply the config as a JavaScript file, it is automatically
also a vector for arbitrary code execution. 

This feature also requires the operator to have certain extra permissions to interact with ConfigMaps:

- `get`, `create`, `update` and `delete` on ConfigMaps in the job's namespace
- No `list` or `watch` permissions on ConfigMaps, so the operator can only touch ConfigMaps it can name.


## What the Renovate pod may be

`create`/`patch` on `renovatejobs` is roughly equivalent to `create` on `pods` in the same
namespace, because most of the spec becomes a pod spec. These controls bound how far that reaches.

### Rejected outright, by the CRD

The API server refuses these at `kubectl apply`, and no operator setting re-enables them:

| Field | Why |
|---|---|
| `spec.extraVolumes[*].hostPath` | mounts the node's filesystem, and with it every credential on that node |
| `spec.securityContext.container.privileged` | equivalent to root on the host |
| `spec.securityContext.container.allowPrivilegeEscalation` | the same, one step removed |

No documented Renovate use case needs any of them; see [extra-volumes.md](../configuration/extra-volumes.md) for
what the supported volume types cover.

### Governed by operator policy

| Control | Default | What it bounds |
|---|---|---|
| `policy.allowedServiceAccounts` | `[]` (namespace default only) | `spec.serviceAccount.name`. Naming a ServiceAccount is how a job borrows another workload's identity, including the operator's own, which can read secrets |
| `policy.allowRootUser` | `false` | `runAsUser: 0` and `runAsNonRoot: false` on either securityContext |
| `policy.allowedImages` | the official Renovate repositories | `spec.image` |

```yaml
policy:
  # only needed if your Renovate pods run as a specific ServiceAccount,
  # e.g. for cloud workload identity
  allowedServiceAccounts:
    - renovate-workload-identity
  allowRootUser: false
  allowedImages:
    - docker.io/renovate/renovate
    - ghcr.io/renovatebot/renovate
```

These refusals surface as the `Accepted` condition, with reasons `ServiceAccountNotAllowed`,
`RootUserNotAllowed` and `ImageNotAllowed`.

### Which image may run

Each `policy.allowedImages` entry authorises **exactly the repository it names**. There is no
prefix or subpath matching: listing `ghcr.io/renovatebot` does not permit
`ghcr.io/renovatebot/renovate`, and listing `renovate/renovate` does not permit
`renovate/renovate-evil` or `renovate/renovate/extra`. An entry can never authorise a repository
nobody enumerated.

The tag or digest on `spec.image` is ignored (only the repository is compared), so
`renovate/renovate:43.104.1`, `renovate/renovate` and `renovate/renovate@sha256:…` all match the
entry `renovate/renovate`.

Because the comparison is literal, **each spelling you use has to be listed**. The chart ships both
Docker Hub forms plus GHCR:

```yaml
policy:
  allowedImages:
    - renovate/renovate            # the short form the docs use
    - docker.io/renovate/renovate  # the same image, written out
    - ghcr.io/renovatebot/renovate
```

That redundancy is deliberate. Resolving `renovate/renovate` and `docker.io/renovate/renovate` to
one canonical string would mean the operator deciding which references are equivalent, and every
such rule is another chance to map an unexpected input onto an allowlist entry. Listing the forms
you actually use keeps the set of runnable images exactly what you wrote.

Add your own entry for an internal registry mirror, or for `index.docker.io/…` if that is the form
your specs use:

```yaml
policy:
  allowedImages:
    - registry.internal:5000/mirror/renovate
```

A reference the operator cannot parse (malformed, or with an uppercase repository path that no
registry would accept) is refused rather than guessed at.

Be clear-eyed about what this buys. It is the weakest of these controls on its own: the official
Renovate image is a Node runtime whose configuration a RenovateJob controls through `spec.extraEnv`,
and `RENOVATE_CONFIG_FILE` can point at an executable JavaScript config. Restricting the image raises
the cost of running arbitrary code; it does not remove it. What bounds that code is everything above:
no `hostPath`, no privileged, no privilege escalation, no borrowed ServiceAccount, and no secret
the operator has not been told it may read.

### Hardened defaults are merged, not replaced

The operator runs Renovate pods as uid/gid 12021, non-root, with the `RuntimeDefault` seccomp
profile and all capabilities dropped. `spec.securityContext` **overlays** those defaults: setting
`fsGroup` alone changes `fsGroup` and leaves `runAsNonRoot`, the seccomp profile and the dropped
capabilities in place. Anything you set wins; anything you omit stays hardened.

## Auditing an existing installation

The destination policy stops *new* damage; it cannot undo what an unbounded `spec.webhook.baseUrl`
already wrote. On a cluster that ran an earlier version:

1. List the webhooks on your repositories and look for delivery hosts you do not recognise. The
   operator's own hooks carry `namespace=` and `job=` query parameters.
2. Rotate any webhook authentication token that was ever synced to GitLab, Gitea or Forgejo; those
   platforms store it in cleartext on the hook, so it was delivered to whatever host the hook
   pointed at.
3. Rotate the platform token in `spec.secretRef` if `spec.provider.endpoint` ever pointed somewhere
   unexpected.
