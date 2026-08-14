# Documentation

- [Getting Started](./getting-started.md) — install the operator and run your first RenovateJob

## Platforms

Platform-specific credential and provider setup.

| Guide                                                     |                                                                    |
| --------------------------------------------------------- | ------------------------------------------------------------------ |
| [GitHub — PAT](./platforms/github-pat.md)                 | Personal access token setup                                        |
| [GitHub — App (Native)](./platforms/github-app-native.md) | GitHub App with built-in token management                          |
| [GitHub — App (ESO)](./platforms/github-app-eso.md)       | GitHub App with External Secrets Operator                          |
| [GitHub — App Setup](./platforms/github-app-setup.md)     | Creating a GitHub App                                              |
| [GitLab](./platforms/gitlab.md)                           | PAT-based setup                                                    |
| [Other platforms](./platforms/generic.md)                 | Azure DevOps, Bitbucket, Gitea, Forgejo, and others via `extraEnv` |

## Configuration

RenovateJob and Operator Settings to customize your experience.

| Guide                                                       |                                                             |
| ----------------------------------------------------------- | ----------------------------------------------------------- |
| [Autodiscovery](./configuration/autodiscovery.md)             | Filters, topics, fork and pending-deletion exclusion        |
| [Authentication](./configuration/auth.md)                     | OIDC, GitHub OAuth, access control                          |
| [Renovate Configuration](./configuration/renovate-config.md)  | Inline or ConfigMap-based Renovate config file              |
| [Scheduling](./configuration/scheduling.md)                   | Node selectors, affinity, tolerations, priority classes     |
| [Extra Volumes](./configuration/extra-volumes.md)             | Mounting ConfigMaps, Secrets, and ephemeral scratch volumes |
| [Image Pull Secrets](./configuration/image-pull-secrets.md)   | Private registry authentication                             |
| [Base Path](./configuration/base-path.md)                     | Serving the UI under a sub-path                             |

## Self-Service

Letting repository owners and development teams use renovate as a service.

| Guide                                                        |                                                              |
| ------------------------------------------------------------ | ------------------------------------------------------------ |
| [Self-Service Onboarding](./self-service/overview.md)        | Topic-based onboarding and multi-tenant RenovateJob creation |
| [Annotation Triggers](./self-service/annotation-triggers.md) | On-demand discovery and scheduling via `kubectl annotate`    |

## Webhooks

Triggering Renovate runs on repository events.

| Guide                                        |                                                     |
| -------------------------------------------- | --------------------------------------------------- |
| [Generic Webhook API](./webhooks/webhook.md) | Platform-agnostic trigger endpoint                  |
| [Automatic Webhook Sync](./webhooks/sync.md) | Operator-managed webhooks after every discovery run |
| [GitHub](./webhooks/github.md)               |                                                     |
| [GitLab](./webhooks/gitlab.md)               |                                                     |
| [Gitea](./webhooks/gitea.md)                 |                                                     |
| [Forgejo](./webhooks/forgejo.md)             |                                                     |
| [Bitbucket](./webhooks/bitbucket.md)         |                                                     |

## Operations

Day-2 concerns: observability, storage, and cost allocation.

| Guide                                                      |                                            |
| ---------------------------------------------------------- | ------------------------------------------ |
| [Metrics](./operations/metrics.md)                         | Prometheus metrics and alerting rules      |
| [PR Activity](./operations/pr-activity.md)                 | Tracking open PRs and dependency issues    |
| [Valkey / Redis](./operations/valkey.md)                   | Session storage, log storage, and caching  |
| [S3 Object Storage](./operations/s3.md)                    | Log archival and Renovate cache forwarding |
| [Pod Label Templates](./operations/pod-label-templates.md) | Templated labels for cost allocation       |

## Security

| Guide                              |                                            |
| ---------------------------------- | ------------------------------------------ |
| [Security](./security/security.md) | Trust model, attack vectors, policy engine |

## Migration

| Guide                                        |                                          |
| -------------------------------------------- | ---------------------------------------- |
| [v5 → v6](./migration/migration-v5-to-v6.md) | Policy engine and access control changes |
