# Running a repository against a custom branch

A custom branch run is a one-off Renovate dry run of a single repository against a branch you name, using the Renovate config on that branch. It is meant for checking a config change against a real repository before you merge it into the base branch, without touching the config that every other run uses. Renovate only logs what it would change; the result is read in the project's logs.

## Starting a run

In the dashboard, each project's action button has a branch icon next to the debug (bug) icon. Click it, enter the branch name and choose **Run**. Tick **Debug log level** for the same extended logs as the debug trigger (`RENOVATE_LOG_LEVEL=debug`).

The run is queued with the same priority as any other dashboard trigger. While it is queued, the branch icon is highlighted and its tooltip names the branch. Once the Job starts, the options are cleared. Later runs, whether scheduled, triggered by a webhook or started from the dashboard, use the repository's normal configuration again.

Starting a custom branch run needs the `admin` role on the RenovateJob, just like the regular trigger button. See [Authentication](../configuration/auth.md). The **Trigger all** action does not support custom branches.

## What the run does

The operator adds these variables to the Renovate Job:

| Variable                                                     | Value          |
| ------------------------------------------------------------ | -------------- |
| `RENOVATE_BASE_BRANCH_PATTERNS` and `RENOVATE_BASE_BRANCHES` | `["<branch>"]` |
| `RENOVATE_USE_BASE_BRANCH_CONFIG`                            | `merge`        |
| `RENOVATE_DRY_RUN`                                           | `full`         |

Two variables carry the branch because Renovate renamed `baseBranches` to `baseBranchPatterns`. Renovate ignores variables it does not recognise, so setting both works with images from before and after the rename.

`useBaseBranchConfig: merge` makes Renovate read the config file on the custom branch and merge it over the config from the default branch. That is how the config change you are testing takes effect.

These variables take precedence over `spec.extraEnv` for this one run.

## Why it is always a dry run

With a single base branch configured, Renovate does not include the base branch in the names of its branches. A real run against the custom branch would therefore reuse the branches and pull requests Renovate keeps for the default branch: it would push to them, rebase them onto the custom branch, close the ones the tested config no longer wants, and rewrite the repository's Dependency Dashboard, until the next normal run moved everything back. A dry run reports the same decisions (`DRY-RUN: Would update PR …`, `Would autoclose PR`, …) without touching the repository.

The API rejects a custom branch run without `dryRun`, and the operator sets `RENOVATE_DRY_RUN=full` for every run with a custom branch, so this holds even for a RenovateProject status written directly.

## Where to read the result

The run's full Renovate log is in the project's **Logs**, like any other run; tick **Debug log level** to see the merged config there. Look for `DRY-RUN: Would …` lines, such as `Would create PR`, `Would update PR` or `Would autoclose PR`.

A custom branch run does not change what the dashboard and the metrics say about the project. Its result, pull request activity and log issues, and the per-project metrics (open pull requests, approvals, failed and issue flags, PR counters), keep describing the last normal run, because the dry run's numbers are what Renovate would do on another branch, not what happened. Only the project's state (back to completed or failed) and the run duration come from the dry run. The Job is marked with the `renovate-operator.mogenius.com/dry-run` annotation.

## Branch names

The branch name must start with a letter or digit. It may contain only letters, digits, `.`, `_`, `-` and `/`, and may be at most 250 characters long. It must not contain `..`, `//` or `/.`, and must not end in `/`, `.` or `.lock`. The name is always taken literally. Renovate's regex (`/…/`) and negation (`!…`) patterns are not accepted.

## Limitations

- **Repository config wins over the operator's settings.** If the repository's own config on its default branch sets `baseBranchPatterns` (or the older `baseBranches`), Renovate's repository config takes precedence over the operator-provided value. The custom branch is then ignored for that repository.
- **A webhook or schedule does not cancel a queued custom run.** If a push webhook or the cron schedule queues the same project while a custom branch run is waiting, the custom branch run still happens. The repository's normal run follows on the next trigger.
