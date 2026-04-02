# PR Activity

After each Renovate run, the operator parses the job's logs and records PR activity in the project's CRD status under `.status.projects[*].prActivity`.

> **Note:** PR activity parsing requires Renovate to run with debug logging enabled (`LOG_LEVEL=debug` via `extraEnv`). Without debug output, log messages such as `branches info extended` are not emitted and the operator cannot extract per-PR details.

## Status fields

```yaml
status:
  projects:
    - name: org/repo
      prActivity:
        automerged: 1   # PRs automatically merged by Renovate
        created: 2      # New PRs opened in this run
        updated: 0      # Existing PRs updated with new commits
        unchanged: 3    # PRs checked but requiring no update
        truncated: false # true if > 100 PRs were found (list is capped)
        prs:
          - branch: renovate/dependency-1.x
            action: automerged
            number: 42
            url: https://github.com/org/repo/pull/42
            title: "Update dependency to v1.2.3"
```

### PR actions

| Action | Description |
|---|---|
| `automerged` | PR was automatically merged by Renovate |
| `created` | A new PR was opened in this run |
| `updated` | An existing PR was updated with new commits |
| `unchanged` | PR was checked but required no changes |

## Per-PR details

Each entry in `.prActivity.prs` contains:

- `branch` — the Renovate branch name
- `action` — one of the actions above
- `number` — PR/MR number on the forge
- `url` — direct link to the PR/MR (backfilled from a single observed URL when the forge returns it only on push)
- `title` — PR title

The list is sorted by action priority (automerged → created → updated → unchanged), then alphabetically by branch name, and capped at **100 entries** per run to prevent CRD bloat. When the cap is reached, `truncated` is set to `true`.

`prActivity` is `null` when no parseable log lines were found (e.g. the job failed before Renovate produced any output, or debug logging was not enabled).
