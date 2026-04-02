# Autodiscovery

Autodiscovery is a feature that automatically finds and catalogs projects across your Git platform
that Renovate should manage. Instead of manually specifying each project, you can configure filters
or topics to let renovate discover them for you.

## How it works

When you configure a `RenovateJob` with autodiscovery settings, the operator will:

1. Trigger a discovery process on schedule.
2. Scan your Git platform using the specified filter or topic criteria.
3. Return a list of discovered projects.
4. Display them in the UI, ready to be scheduled for automated dependency updates.

## Configuration in the RenovateJob CRD

### Using Discovery Filter

The `discoveryFilter` field uses Renovate's autodiscover filter syntax to select projects.

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-group1
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  discoveryFilters: 
    - "Group1/*"
  ...
```

Common filter patterns:

- `Group1/*` — discover all projects under "Group1" group.
- `org/**` — discover all projects recursively under "org" (supports subgroups).
- `Group1/project-*` — discover projects matching a specific name pattern.
- Leave empty or omit to discover all projects (depending on your Git platform settings).

Refer to [Renovate's documentation](https://docs.renovatebot.com/self-hosted-configuration/#autodiscoverfilter) for detailed syntax.

### Using Discovery Topics

The `discoverTopics` field filters projects by topic tags on your Git platform. This is especially
useful on Github where you can not create subgroups in your organization.

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-group1
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  discoverTopics: "renovate"
  ...
```

In this example, projects tagged with `renovate` will be discovered.

Refer to [Renovate's documentation](https://docs.renovatebot.com/self-hosted-configuration/#autodiscovertopics) for detailed syntax.

### Excluding Forked Repositories

When using autodiscovery, forked repositories are included by default. This can lead to unnecessary
jobs being created for repositories you don't intend to manage. The `skipForks` field tells the
operator to query the platform API after discovery and exclude any forked repositories before
creating execution jobs.

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: renovate-group1
  namespace: renovate-operator
spec:
  schedule: "0 * * * *"
  skipForks: true
  secretRef: renovate-secret
  provider:
    name: github
  ...
```

Requirements:
- `secretRef` must be set and the referenced secret must contain a platform API token
  (one of: `RENOVATE_TOKEN`, `GITHUB_COM_TOKEN`, `GITLAB_TOKEN`, `BITBUCKET_TOKEN`,
  `GITEA_TOKEN`, or `FORGEJO_TOKEN`).
- `provider` must be configured with a supported platform.

Supported platforms:

| Platform    | API used to detect forks                                  |
|-------------|-----------------------------------------------------------|
| `github`    | `GET /repos/{owner}/{repo}` — checks `fork` field        |
| `gitlab`    | `GET /projects/{path}` — checks `forked_from_project`    |
| `gitea`     | `GET /api/v1/repos/{owner}/{repo}` — checks `fork` field |
| `forgejo`   | Same as Gitea                                             |
| `bitbucket` | `GET /2.0/repositories/{workspace}/{slug}` — checks `parent` field |

If the API call fails for a specific repository, the repository is kept (fail-open) to avoid
accidentally excluding valid projects.
