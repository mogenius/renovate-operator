# Timezone Configuration

By default the operator interprets cron schedules in UTC. Two approaches let you use a different timezone.

## Per-schedule: `CRON_TZ=` prefix

Prefix the `spec.schedule` field with `CRON_TZ=<timezone>` to set a timezone for that specific RenovateJob. The timezone is applied only to that schedule and has no effect on any other job.

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-my-org
  namespace: renovate-operator
spec:
  schedule: "CRON_TZ=Europe/Copenhagen 0 5 * * *"  # runs at 05:00 Copenhagen time
  image: renovate/renovate:latest
  secretRef: "renovate-secret"
  parallelism: 1
```

Use any [IANA timezone name](https://en.wikipedia.org/wiki/List_of_tz_database_time_zones), for example:
- `Europe/Berlin`
- `America/New_York`
- `Asia/Tokyo`
- `UTC`

## Global default: `TZ` environment variable via `extraEnv`

Set the `TZ` environment variable on the operator pod to change the default timezone for all schedules that do not carry an explicit `CRON_TZ=` prefix.

```yaml
extraEnv:
  - name: TZ
    value: "Europe/Copenhagen"
```

Schedules with an explicit `CRON_TZ=` prefix always override this global default.

> **Note:** `TZ` only affects the operator process — it has no effect on the Renovate executor jobs themselves.
