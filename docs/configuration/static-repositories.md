# Static Repositories

Autodiscovery asks your Git platform which repositories exist. When you already
know the exact list, `spec.repositories` skips that scan: the operator creates a
`RenovateProject` for each entry and runs Renovate against those projects
directly.

This is useful for small, fixed sets of repositories, for platforms where listing
projects is expensive, or when you want the managed set to be explicit in GitOps.

## Configuration

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-platform
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  provider:
    name: github
  secretRef: renovate-secret
  repositories:
    - acme/api
    - acme/web
    - acme/platform/infrastructure
```

Each entry is the platform's full project path — the same value Renovate accepts
on the command line. For GitLab that includes the full group and subgroup path;
for GitHub it is `owner/repo`.

The list must contain between 1 and 1000 entries. Entries are trimmed and
deduplicated; a duplicate is created only once.

## How it works

With a static list, the operator never runs a discovery job. Instead, on every
reconcile and on every scheduled run it compares the list with the existing
`RenovateProject` resources and:

1. Creates a `RenovateProject` for each new entry.
2. Deletes `RenovateProject` resources whose repository is no longer listed.
3. Leaves existing projects and their run history untouched.

A scheduled run also marks every non-running project as `Scheduled`, just like a
completed autodiscovery run would. Editing `spec.repositories` is picked up by the
reconciler without waiting for the next cron tick.

Because the list is authoritative, `spec.repositories` cannot be combined with
`discoveryFilters` or `discoverTopics`. It also cannot be used with
`skipForks: true` or `skipPendingDeletion: true`; explicit `false` values are
allowed. The CRD rejects invalid `RenovateJob` specs, and the operator also
refuses them after resolving a [shared template](./shared-templates.md).

## Dashboard

The job card shows the repositories in its configuration tooltip and hides the
**Run Discovery** button, since there is nothing to discover. The annotation
trigger and repository webhooks still work: they reconcile the static list into
`RenovateProject` resources, and newly listed projects are queued automatically.
Use the `schedule-all` annotation to requeue the rest.

## Relation to autodiscovery

| Behaviour                         | Autodiscovery                        | Static repositories           |
| --------------------------------- | ------------------------------------ | ----------------------------- |
| Source of the project list        | Scanned from the platform            | `spec.repositories`           |
| Discovery job                     | Runs on schedule                     | Never                         |
| Adding a repository               | Appears after the next discovery run | Appears on the next reconcile |
| Removing a repository             | Removed after the next discovery run | Removed on the next reconcile |
| `discoveryFilters`/topics         | Supported                            | Not allowed                   |
| `skipForks`/`skipPendingDeletion` | Supported                            | `true` not allowed            |

Use [autodiscovery](./autodiscovery.md) when the set of repositories changes
often or is large; use `spec.repositories` when it is small and known.
