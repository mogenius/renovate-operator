# Metrics

Activate metric-export using Prometheus by setting the following Helm values:

```yaml
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
```

All counters and histograms are also emitted via OpenTelemetry when an OTel meter is
configured (`OTEL_EXPORTER_OTLP_ENDPOINT`), using the dotted equivalent of each name
(e.g. `renovate_operator.job.duration`) and the same label dimensions. Gauges are
Prometheus-only.

In addition to the metrics below, the operator exposes controller-runtime's built-in
metrics for free (`controller_runtime_reconcile_*`, workqueue depth/latency,
`leader_election_master_status`).

## Execution and job lifecycle

| Name                                          | Type      | Description                                                                 | Labels                                                    |
|-----------------------------------------------|-----------|-----------------------------------------------------------------------------|-----------------------------------------------------------|
| renovate_operator_executor_loop_duration_seconds | Histogram | Duration of a single executor loop tick                                  | (none)                                                    |
| renovate_operator_project_executions_total    | Counter   | Total number of executed Renovate projects                                  | `renovate_namespace`, `renovate_job`, `project`, `status` |
| renovate_operator_jobs_dispatched_total       | Counter   | Kubernetes Jobs launched by the operator                                    | `renovate_namespace`, `renovate_job`, `kind`              |
| renovate_operator_job_duration_seconds        | Histogram | Wall-clock duration of a Renovate Kubernetes Job                            | `renovate_namespace`, `renovate_job`, `kind`, `status`    |
| renovate_operator_project_queue_wait_seconds  | Histogram | Time a project spent in Scheduled before being dispatched                   | `renovate_namespace`, `renovate_job`                      |
| renovate_operator_job_failures_total          | Counter   | Job failures by mode (`timeout`/`backoff_exceeded`/`job_not_found`/`pod_error`/`unknown`) | `renovate_namespace`, `renovate_job`, `kind`, `reason` |
| renovate_operator_run_failed                  | Gauge     | Whether the last run for this project failed (1=failed, 0=success)          | `renovate_namespace`, `renovate_job`, `project`           |
| renovate_operator_last_execution_duration_seconds | Gauge | Duration of the most recent run for this project                            | `renovate_namespace`, `renovate_job`, `project`           |

`kind` is `executor` or `discovery`.

## Saturation and queue depth

| Name                                          | Type  | Description                                              | Labels                               |
|-----------------------------------------------|-------|----------------------------------------------------------|--------------------------------------|
| renovate_operator_projects_scheduled          | Gauge | Projects currently in Scheduled (queue depth) per job    | `renovate_namespace`, `renovate_job` |
| renovate_operator_projects_running            | Gauge | Projects currently Running (in-flight) per job           | `renovate_namespace`, `renovate_job` |
| renovate_operator_global_running_projects     | Gauge | Total Running projects across all jobs                   | (none)                               |
| renovate_operator_global_parallelism_limit    | Gauge | Configured global parallelism limit (0 = unlimited)      | (none)                               |

## Discovery

| Name                                          | Type    | Description                                            | Labels                                          |
|-----------------------------------------------|---------|--------------------------------------------------------|-------------------------------------------------|
| renovate_operator_discovery_jobs_total        | Counter | Discovery Jobs completed by status                     | `renovate_namespace`, `renovate_job`, `status`  |
| renovate_operator_discovered_repositories     | Gauge   | Repositories seen by the last discovery run            | `renovate_namespace`, `renovate_job`            |
| renovate_operator_repositories_filtered_total | Counter | Repositories dropped by filters (`fork`/`pending_deletion`) | `renovate_namespace`, `renovate_job`, `reason` |

## Scheduler

| Name                                                   | Type    | Description                                      | Labels                                          |
|--------------------------------------------------------|---------|--------------------------------------------------|-------------------------------------------------|
| renovate_operator_schedule_runs_total                  | Counter | Cron schedule firings executed by result (`success`/`error`) | `renovate_namespace`, `renovate_job`, `result` |
| renovate_operator_schedule_next_run_timestamp_seconds  | Gauge   | Unix timestamp of the next planned scheduled run | `renovate_namespace`, `renovate_job`            |

## Results and outcomes

| Name                                              | Type    | Description                                                | Labels                                          |
|---------------------------------------------------|---------|------------------------------------------------------------|-------------------------------------------------|
| renovate_operator_open_pull_requests              | Gauge   | Open Renovate-managed pull requests after the last run     | `renovate_namespace`, `renovate_job`, `project` |
| renovate_operator_pull_requests_awaiting_approval | Gauge   | Pull requests awaiting human approval after the last run   | `renovate_namespace`, `renovate_job`, `project` |
| renovate_operator_pull_requests_created_total     | Counter | Pull requests created                                      | `renovate_namespace`, `renovate_job`            |
| renovate_operator_pull_requests_merged_total      | Counter | Pull requests automerged (updates that landed)             | `renovate_namespace`, `renovate_job`            |
| renovate_operator_pull_requests_updated_total     | Counter | Pull requests updated                                      | `renovate_namespace`, `renovate_job`            |
| renovate_operator_repositories_by_status          | Gauge   | Repositories per Renovate result status (coverage). `status` is a bounded enum: `disabled`, `no_config`, `onboarding`, `onboarding_closed`, `unknown`, `other` | `renovate_namespace`, `renovate_job`, `status`  |
| renovate_operator_approvals_needed                | Gauge   | Dependency updates awaiting approval after the last run    | `renovate_namespace`, `renovate_job`, `project` |

## Log quality

| Name                                  | Type  | Description                                                                  | Labels                                                  |
|---------------------------------------|-------|------------------------------------------------------------------------------|---------------------------------------------------------|
| renovate_operator_log_issues          | Gauge | WARN/ERROR log entry counts in the last run, by `level` (`warn`/`error`)     | `renovate_namespace`, `renovate_job`, `project`, `level`|
| renovate_operator_dependency_issues   | Gauge | Whether the last run had WARN/ERROR log entries (1=issues, 0=clean)          | `renovate_namespace`, `renovate_job`, `project`         |

## Webhook integrity

| Name                                                            | Type    | Description                                                  | Labels                       |
|-----------------------------------------------------------------|---------|--------------------------------------------------------------|------------------------------|
| renovate_operator_webhook_requests_total                        | Counter | Webhook requests by `provider` and `result` (`accepted`/`rejected`/`ignored`) | `provider`, `result` |
| renovate_operator_webhook_signature_verification_failures_total | Counter | Webhook HMAC signature verification failures                 | `provider`                   |
| renovate_operator_webhook_auth_failures_total                   | Counter | Webhook auth failures by `error_type` (`no_matching_job`/`auth_failed`/`secret_error`) | `provider`, `error_type` |
| renovate_operator_webhook_payload_decode_failures_total         | Counter | Webhook payloads that failed to decode                       | `provider`                   |

`provider` is one of `github`, `gitlab`, `forgejo`, `schedule`.

## Credentials

| Name                                             | Type    | Description                                                                 | Labels       |
|--------------------------------------------------|---------|-----------------------------------------------------------------------------|--------------|
| renovate_operator_secret_resolution_errors_total | Counter | Kubernetes Secret resolution errors (`not_found`/`key_missing`/`api_error`) | `error_type` |

Security metric labels are deliberately bounded enums; user identifiers, IP addresses,
and raw request paths are never used as label values.

## Dependency Issues Detection

The `renovate_operator_dependency_issues` and `renovate_operator_log_issues` metrics
detect issues by parsing Renovate's JSON log output. This includes:

- Configuration validation warnings
- Dependency lookup failures
- Rate limiting issues
- Invalid `renovate.json` configuration

**Important**: These metrics require Renovate to output logs in JSON format. The operator
sets `RENOVATE_LOG_FORMAT=json` by default for all Renovate jobs. If you override this via
`extraEnv` in your RenovateJob spec, these metrics will not function correctly and will
always report 0.

## Example Prometheus Alerting Rules

```yaml
groups:
  - name: renovate-operator
    rules:
      - alert: RenovateRunFailed
        expr: renovate_operator_run_failed == 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Renovate run failed for {{ $labels.project }}"
          description: "The last Renovate run for project {{ $labels.project }} in job {{ $labels.renovate_job }} failed."

      - alert: RenovateDependencyIssues
        expr: renovate_operator_dependency_issues == 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Renovate detected dependency issues for {{ $labels.project }}"
          description: "The last Renovate run for project {{ $labels.project }} in job {{ $labels.renovate_job }} had warnings or errors. Check the Dependency Dashboard or Renovate logs for details."

      # SRE: runs taking dangerously close to the 30m default job timeout.
      - alert: RenovateJobDurationHigh
        expr: histogram_quantile(0.95, sum(rate(renovate_operator_job_duration_seconds_bucket[1h])) by (le, renovate_job)) > 1500
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "Renovate job p95 duration is high for {{ $labels.renovate_job }}"
          description: "p95 job duration is approaching the 1800s (JOB_TIMEOUT_SECONDS) default."

      # SRE: automerge has flatlined while the open-PR backlog keeps growing.
      - alert: RenovateAutomergeStalled
        expr: sum(rate(renovate_operator_pull_requests_merged_total[6h])) == 0 and sum(renovate_operator_open_pull_requests) > 0
        for: 6h
        labels:
          severity: warning
        annotations:
          summary: "No Renovate PRs merged in 6h while open PRs exist"
          description: "Automerge may be broken or gated; the dependency-update backlog is not draining."

      # SRE: a schedule that should have fired is overdue (leader stall / requeue failure).
      - alert: RenovateScheduleMissed
        expr: time() - renovate_operator_schedule_next_run_timestamp_seconds > 3600
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Renovate schedule overdue for {{ $labels.renovate_job }}"
          description: "The next planned run is more than an hour in the past."

      # SecOps: sustained webhook HMAC signature failures - forged or misconfigured webhooks.
      - alert: RenovateWebhookSignatureFailures
        expr: sum(rate(renovate_operator_webhook_signature_verification_failures_total[10m])) by (provider) > 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Renovate webhook signature failures from {{ $labels.provider }}"
          description: "HMAC signature verification is failing; investigate forged or misconfigured webhooks."
```
