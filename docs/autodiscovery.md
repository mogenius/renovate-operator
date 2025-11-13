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
  discoveryFilter: "Group1/*"
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
