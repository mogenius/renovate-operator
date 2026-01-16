# Metrics

Activate metric-export using Prometheus by setting the following Helm values:

```yaml
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
```

## Exported Metrics

| Name                                       | Type    | Description                                | Labels                                                    |
|--------------------------------------------|---------|--------------------------------------------|-----------------------------------------------------------|
| renovate_operator_project_executions_total | Counter | Total number of executed Renovate projects | `renovate-namespace`, `renovate-job`, `project`, `status` |
