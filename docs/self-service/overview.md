# Self-Service Onboarding

Repository owners add themselves to Renovate by tagging a repository on the Git platform. No
cluster access, no pull request against your GitOps repo, no ticket to the team that runs the
operator.

The model is a clean split of responsibility:

- **The Git platform owns _which_ repositories are managed.** A topic on a repo is the whole
  opt-in, and the people who own the repo can set it themselves.
- **The cluster owns _how_ they are managed.** Schedule, parallelism, credentials, security policy
  and image live in the `RenovateJob` and stay with the operator's admins.

The two never have to meet. Once a `RenovateJob` is configured with `discoverTopics`, onboarding a
repository is a platform action, and the operator reconciles the rest: it discovers the repo,
queues it, runs Renovate against it, and — with webhook sync on — installs the webhook so
Dependency Dashboard checkboxes work.

## Topic-based self-service

### Prerequisites

A `RenovateJob` configured with `discoverTopics`. Everything below assumes a job like this:

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  discoverTopics:
    - renovate
  parallelism: 3
  secretRef: renovate-secret
  image: ghcr.io/renovatebot/renovate:latest
  provider:
    name: github
  webhook:
    enabled: true
    sync:
      enabled: true
```

`discoverTopics` is a list; entries are joined with `,` and passed to Renovate as
`RENOVATE_AUTODISCOVER_TOPICS`. Pick a topic name that is unlikely to collide with anything else in
your organisation — `renovate` is the obvious choice.

> **Platform support:** topics are what GitHub calls *topics*, GitLab calls *topics*, and Gitea and
> Forgejo call *topics*. Bitbucket has no equivalent — use
> [`discoveryFilters`](../configuration/autodiscovery.md#using-discovery-filter) there instead.

### Onboarding a repository

#### 1. Tag the repository

The repository owner adds the topic on the platform — no cluster access needed.

- **GitHub** — repo page → ⚙️ next to *About* → *Topics* → add `renovate` → *Save changes*
- **GitLab** — *Settings* → *General* → *Topics* → add `renovate` → *Save changes*
- **Gitea / Forgejo** — repo page → *Manage topics* → add `renovate` → *Save*

#### 2. Wait for the next discovery run, or trigger one immediately

Discovery runs on `spec.schedule`. To onboard without waiting for the next cron tick, annotate the
job:

```sh
kubectl annotate renovatejob renovate -n renovate-operator \
  renovate-operator.mogenius.com/discovery=true
```

The operator picks the annotation up on its next reconcile (within ~1 minute), starts the discovery
job, and removes the annotation. See [Annotation Triggers](./annotation-triggers.md).

#### 3. The operator does the rest

Once the discovery job finishes:

1. The new repository appears in the `RenovateJob` status and in the UI.
2. It is added with status `scheduled` — newly discovered projects are queued automatically.
3. The executor dispatches it on its next tick (within 10 seconds), subject to `spec.parallelism`
   and the global parallelism limit.
4. If `webhook.sync` is enabled, the operator installs its webhook on the repository so Dependency
   Dashboard and PR rebase checkboxes trigger runs on demand. See
   [Automatic Webhook Sync](../webhooks/sync.md).

From the repo owner's point of view: add a topic, and the onboarding PR shows up.

### Offboarding a repository

Remove the topic. On the next discovery run the repository drops out of the project list, and the
operator removes it from the `RenovateJob` status and the UI, deletes its metrics, and removes the
webhook it installed (webhook sync only).

Renovate stops opening PRs against it. Existing PRs are left open — close them yourself if you
don't want them.

### Excluding a repository that matches the topic anyway

Topics can be negated with `!`, which is useful when a topic is applied broadly and a few repos
should opt out:

```yaml
discoverTopics:
  - renovate
  - "!renovate-ignore"
```

A repository tagged with both `renovate` and `renovate-ignore` is skipped. Quote entries starting
with `!` in YAML.

### Troubleshooting

**The repo didn't show up after discovery.**

- Confirm the topic is spelled exactly as in `discoverTopics` — matching is exact and
  case-sensitive on most platforms.
- Confirm the token in `spec.secretRef` can see the repository. Autodiscovery only returns repos
  the token has access to; a private repo in an org the bot account was never added to is
  invisible, not filtered.
- Check the discovery job's logs:

```sh
kubectl logs -n renovate-operator \
  -l renovate-operator.mogenius.com/type=discovery --tail=100
```

**The repo showed up but never ran.** Check whether the `RenovateJob` is halted by the policy
engine — the UI shows a banner and the run controls are disabled. The `Accepted` condition on the
resource shows the reason:

```sh
kubectl get renovatejob renovate -n renovate-operator \
  -o jsonpath='{.status.conditions}'
```

See [Security](../security/security.md).

**The repo ran but got no webhook.** Webhook sync fails open — it logs and continues rather than
blocking discovery. The usual cause is that the platform token has push access (enough for
Renovate) but not admin/Maintainer (needed to manage webhooks). See
[Permissions](../webhooks/sync.md#permissions).

**It ran, but Renovate found nothing to do.** That is a Renovate-level concern, not an operator
one — check the repository's own `renovate.json` and the Dependency Dashboard issue.

## Multi-tenant RenovateJob creation

For teams that want repo owners to have finer control — their own schedule, their own credentials,
their own parallelism — you can grant users the ability to create `RenovateJob` objects directly
in their own namespaces using Kubernetes RBAC.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: renovatejob-creator
  namespace: my-team
rules:
  - apiGroups: ["renovate-operator.mogenius.com"]
    resources: ["renovatejobs"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: renovatejob-creator
  namespace: my-team
subjects:
  - kind: User
    name: alice
    apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: renovatejob-creator
  apiGroup: rbac.authorization.k8s.io
```

Users then create a `RenovateJob` in their own namespace just like the operator's admins do. The
policy engine still applies — `spec.provider.endpoint`, `spec.webhook.baseUrl`, and
`spec.secretRef`-referenced secrets are validated against the operator's allow-list, so a user
cannot point Renovate at an arbitrary host or read secrets they don't own. See
[Security](../security/security.md) for the full list of restrictions.

Keep in mind that each `RenovateJob` runs its own discovery cycle and holds its own credential
reference, so this model works best when teams are already separated by namespace and each team
manages a distinct set of repositories or platforms.

## Related

- [Autodiscovery](../configuration/autodiscovery.md) — filters, topics, fork and pending-deletion filtering
- [Annotation Triggers](./annotation-triggers.md) — on-demand discovery and scheduling
- [Automatic Webhook Sync](../webhooks/sync.md) — per-repo webhooks without manual setup
- [Scheduling](../configuration/scheduling.md) — schedule, parallelism, and priority
- [Security](../security/security.md) — policy engine, destination allow-list, secret opt-in
