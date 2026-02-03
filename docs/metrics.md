# Metrics

Activate metric-export using Prometheus by setting the following Helm values:

```yaml
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
```

## Exported Metrics

| Name                                       | Type    | Description                                                              | Labels                                                    |
|--------------------------------------------|---------|--------------------------------------------------------------------------|-----------------------------------------------------------|
| renovate_operator_project_executions_total | Counter | Total number of executed Renovate projects                               | `renovate-namespace`, `renovate-job`, `project`, `status` |
| renovate_operator_run_failed               | Gauge   | Whether the last Renovate run for this project failed (1=failed, 0=success) | `renovate-namespace`, `renovate-job`, `project`           |
| renovate_operator_dependency_issues        | Gauge   | Whether the last Renovate run had WARN/ERROR log entries (1=issues, 0=clean) | `renovate-namespace`, `renovate-job`, `project`           |

## Dependency Issues Detection

The `renovate_operator_dependency_issues` metric detects issues by parsing Renovate's JSON log output. This includes:

- Configuration validation warnings
- Dependency lookup failures
- Rate limiting issues
- Invalid `renovate.json` configuration

**Important**: This metric requires Renovate to output logs in JSON format. The operator sets `LOG_FORMAT=json` by default for all Renovate jobs. If you override this via `extraEnv` in your RenovateJob spec, the `renovate_operator_dependency_issues` metric will not function correctly and will always report 0.

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
```
