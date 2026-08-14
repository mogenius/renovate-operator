# Annotation Triggers

Annotation triggers let you imperatively kick off actions on a `RenovateJob` without waiting for the next scheduled cron run. Set an annotation on the resource and the operator picks it up on the next reconcile loop (within ~1 minute), executes the action, and removes the annotation — making each trigger a one-shot command.

> **Why annotations instead of labels?**
> Kubernetes label values cannot contain slashes (`/`), which makes them incompatible with typical project names like `org/repo`. Annotations have no such restriction.

## Available triggers

| Annotation                                    | Value                   | Effect                                              |
| --------------------------------------------- | ----------------------- | --------------------------------------------------- |
| `renovate-operator.mogenius.com/discovery`    | `"true"`                | Starts a discovery run to refresh the project list  |
| `renovate-operator.mogenius.com/schedule-all` | `"true"`                | Sets all non-running projects to `Scheduled`        |
| `renovate-operator.mogenius.com/schedule`     | `"org/repo1,org/repo2"` | Sets the listed non-running projects to `Scheduled` |

Multiple triggers can be set simultaneously — the operator processes all of them in a single reconcile.

## Trigger a discovery run

Useful when you have added repositories to your Git platform and want the operator to pick them up immediately rather than waiting for the next scheduled cron run.

```sh
kubectl annotate renovatejob <name> -n <namespace> \
  renovate-operator.mogenius.com/discovery=true
```

The operator starts a discovery job, which runs Renovate in autodiscover mode and reconciles the resulting project list into the `RenovateJob` status. The annotation is removed once the job is created successfully.

## Schedule all projects immediately

Marks every non-running project as `Scheduled` so the executor dispatches them on the next tick (within 10 seconds). Projects that are currently `Running` are left untouched.

```sh
kubectl annotate renovatejob <name> -n <namespace> \
  renovate-operator.mogenius.com/schedule-all=true
```

## Schedule specific projects

Marks a comma-separated list of projects as `Scheduled`. Only projects already present in the `RenovateJob` status are affected — projects not yet discovered are silently skipped. Projects that are currently `Running` are also skipped.

```sh
kubectl annotate renovatejob <name> -n <namespace> \
  "renovate-operator.mogenius.com/schedule=org/repo1,org/repo2"
```

Whitespace around commas is trimmed, so `"org/repo1, org/repo2"` works too.

## Combining triggers

All three can be set at once. The operator handles them in this order: discovery → schedule-all → schedule.

```sh
kubectl annotate renovatejob <name> -n <namespace> \
  renovate-operator.mogenius.com/discovery=true \
  renovate-operator.mogenius.com/schedule-all=true
```

This is useful when you want to refresh the project list **and** immediately queue everything that was already known.

## Overwriting an existing annotation

If you set the same trigger twice before the operator has processed the first one, use `--overwrite`:

```sh
kubectl annotate renovatejob <name> -n <namespace> \
  renovate-operator.mogenius.com/schedule-all=true \
  --overwrite
```

## Behaviour on failure

If an action fails (for example, a discovery job cannot be created because one is already running), the annotation is **not removed**. The operator logs the error and retries on the next reconcile (~1 minute). Once the action succeeds, the annotation is removed automatically.
