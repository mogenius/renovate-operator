# Templated Pod Labels

The operator can attach extra labels to every Renovate Job/Pod it spawns
(discovery and executor), with values built from templates instead of static
strings. This is useful for tools that group or bill workloads by label (e.g.
FinOps/cost-allocation tools), where the label value needs to encode which
RenovateJob, project, or run type produced the workload.

This is operator-wide configuration, applied to every RenovateJob and
project the same way - it is not a per-`RenovateJob` field.

## Configuration

Set `config.podLabelTemplates` in the Helm chart to a map of label key to
template string:

```yaml
# values.yaml
config:
  podLabelTemplates:
    perfectscale.io/workload-grouping-workload-name: "{jobType}-{job}-{namespace}-{project}"
```

Leaving it empty (the default, `{}`) attaches no extra labels, so existing
installations are unaffected.

## Placeholders

| Placeholder   | Value                                      | Empty when |
|---------------|---------------------------------------------|------------|
| `{job}`       | The `RenovateJob` resource's name           | never |
| `{jobType}`   | `discovery` or `executor`                   | never |
| `{namespace}` | The namespace the Job runs in               | never |
| `{project}`   | The project slug (e.g. `org/repo`)          | discovery jobs (there's no project yet) |

Given the example above, a project `foo` in namespace `my-organization` run
by `RenovateJob` `github-renovate` produces:

- executor job: `executor-github-renovate-my-organization-foo`
- discovery job: `discovery-github-renovate-my-organization`

Note the discovery label has no trailing separator even though `{project}`
is empty - see below.

## Value sanitization

Rendered values are automatically turned into valid Kubernetes label values:

- Characters outside `[A-Za-z0-9_.-]` (e.g. the `/` in a project slug, or
  literal `{`/`}` from a mistyped placeholder) are replaced with `-`.
- Runs of separator characters (`-`, `_`, `.`) - including ones left behind
  by an empty placeholder - collapse into a single `-`.
- Leading/trailing separators are trimmed.
- The result is truncated to 63 characters (the Kubernetes label value
  limit).

## Running without Helm

If you run the operator directly, set the `POD_LABEL_TEMPLATES` environment
variable to a JSON object:

```sh
POD_LABEL_TEMPLATES='{"perfectscale.io/workload-grouping-workload-name":"{jobType}-{job}-{namespace}-{project}"}'
```
