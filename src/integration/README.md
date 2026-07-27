# Webhook signing-token integration test

An over-the-wire integration test for incoming-webhook authentication, covering the
[Standard Webhooks](https://www.standardwebhooks.com/) signature scheme (what GitLab calls webhook
"signing tokens") alongside the legacy `X-Gitlab-Token` secret token.

## What it exercises

Unlike the in-package webhook tests (`src/webhook/*_integration_test.go`), which use
`httptest.NewRecorder` and a **mock** manager, this boots the **real** `webhook.Server` on a real
TCP socket, backed by the **real** `crdManager.RenovateJobManager` over a controller-runtime fake
client seeded with a real `Secret`. Requests are sent with a real `http.Client` by a signer that
**independently** reimplements the sender side of Standard Webhooks — so the operator's signature
verification is checked as a black box.

Path under test: `HTTP → router → gitLabWebhook (event filter) → resolver → manager → Secret read →
HMAC-SHA256 over "{id}.{timestamp}.{body}" → CRD status update`.

No Kubernetes cluster is needed: the fake client provides the API surface, so the test is
deterministic and runs in ~0.1s.

## Cases

| Case | Expected |
| --- | --- |
| Valid signing-token signature | `202` + project status → `scheduled` |
| Body tampered after signing | `401` |
| Signature from a different key | `401` |
| Stale timestamp (now − 10m) | `401` (replay window) |
| Future timestamp (now + 10m) | `401` (replay window) |
| No credentials on an auth-required job | `401` |
| Legacy `X-Gitlab-Token` | `202` |
| Auth-disabled job, unsigned | `202` + project status → `scheduled` |

## Run

```sh
just test-integration
# or
cd src && go test -tags integration -count=1 -v ./integration/...
```

The test is behind a `//go:build integration` tag, so it is excluded from `just test-unit` and only
runs when `-tags integration` is passed.

## Signing key

The seeded secret holds the public Standard Webhooks example key
(a `whsec_…` value; the prefix is stripped and the remainder base64-decoded to the HMAC key) plus a
comma-separated plain token for the legacy path. To point the operator at a real
GitLab webhook instead, store the `whsec_…` value GitLab issues in the `Secret` referenced by
`webhook.authentication.secretRef`.
