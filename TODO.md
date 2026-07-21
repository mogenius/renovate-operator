# GitHub Enterprise App — Auto-discovery of Installation IDs

Implement step by step. Each section below is one self-contained change.

## Background

Currently `GithubAppReference` requires an explicit `InstallationIdSecretKey`. For GitHub Enterprise Apps
(or any GitHub App installed across multiple orgs), the operator should call `GET /app/installations`
with a JWT, discover all installation IDs automatically, and iterate — running a full Renovate pipeline
(discovery → execution) per installation. The private key never leaves the operator; Renovate pods only
see short-lived installation access tokens (`RENOVATE_TOKEN`), same as today.

A new explicit struct `GithubEnterpriseAppReference` is added. Its presence is the unambiguous signal
that enterprise mode is active. `GithubAppReference` and all existing behavior are completely unchanged.

---

## Step 1 — CRD types (`src/api/v1alpha1/renovatejob_types.go`)

- Add new struct next to `GithubAppReference`:
  ```go
  type GithubEnterpriseAppReference struct {
      SecretName     string `json:"secretName"`
      AppIdSecretKey string `json:"appIdSecretKey"`
      PemSecretKey   string `json:"pemSecretKey"`
  }
  ```
- Add optional field to `RenovateJobSpec`:
  ```go
  GithubEnterpriseAppReference *GithubEnterpriseAppReference `json:"githubEnterpriseAppReference,omitempty"`
  ```
- Add optional field to `ProjectStatus`:
  ```go
  TokenSecretName string `json:"tokenSecretName,omitempty"`
  ```
  The executor uses this secret instead of the job-level default when non-empty.
  `DeepCopyInto` needs no change (plain string).

Verify: `just generate` regenerates the CRD schema.

---

## Step 2 — Secret naming (`src/github/githubAppSecretNames.go`)

Add:
```go
func GetNameForGithubAppInstallationSecret(job *api.RenovateJob, installationID string) string
```
Pattern: `{jobName}-github-app-{installID}-{sha256[:4]}`. Keeps naming consistent with `GetNameForGithubAppSecret`.

Verify: `just build`

---

## Step 3 — `GithubAppToken` interface + implementation (`src/github/githubAppToken.go`)

- **Extract a shared `mintJWT(appID string, key *rsa.PrivateKey) (string, error)` helper** — the JWT-minting
  logic is currently inlined inside `createGithubAppTokenDetailed`. Extract it so `listInstallationIDs`
  can reuse it without duplication.
- **Add to `GithubAppToken` interface**:
  ```go
  EnsureTokensForEnterpriseApp(ctx context.Context, job *api.RenovateJob) ([]string, error)
  ```
- **New private method** `listInstallationIDs(appID, pemStr, githubApi string) ([]string, error)`:
  - Parses PEM, calls `mintJWT`.
  - `GET {githubApi}/app/installations` with `Authorization: Bearer <jwt>` + `Accept: application/vnd.github+json`.
  - Parses `[{"id": 12345}, ...]`, returns IDs as `[]string`.
- **`EnsureTokensForEnterpriseApp` implementation**:
  1. Guard: error if `job.Spec.GithubEnterpriseAppReference == nil`.
  2. Read `AppIdSecretKey` + `PemSecretKey` from the referenced Kubernetes Secret.
  3. Resolve `githubApi` via `utils.GetPlatformAndEndpoint(job.Spec.Provider)`.
  4. Call `listInstallationIDs`.
  5. For each installation ID: check for a fresh token (same 30-minute threshold as `EnsureToken`).
     Mint via `createGithubAppTokenDetailed` if stale. Store `RENOVATE_TOKEN` + expiry annotation in
     `GetNameForGithubAppInstallationSecret(job, id)`, owned by the `RenovateJob`.
  6. Return all secret names.
- **`EnsureToken`**: no change. It guards on `GithubAppReference == nil` as before.

Verify: `just test-unit` (add unit tests for `listInstallationIDs` and `EnsureTokensForEnterpriseApp`)

---

## Step 4 — Annotation constant (`src/internal/crdManager/`)

Add next to existing job annotation constants:
```go
JOB_ANNOTATION_TOKEN_SECRET_NAME = "renovate-operator.mogenius.com/token-secret-name"
```

Verify: `just build`

---

## Step 5 — Job definitions (`src/internal/renovate/jobDefinitions.go`)

Change signatures:
```go
func newDiscoveryJob(job *api.RenovateJob, tokenSecretName, traceparent string) *batchv1.Job
func newRenovateJob(job *api.RenovateJob, project, tokenSecretName, traceparent string) *batchv1.Job
```
When `tokenSecretName != ""`, use it in the `envFrom` block instead of `github.GetNameForGithubAppSecret(job)`.
When empty: existing behavior unchanged.

Verify: `just build` (fixes all call sites that now need the extra param)

---

## Step 6 — Discovery agent (`src/internal/renovate/discoveryAgent.go`)

- **`DiscoveryJobOptions`**: add `TokenSecretName string` (empty = derive from job, existing behavior).
- **`CreateDiscoveryJob`**:
  - Pass `options.TokenSecretName` to `newDiscoveryJob`.
  - When non-empty, stamp annotation `JOB_ANNOTATION_TOKEN_SECRET_NAME` on the created k8s Job.
- **`ProcessDiscoveryJobResult`**:
  - Read `JOB_ANNOTATION_TOKEN_SECRET_NAME` annotation from the completed k8s Job.
  - After `parseAndSortDiscoveredProjects`, ensure newly reconciled projects have `TokenSecretName` set.
    Investigate `ReconcileProjects` signature — extend it or apply a post-reconcile patch.

Verify: `just test-unit`

---

## Step 7 — Executor (`src/internal/renovate/executor.go`)

**`dispatchScheduled`**: pass `project.TokenSecretName` as the `tokenSecretName` argument to `newRenovateJob`.
Empty string falls through to existing secret derivation — no behavior change for non-enterprise jobs.

Verify: `just test-unit`

---

## Step 8 — Reconciler (`src/controllers/renovatejob_controller.go`)

**`Reconcile`** — after the existing `EnsureToken` call:
```go
if renovateJob.Spec.GithubEnterpriseAppReference != nil {
    if _, err := r.GithubApp.EnsureTokensForEnterpriseApp(ctx, renovateJob); err != nil {
        logger.Error(err, "failed to ensure enterprise github app tokens")
    }
}
```

**`createScheduler` closure** — extend the scheduled function:
```
if GithubEnterpriseAppReference != nil:
    secretNames, err = GithubApp.EnsureTokensForEnterpriseApp(ctx, currentJob)
    for each secretName:
        Discovery.CreateDiscoveryJob(ctx, *currentJob, DiscoveryJobOptions{
            TriggerAllProjects: true,
            TokenSecretName:    secretName,
        })
else:
    Discovery.CreateDiscoveryJob(ctx, *currentJob, DiscoveryJobOptions{TriggerAllProjects: true})
```

Verify: `just build`, `just test-unit`

---

## Known Limitations (follow-up tasks)

- `skipForks`, `skipPendingDeletion`, and `webhook.sync` are **not** supported when using
  `githubEnterpriseAppReference`. The factory does not yet route per-project API calls through
  `project.TokenSecretName`. Document in CRD / Helm chart comments.
- Per-installation webhook sync: out of scope for now.

---

## Final Verification

```
just build
just generate
just test-unit
```
