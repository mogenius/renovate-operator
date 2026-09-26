# Shared Configuration Templates

When you run many `RenovateJob`s that share most of their configuration (image,
provider, `extraEnv`, resources, schedule, GitHub App reference, …), put the
shared parts in a template and let each job inherit them, overriding only what
differs.

## The two template kinds

| Kind                          | Scope        | Referenced by                                   |
| ----------------------------- | ------------ | ----------------------------------------------- |
| `RenovateJobTemplate`         | Namespaced   | `RenovateJob`s in the **same** namespace        |
| `ClusterRenovateJobTemplate`  | Cluster-wide | `RenovateJob`s in **any** namespace             |

Both wrap a `spec` with the exact shape of a `RenovateJob`'s `spec`. Set any
subset of fields; leave the rest out.

## Referencing a template

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJobTemplate
metadata:
  name: default-settings
  namespace: renovate
spec:
  image: renovate/renovate:41
  parallelism: 2
  provider:
    name: github
  extraEnv:
    - name: LOG_LEVEL
      value: info
  githubAppReference:
    secretName: renovate-github-app
    appIdSecretKey: appId
    installationIdSecretKey: installationId
    pemSecretKey: privateKey
---
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: team-a
  namespace: renovate
spec:
  templateRef:
    name: default-settings        # kind defaults to RenovateJobTemplate
  schedule: "*/15 * * * *"        # per-job
  discoveryFilters:               # per-job
    - team-a/*
```

Reference a cluster-scoped template by naming its kind:

```yaml
spec:
  templateRef:
    kind: ClusterRenovateJobTemplate
    name: org-baseline
```

A namespaced `templateRef` always resolves in the `RenovateJob`'s own namespace;
there is no cross-namespace reference.

## Merge rules

Inheritance is resolved **per top-level field**:

- A field the `RenovateJob` leaves unset is inherited from the template.
- A field the `RenovateJob` sets **replaces** the template's value wholesale.
  There is no deep merge: a job's `extraEnv` replaces the template's entire
  `extraEnv` list, it does not append to it.

> **Nested objects do not compose.** Inheritance is at the *top level only*. Setting
> any part of a nested object replaces the whole object, not just the part you set.
> If the template defines `access.readerGroups` and the job defines
> `access.adminGroups`, the job's `access` replaces the template's entirely and the
> inherited `readerGroups` is **lost** — the effective job has only `adminGroups`.
> The same holds for `securityContext` (setting `.container` drops an inherited
> `.pod`), `renovateConfig`, `provider`, `webhook`, and every other object-valued
> field. To combine values, repeat the inherited ones on the job.
- `skipForks`, `skipPendingDeletion` and `suspend` are booleans that inherit when
  unset; set any of them to `false` explicitly to override an inherited `true`.
  A template's `suspend: true` therefore pauses every job using it, see
  [Suspending a RenovateJob](../operations/suspend.md).
- `access` and the deprecated `allowedGroups` are two spellings of the same access
  control. A job that sets **either** replaces the template's access control
  entirely, dropping the inherited spelling too, so the effective job never carries
  both. The operator-wide access defaults still apply underneath, so the resolution
  order is **operator defaults → template → job**.

`schedule`, `image`, `provider` and `parallelism` are required on the *effective*
job. Supply them on the `RenovateJob`, the template, or both — but if none does,
the job is refused (see below).

## References inside a template

`secretRef`, `githubAppReference` and `webhook.*.secretRef` carried by a template
resolve in the **consuming job's namespace**. A `ClusterRenovateJobTemplate` that
names `secretRef: renovate-secret` therefore expects a Secret of that name to
exist in each job's namespace. The operator's policy checks (secret opt-in via the
`allow-ref` label, allowed hosts) apply to the effective job exactly as if the
fields had been written on the `RenovateJob` directly.

## Propagation and failures

- Editing a template immediately re-reconciles every `RenovateJob` that references
  it — schedules, tokens and config maps are refreshed.
- A `templateRef` that does not resolve, or an effective spec still missing a
  required field, sets the job's `Accepted` condition to `False`
  (`TemplateNotFound` / `IncompleteSpec`) and stops it running until fixed:

  ```
  kubectl get renovatejob team-a -o jsonpath='{.status.conditions}'
  ```

- Deleting a template that is still referenced is **not** blocked. Its dependents
  flip to `Accepted=False` until the template returns or they are re-pointed.

## RBAC

The Helm chart grants the operator read access to both template kinds. In
`rbac.ownNamespaceOnly` installs the namespaced `RenovateJobTemplate` is covered
by the namespaced Role, and a small dedicated ClusterRole grants read on the
cluster-scoped `ClusterRenovateJobTemplate`.
