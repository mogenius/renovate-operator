# Suspending a RenovateJob

Set `spec.suspend` to pause a RenovateJob without stopping the operator, for example while its Git platform is down for maintenance:

```sh
kubectl patch renovatejob <name> -n <namespace> --type merge -p '{"spec":{"suspend":true}}'
```

`kubectl get renovatejobs` shows the state in its `SUSPEND` column, and the UI marks the job as suspended.

Where the operator enables it with `authorization.suspendFromUI: true` (`AUTHORIZATION_SUSPEND_FROM_UI`), admins of a job can also flip it from the UI with the job card's **Suspend** and **Resume** button. It writes the smallest change that gets there: the job's own `suspend` is cleared when its template, or the default, already gives the wanted state, and set explicitly only to override a template. Readers see the button disabled. Every change is logged with the user who made it. The button is off by default because it changes the live object, which a sync can undo: see [GitOps](#gitops).

## What stops

While a RenovateJob is suspended, the operator starts no Kubernetes Job for it:

- its schedule is removed, so no discovery runs on the cron;
- projects in `Scheduled` are not dispatched;
- a discovery requested from the UI is refused with `409 Conflict`, and a `renovate-operator.mogenius.com/discovery` annotation is kept until the job is resumed.

## What keeps working

- Runs already in progress finish, and their results are recorded as usual.
- Webhooks, the UI's trigger buttons and the `schedule` and `schedule-all` [annotation triggers](../self-service/annotation-triggers.md) still queue projects. They are dispatched once the job is resumed.
- The UI and the webhook endpoint stay up. Scaling the operator to zero also stops new runs, but takes both down with it, so webhook deliveries fail meanwhile and some platforms disable a webhook that keeps failing.

## Resuming

```sh
kubectl patch renovatejob <name> -n <namespace> --type merge -p '{"spec":{"suspend":false}}'
```

The schedule is registered again on the next reconcile, and every project still queued is dispatched from the next executor tick, within the usual parallelism limits. Schedule ticks missed while suspended are not replayed.

To suspend or resume every RenovateJob in a namespace:

```sh
kubectl get renovatejobs -n <namespace> -o name \
  | xargs -I{} kubectl patch {} -n <namespace> --type merge -p '{"spec":{"suspend":true}}'
```

Jobs that share a [template](../configuration/shared-templates.md) can be paused together by setting `suspend: true` on the template: every job using it is suspended, except one that sets `suspend: false` itself. The `SUSPEND` column only shows the job's own value, so a job suspended through its template shows it empty there, while the UI and the metric below report the effective state.

## GitOps

If a GitOps tool manages the RenovateJobs, pause them in the source where you can, on the jobs or on their template. A live change, from `kubectl` or the UI button, lasts only as long as the sync leaves `spec.suspend` alone:

- a sync that applies only the fields the source sets, like Argo CD's default apply or a server-side apply, keeps a live `suspend` the source does not set;
- a source that sets `suspend` itself puts its value back on the next sync, or at once with self-heal;
- a sync that replaces the whole object, like Argo CD's `Replace=true`, drops it.

To keep a live value in Argo CD where the source sets `suspend`, ignore the field and make the sync respect that:

```yaml
spec:
  ignoreDifferences:
    - group: renovate-operator.mogenius.com
      kind: RenovateJob
      jsonPointers:
        - /spec/suspend
  syncPolicy:
    syncOptions:
      - RespectIgnoreDifferences=true
```

## Monitoring

`renovate_operator_renovatejob_suspended` is `1` while a RenovateJob is suspended. Its `renovate_operator_schedule_next_run_timestamp_seconds` series is removed at the same time, so the `RenovateScheduleMissed` example alert does not fire for it. The `RenovateJobSuspended` example in [Metrics](./metrics.md#example-prometheus-alerting-rules) catches a job left suspended instead. A scheduled run that was already due when the job was suspended starts nothing and counts as `result="skipped"` in `renovate_operator_schedule_runs_total`.
