# GitHub App — Every Installation

A `GitHubAppInstallationSet` runs Renovate for every account a GitHub App is
installed on. The operator lists the App's installations from the GitHub API and
generates one `RenovateJob` per installation, so anyone who installs the App on
their user account or organization gets Renovate without a new Secret or
`RenovateJob`.

Use this instead of the [native integration](./github-app-native.md) when the
set of installations changes over time, or when you don't control it.

If you haven't created your GitHub App yet: [GitHub App Setup](./github-app-setup.md)

---

## 1. Create the credentials Secret

Unlike the native integration, the Secret carries no installation id:

```yaml
kind: Secret
apiVersion: v1
type: Opaque
metadata:
  name: github-app-credentials
  namespace: renovate-operator
  labels:
    renovate-operator.mogenius.com/allow-ref: "true"
data:
  APP_ID: <base64-encoded App ID>
  PEM: <base64-encoded private key>
```

## 2. Create the template

Every generated `RenovateJob` inherits its configuration from a
[shared template](../configuration/shared-templates.md):

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: RenovateJobTemplate
metadata:
  name: installation-defaults
  namespace: renovate-operator
spec:
  image: renovate/renovate:43.104.11
  schedule: "H * * * *"
  parallelism: 2
```

> **Schedule:** every generated `RenovateJob` runs on the **same** schedule,
> the template's. With a fixed expression like `0 * * * *`, every installation
> starts Renovate at the same moment. Use a hashed schedule in the template
> instead: an `H` field resolves to a stable value derived from each
> `RenovateJob`'s name and namespace, so the runs spread out.
>
> | Expression        | Runs                                               |
> | ----------------- | -------------------------------------------------- |
> | `H * * * *`       | once an hour, at a per-job minute                  |
> | `H/15 * * * *`    | every 15 minutes, at a per-job offset              |
> | `H H * * *`       | once a day, at a per-job hour and minute           |
> | `H(0-29) 6 * * *` | once a day between 06:00 and 06:29, per-job minute |

Each generated job's token is scoped to its own installation, so autodiscovery
finds exactly the repositories that installation grants. No `discoveryFilters`
are needed.

## 3. Create the GitHubAppInstallationSet

```yaml
apiVersion: renovate-operator.mogenius.com/v1alpha1
kind: GitHubAppInstallationSet
metadata:
  name: club
  namespace: renovate-operator
spec:
  app:
    secretName: github-app-credentials
    appIdSecretKey: APP_ID
    pemSecretKey: PEM
  templateRef:
    name: installation-defaults
  # optional, see below
  installations:
    exclude:
      - some-account
  resyncInterval: 10m
```

```console
$ kubectl get gais -n renovate-operator
NAME   READY   LAST SYNC
club   True    2m
```

`status.installations` lists every matching installation and the `RenovateJob`
generated for it.

## Fields

| Field                        | Description                                                                                       |
| ---------------------------- | ------------------------------------------------------------------------------------------------- |
| `app`                        | Secret and keys holding the App id and private key.                                               |
| `templateRef`                | Template the generated `RenovateJob`s inherit from. `kind` may be `ClusterRenovateJobTemplate`.   |
| `provider.endpoint`          | GitHub API base URL. Defaults to `https://api.github.com`; set it for GitHub Enterprise Server.   |
| `provider.publicEndpoint`    | URL used for UI links, if it differs from `endpoint`.                                             |
| `installations.include`      | Account logins to include. All accounts match when empty.                                         |
| `installations.exclude`      | Account logins to exclude. Takes precedence over `include`.                                       |
| `installations.accountTypes` | `User` and/or `Organization`. All types match when empty.                                         |
| `resyncInterval`             | How often installations are listed. Defaults to `10m`, minimum `1m`.                              |
| `webhook`                    | Resync immediately on installation events, see below.                                             |

Logins match case-insensitively.

Installations on an enterprise account are skipped. They grant access to the
enterprise itself, not to its organizations' repositories; install the App on
the organizations instead.

> **Public Apps:** with no `installations` filter, anyone who can install the
> App gets a `RenovateJob` and the cluster resources it consumes. For a public
> App, restrict `installations.include`, or make the App private.

## Generated resources

For each installation the operator creates:

- a `RenovateJob` named `<set>-<account login>`, or `<set>-<installation id>` if
  that name is already taken by a `RenovateJob` the set doesn't own. Names are
  truncated and hashed to fit 63 characters.
- a Secret holding the App id, private key and installation id, which the
  `RenovateJob`'s `githubAppReference` points at. The token itself is minted as
  for the [native integration](./github-app-native.md#how-it-works).

Both carry the labels `renovate-operator.mogenius.com/installation-set` and
`renovate-operator.mogenius.com/installation-id`.

The set owns the `RenovateJob`s, which in turn own their Secrets. Deleting the
set deletes everything it generated. Uninstalling the App from an account, or
excluding the account through the filter, deletes that installation's
`RenovateJob` on the next sync.

The set manages `templateRef`, `provider` and `githubAppReference` on each
generated `RenovateJob` and resets changes to them. You can set any other field
directly on a generated `RenovateJob` to override its template for that
installation. That override is lost if the `RenovateJob` is deleted and
regenerated.

A suspended installation can't mint tokens, so its `RenovateJob` is
[suspended](../operations/suspend.md) until the installation is unsuspended.

If listing installations fails, the set reports `Ready=False` with reason
`ListInstallationsFailed` and keeps the existing `RenovateJob`s.

## Immediate resync via webhook

By default new installations are picked up within `resyncInterval`. To pick
them up immediately, point the App's webhook at the operator:

1. In the App's settings, activate the webhook, set the **Webhook URL** to
   `https://your-webhook-host/webhook/v1/github` and set a **Webhook secret**.
   GitHub delivers `installation` and `installation_repositories` events to an
   App's webhook without a subscription.
2. Store the webhook secret and enable the webhook on the set:

```yaml
spec:
  webhook:
    enabled: true
    secretRef:
      name: github-app-webhook
      key: secret
```

The Secret needs the `renovate-operator.mogenius.com/allow-ref` label once the
policy engine is enabled.

The operator verifies the `X-Hub-Signature-256` header against the secret of
every set with the webhook enabled, and resyncs each set it verifies against.
An `installation_repositories` event also starts discovery on that
installation's `RenovateJob`, so added or removed repositories show up without
waiting for the next schedule. Append `?namespace=<namespace>` to the URL to
only consider sets in that namespace.

The same URL also handles the pull request and issue events described in
[GitHub webhooks](../webhooks/github.md). To use both, enable `webhook` and
`webhook.authentication` in the template with the same secret.

## Policy

Once the [policy engine](../security/security.md) is enabled:

- the credentials and webhook Secrets need the
  `renovate-operator.mogenius.com/allow-ref` label.
- the host of `provider.endpoint` (`api.github.com` by default) and of
  `provider.publicEndpoint` must be listed in `policy.allowedHosts`. The App's
  private key is never sent anywhere else.

The generated credentials Secrets are labelled `allow-ref` by the operator,
since their source Secret already passed this check. Each generated
`RenovateJob` then goes through the same acceptance check as any other.
