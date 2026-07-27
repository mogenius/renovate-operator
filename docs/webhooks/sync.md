# Automatic Webhook Sync

Automatic webhook sync removes the need to add the operator's webhook to every repository by hand.
After each autodiscovery cycle, the operator ensures a webhook exists on every repository that was discovered for the RenovateJob, so checkbox interactions (Dependency Dashboard, PR rebase checkboxes) trigger Renovate runs without any per-repo setup.

When a repo drops out of the project list (or sync is disabled), the operator removes its webhook there, and when the RenovateJob is deleted, a finalizer removes the operator's webhooks from all of the job's repositories before the resource goes away.

Sync runs as part of the regular discovery schedule — there is no separate schedule to configure.

## Supported providers

Below is the list of providers that support webhook sync. By default the platform API token is read from the job's provider secret (`spec.secretRef`) — the same token Renovate itself uses. Optionally, a dedicated token for webhook management can be configured via `sync.secretRef` (see [Permissions](#permissions)).

| Provider  | Supported | Webhook endpoint      |
| :-------- | :-------: | :-------------------- |
| Forgejo   |    yes    | `/webhook/v1/forgejo` |
| Gitea     |    yes    | `/webhook/v1/gitea`   |
| GitHub    |    yes    | `/webhook/v1/github`  |
| GitLab    |    yes    | `/webhook/v1/gitlab`  |
| Bitbucket |    yes    | `/webhook/v1/bitbucket` |

## Configuration

Add the `sync` section to the `webhook` block in your RenovateJob:

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJob
metadata:
  name: my-renovate-job
  namespace: renovate-operator
spec:
  # ... other configuration ...
  webhook:
    enabled: true
    authentication:
      enabled: true
      secretRef:
        name: renovate-webhook-token
        key: token
    sync:
      enabled: true
      # optional: use a dedicated token for webhook management instead of the
      # job's Renovate token
      secretRef:
        name: webhook-admin-token
        key: token
```

### Sync fields

| Field       | Required | Description                               |
| :---------- | :------: | :----------------------------------------- |
| `enabled`   |   yes    | Enable or disable automatic webhook sync. |
| `secretRef` |    no    | Reference (`name`, `key`) to a secret in the job's namespace holding a platform token used only for webhook management. When omitted, the job's platform token (`spec.secretRef` / `spec.githubAppReference`) is used. When `key` is omitted, the common Renovate token key names (`RENOVATE_TOKEN`, `GITHUB_COM_TOKEN`, …) are tried. |

### Delivery authentication

If `webhook.authentication` is enabled, the first token from its `secretRef` is attached to each created webhook in the platform's native way, so deliveries authenticate back to the operator automatically:

| Provider        | Mechanism                                        |
| :-------------- | :----------------------------------------------- |
| Forgejo / Gitea | Authorization header (`Bearer <token>`)          |
| GitHub          | Hook secret → `X-Hub-Signature-256` HMAC header  |
| GitLab          | Hook token → `X-Gitlab-Token` header             |
| Bitbucket       | Hook secret → `X-Hub-Signature` HMAC header      |

## Webhook URL

The operator builds each hook's delivery URL from its external base URL (`WEBHOOK_BASE_URL` environment variable) plus the platform-specific path (see the provider table above) based on `spec.provider.name`, and appends `?namespace=...&job=...` query parameters to route deliveries to the right RenovateJob.

The Helm chart derives `WEBHOOK_BASE_URL` at deploy time from the webhook exposure it creates itself:

1. **HTTPRoute** — `webhook.route.hostnames[0]`, with `https` (TLS terminates at the Gateway).
2. **Ingress** — `webhook.ingress.host`, with `https` when `webhook.ingress.tls` is set, `http` otherwise.

Set `webhook.baseUrlScheme` to `http` or `https` to override the detected scheme, e.g. when TLS terminates at an external load balancer the chart cannot see.

## How sync works

Webhook sync runs automatically at the end of each autodiscovery cycle (controlled by `spec.schedule`).

1. The operator takes the projects discovered for the RenovateJob as the desired repo list.
2. For each repo, it checks for an existing webhook matching the delivery URL. If missing, it creates one; if the existing hook's events or active state drifted from the desired configuration (e.g. the hook subscribes to more events than the operator needs), it is updated in place. The auth token is write-only on every platform and cannot be drift-checked — it is only (re)applied when a hook is created or updated for another reason.
3. Repos that dropped out of the project list since the previous discovery are cleaned up — the operator deletes its webhook (again identified by the delivery URL) on each of them. Disabling sync removes the operator's webhook from all of the job's repos on the next discovery cycle.
4. Deleting the RenovateJob triggers the `renovate-operator.mogenius.com/webhook-cleanup` finalizer, which removes the operator's webhooks from all of the job's repos. Cleanup is best effort and never blocks deletion (e.g. when the platform secret is already gone).
5. Ensure failures (e.g. missing permission to manage webhooks) are logged and retried on the next cycle; they never block discovery. A **removal** that fails is not retried — the orphaned hook is logged and must be removed manually (harmless otherwise: its deliveries are rejected by the operator).

## Permissions

By default, webhook sync reuses the platform token Renovate already has.
The token's scope is usually sufficient (GitHub's `repo` scope includes repository hooks, GitLab's `api` scope covers project hooks, Bitbucket needs the `webhook` scope), but the account behind it must also hold a role that allows webhook management on every repo — **admin permission** on Forgejo/Gitea/GitHub/Bitbucket, **Maintainer** role on GitLab.
Note that this is more than Renovate itself needs (push/Developer access), so a bot account deliberately kept at minimal permissions may be able to run Renovate but not sync webhooks.
Repos where the token lacks that permission fail with a log message and are skipped; Renovate runs are unaffected.

To keep the Renovate bot account at minimal permissions, configure a dedicated webhook-management token via `sync.secretRef` — webhook sync then uses that token for all platform calls while Renovate keeps using the job's own token. The dedicated token is also used to clean up leftover managed hooks after sync is disabled, so keep the secret around until cleanup has run.

**GitHub App users** (`spec.githubAppReference`): installation tokens only carry the permissions granted to the app, and a Renovate app set up per the Renovate docs typically does not include webhook access. Add the **"Repository webhooks: Read and write"** permission to your GitHub App (and accept the updated permissions on the installation) for webhook sync to work.
