# PR Activity

After each Renovate run, the operator parses the job's logs and records PR activity in the project's CRD status under `.status.projects[*].prActivity`.

> **Note:** PR activity parsing requires Renovate to run with debug logging enabled (`RENOVATE_LOG_LEVEL=debug` via `extraEnv`). Without debug output, log messages such as `branches info extended` are not emitted and the operator cannot extract per-PR details.

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

## Log issues

The same log parsing records the warnings and errors of the run under `.status.projects[*].logIssues`. Unlike PR activity, it does not need debug logging: Renovate logs warnings and errors at its default log level.

```yaml
status:
  projects:
    - name: org/repo
      logIssues:
        warnCount: 3          # log lines at level 40 (warn)
        errorCount: 1         # log lines at level 50 (error) or 60 (fatal)
        categoryCounts:       # every warn/error line by category; adds up to warnCount + errorCount
          lookup: 2
          config: 1
          platform: 1
        hasConfigMigration: true  # only set when the run logged at debug level
        truncated: false      # true if more than 20 distinct messages were found
        issues:
          - level: 40
            message: Package lookup failures
            category: lookup
          - level: 60
            message: Authentication failure
            category: platform
```

- `issues` keeps the first **20 distinct** messages, each cut at **256 bytes** with a trailing `…`. Repeated messages are stored once but counted every time.
- `warnCount`, `errorCount` and `categoryCounts` count **every** warning and error line, including the ones `issues` leaves out.
- `hasConfigMigration` comes from a debug-level message (`Config migration necessary`). It is not a warning, and is not counted in `warnCount`.

### Categories

Each warning and error is filed under one category, decided from its message (see `src/internal/parser/issueCategory.go`). The rules are tried in the order below, and the first match wins. So "config option X is deprecated" is a deprecation, and "registry rate limit reached" is a platform limit, not a lookup failure.

| Category | Meaning | Example messages |
|---|---|---|
| `deprecation` | Works today, will stop working in a later Renovate version | `Updating multiple npm lock files is deprecated…` |
| `config` | The repository's Renovate config, or a preset it extends | `Found renovate config warnings`, `Cannot find preset's package` |
| `platform` | Authentication, permissions or rate limits of the Git platform or a registry | `Authentication failure`, `403 Forbidden`, `Registry rate limit reached` |
| `lookup` | A dependency's versions could not be looked up | `Package lookup failures`, `Failed to look up npm package …` |
| `branches` | Creating or updating a branch, a lock file or another artifact failed | `Error updating branch: update failure`, `Artifact update error …`, `Command failed: npm install …` |
| `other` | Anything else, including lines without a message | `Ignoring pinned dependency …` |

Statuses written by an operator version before categories existed have neither `categoryCounts` nor `category`. The dashboard treats their issues as `other` until the project runs again.

### Filtering in the dashboard

The **With Issues** counter selects every project with at least one warning or error. While it is selected, a bar below the counters narrows the list by three facets:

| Facet | Values | Decided by |
|---|---|---|
| Severity | Errors only (fatal counts as an error) | `errorCount > 0` |
| Category | Config, Deprecations, Platform & access, Lookup failures, Branches & artifacts, Other | `categoryCounts` |
| Repository | Onboarded, Onboarding, Onboarding closed, Disabled, No config | `renovateResultStatus` |

- Facets combine with AND; values within one facet combine with OR. Each chip shows how many projects picking it would leave, given the other facets.
- Every facet is decided per project. `categoryCounts` is not split by level, so **Errors only** together with a category means "the project has errors, and has issues in that category", not that those issues are errors.
- The selection is kept in the URL, so a filtered view can be shared or bookmarked:

  ```text
  /?filter=withIssues&issues=lookup,config&severity=error&repo=onboarding
  ```

  `issues` takes category keys, `severity=error` selects errors only, and `repo` takes `active`, `onboarding`, `onboardingClosed`, `disabled` or `noConfig`. Unknown values are ignored.
- Leaving **With Issues**, or pressing <kbd>Esc</kbd>, drops the refinement.

Each project row shows its categories next to the warn/error counts. The expanded issue list labels each message with its category, and level-60 messages as `fatal`.
