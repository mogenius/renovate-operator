# Running a repository against a custom branch

A custom branch run is a one-off Renovate run of a single repository against a branch you name, using the Renovate config on that branch. It is meant for checking a config change against a real repository before you merge it into the base branch, without touching the config that every other run uses.

## Starting a run

In the dashboard, each project's action button has a branch icon next to the debug (bug) icon. Click it, enter the branch name and choose **Run**. The dialog has two optional checkboxes:

| Option              | Effect                                                                             |
| ------------------- | ---------------------------------------------------------------------------------- |
| **Dry run**         | Renovate only logs what it would change and opens no pull requests (`dryRun=full`) |
| **Debug log level** | Same as the debug trigger (`RENOVATE_LOG_LEVEL=debug`)                             |

The run is queued with the same priority as any other dashboard trigger. While it is queued, the branch icon is highlighted and its tooltip names the branch. Once the Job starts, the options are cleared. Later runs, whether scheduled, triggered by a webhook or started from the dashboard, use the repository's normal configuration again.

Starting a custom branch run needs the `admin` role on the RenovateJob, just like the regular trigger button. See [Authentication](../configuration/auth.md). The **Trigger all** action does not support custom branches.

## What the run does

The operator adds these variables to the Renovate Job:

| Variable                                                     | Value                      |
| ------------------------------------------------------------ | -------------------------- |
| `RENOVATE_BASE_BRANCH_PATTERNS` and `RENOVATE_BASE_BRANCHES` | `["<branch>"]`             |
| `RENOVATE_USE_BASE_BRANCH_CONFIG`                            | `merge`                    |
| `RENOVATE_DRY_RUN`                                           | `full` (only with dry run) |

Two variables carry the branch because Renovate renamed `baseBranches` to `baseBranchPatterns`. Renovate ignores variables it does not recognise, so setting both works with images from before and after the rename.

`useBaseBranchConfig: merge` makes Renovate read the config file on the custom branch and merge it over the config from the default branch. That is how the config change you are testing takes effect.

These variables take precedence over `spec.extraEnv` for this one run.

> [!WARNING]
> Without **Dry run**, this is a real Renovate run: it opens and updates pull requests that target the custom branch, using the bot's normal credentials. Use a dry run first if you only want to read the result in the logs.

## Branch names

The branch name must start with a letter or digit. It may contain only letters, digits, `.`, `_`, `-` and `/`, and may be at most 250 characters long. It must not contain `..`, `//` or `/.`, and must not end in `/`, `.` or `.lock`. The name is always taken literally. Renovate's regex (`/…/`) and negation (`!…`) patterns are not accepted.

## Limitations

- **Repository config wins over the operator's settings.** If the repository's own config on its default branch sets `baseBranchPatterns` (or the older `baseBranches`), Renovate's repository config takes precedence over the operator-provided value. The custom branch is then ignored for that repository.
- **A webhook or schedule does not cancel a queued custom run.** If a push webhook or the cron schedule queues the same project while a custom branch run is waiting, the custom branch run still happens. The repository's normal run follows on the next trigger.
- **Results are kept as the project's latest run.** The dashboard shows a project's latest result, so the status, PR activity and log issues from a custom branch run replace those of the previous normal run until the next one.
