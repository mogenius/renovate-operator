# Renovate Operator — Claude Code Guide

## Project Overview

A Kubernetes-native operator that runs [Renovate](https://github.com/renovatebot/renovate) for automated dependency updates. It manages CRDs, cron scheduling, parallel job execution, and a web UI — all while abstracting over multiple Git platforms.

## Directory Structure

```
src/
├── api/v1alpha1/          # Kubernetes CRD API types (RenovateJob, specs, status) + well-known label/annotation keys
├── assert/                # Fail-fast assertion utilities (panic on critical errors)
├── clientProvider/        # Kubernetes client factory
├── cmd/                   # main.go — wires all components together
├── config/                # Singleton env-var config with schema validation
├── controllers/           # Kubernetes reconciliation loop (controller-runtime)
├── gitProviderClients/    # Git provider abstraction (interface + implementations)
│   ├── factory/           # Factory: creates correct provider from job.Spec.Provider
│   ├── githubProvider/
│   ├── gitlabProvider/
│   ├── giteaProvider/
│   ├── forgejoProvider/
│   └── bitbucketProvider/
├── health/                # Thread-safe health state tracking
├── internal/
│   ├── crdManager/        # CRUD on RenovateJob CRDs and Kubernetes Jobs
│   ├── webhookSync/       # Provider-agnostic repo webhook sync (via GitProviderClient)
│   ├── policy/            # Install-wide guard rails on RenovateJob specs (allowed destinations)
│   ├── parser/            # Extracts discovered repos and dependency issues from Renovate logs
│   ├── renovate/          # Core engine: discovery, executor, job definitions
│   ├── types/             # Shared internal types
│   └── utils/             # Platform endpoints, job naming helpers
├── metricStore/           # Prometheus metrics
├── scheduler/             # Cron scheduling wrapper (go-cron)
├── static/                # Frontend assets (CSS, JS)
├── ui/                    # Web UI server, auth (OIDC, GitHub OAuth), API endpoints
└── webhook/               # HTTP webhook server (GitHub, GitLab, Forgejo triggers)
charts/                    # Helm chart
docs/                      # Documentation
```

## Coding Conventions

### 1. Everything is Generic — Program to Interfaces

This is the most important rule. **Any component that touches a Git provider, scheduler, executor, or external system must be expressed as an interface.** New code must never be coupled to a concrete implementation.

- Define the interface first, then implement it per platform/provider.
- Add new Git providers by implementing `GitProviderClient` — never add provider-specific branches to shared code.
- Existing interfaces to extend (not replace):
  - `gitProviderClients.GitProviderClient` — repository info (fork & pending-deletion detection, fetched in one call), webhooks, repo search
  - `ui.AuthProvider` — authentication backends
  - `internal/renovate.DiscoveryAgent`, `RenovateExecutor`
  - `scheduler.Scheduler`

### 2. Git Provider Factory Pattern

Provider clients are created exclusively through the factory in `gitProviderClients/factory/`. The factory reads `job.Spec.Provider` and returns the appropriate `GitProviderClient` implementation. When adding a new provider:

1. Create a new package under `gitProviderClients/<providerName>Provider/`
2. Implement the full `GitProviderClient` interface
3. Register it in the factory

Never instantiate a provider client directly outside the factory.

### 3. Kubernetes Job–Based Execution

Renovate runs are launched as Kubernetes Jobs (not bare Pods). This ensures:

- TTL-based cleanup via `ttlSecondsAfterFinished`
- Restart policies and retry semantics
- Label-based selection (`renovate-operator.mogenius.com/job-type`, `job-name`, `generation`)

Job templates are built in `internal/renovate/jobDefinitions.go`. Extend templates there — never build job specs inline in other packages.

### 4. Configuration

All configuration is environment-variable driven via the singleton in `config/`. Rules:

- Declare new config values in the config schema (with `Optional`/`Required` and defaults)
- Access config values via `config.GetValue()` — never read `os.Getenv` directly elsewhere
- Keep the config schema the single source of truth for what the operator accepts

### 5. Error Handling

- Use `fmt.Errorf("context: %w", err)` for wrapping and propagating errors in normal paths
- Use `assert.NoError()` / `assert.Assert()` only for truly unrecoverable startup/initialization failures
- Fail open on external API errors where exclusion would be worse than inclusion (e.g., fork filtering keeps repos if the API call fails)

### 6. Concurrency

- Use `sync.Mutex` / `sync.RWMutex` to protect shared state
- Use channel-based semaphores to cap parallelism on external API calls (see `forkFilter.go` for the pattern — max 10 concurrent goroutines)
- The executor polls every 10 seconds and respects the `parallelism` field from the CRD

### 7. Logging

Use the `logr.Logger` interface throughout (injected, never obtained globally). Follow these conventions:

- `logger.Info(...)` for normal operational events
- `logger.V(1).Info(...)` for verbose/debug output
- `logger.Error(err, ...)` for errors with context
- Never use `fmt.Println` or `log.Print` in production code paths

### 8. Kubernetes Reconciler Pattern

Controllers use `controller-runtime` reconciliation. In reconcilers:

- Return `ctrl.Result{RequeueAfter: ...}` for scheduled requeues
- Return `ctrl.Result{}, err` to trigger immediate retry on error
- Keep reconcilers idempotent — rerunning the same reconcile must be safe

### 9. Health Checks

Health state is managed centrally in the `health/` package with thread-safe setters. Update health state there rather than managing it locally in components. Health is exposed via the operator's HTTP health endpoint.

### 10. Naming Conventions

- Kubernetes Job names are generated in `internal/utils/jobNames.go` — use helpers there for consistent naming
- Platform API base URLs are resolved in `internal/utils/platformEndpoints.go`
- Labels follow the pattern `renovate-operator.mogenius.com/<key>`

### 11. Well-Known Label, Annotation and Finalizer Keys

Every key the operator reads or writes on an object is declared in `api/v1alpha1` and nowhere else, following the upstream layout: `well_known_labels.go` (labels and the finalizer) and `annotation_key_constants.go`. They live with the API types rather than in an `internal/` package because they _are_ API surface — users apply the trigger annotations and `LabelAllowRef` by hand, and third parties select Jobs by the job labels — so a literal spelled inline is a silent contract break. Every consumer already imports the API package, so there is no extra coupling.

Upstream naming: labels are `Label<Thing>`, annotations are `<Thing>AnnotationKey`, finalizers are `Finalizer<Thing>`, label values are `LabelValue<Thing>`. `GroupName` (in `groupversion_info.go`) is the shared prefix and the API group `GroupVersion` is built from; the `+groupName` marker stays a literal because controller-gen reads it from the comment. The upstream recommended `app.kubernetes.io/*` keys are declared there too, flagged as not owned by this group.

Add new keys to `ownedKeys` in `well_known_labels_test.go`, which enforces the group prefix, `IsQualifiedName` validity, and uniqueness within labels and within annotations separately (a label and an annotation may share a key, as `LabelProject` and `ProjectAnnotationKey` do).

## Technology Stack

| Concern            | Library                                             |
| ------------------ | --------------------------------------------------- |
| Operator framework | `sigs.k8s.io/controller-runtime`                    |
| Scheduling         | `github.com/netresearch/go-cron`                    |
| HTTP routing       | `github.com/gorilla/mux`                            |
| Metrics            | `github.com/prometheus/client_golang`               |
| Logging            | `github.com/go-logr/logr` + `go.uber.org/zap`       |
| OIDC auth          | `github.com/coreos/go-oidc` + `golang.org/x/oauth2` |

## Key Architectural Decisions

- **Leader election** — only the leader runs the executor and scheduler to prevent duplicate job launches
- **Platform credentials** live in Kubernetes Secrets, referenced from the CRD — never hardcoded
- **Webhook servers** are platform-specific (`/webhook/v1/gitlab`, `/github`, `/forgejo`, `/gitea`, `/bitbucket`) but trigger the same internal scheduling interface
- **Webhook sync is provider-agnostic and stateless** — after each discovery run, `crdManager.SyncWebhooks` ensures the operator's webhook exists on every discovered project (config: `spec.webhook.sync`) and removes it from the repos `ReconcileProjects` reported as removed. Hooks are identified by the platform endpoint path plus the `namespace`/`job` parameters of their delivery URL — the host is configuration, not identity, so changing the base URL reconciles the existing hook in place instead of creating a duplicate. No state is stored. The `webhook-cleanup` finalizer on the RenovateJob removes all hooks on deletion (best effort, never blocks deletion). The sync logic lives in `internal/webhookSync.Sync`, all platform access goes through `GitProviderClient` (implemented for all five providers). Sync uses the job's Renovate token by default; an optional dedicated webhook-management token can be set via `spec.webhook.sync.secretRef` (factory method `NewClientWithTokenRef`). Sync failures are logged, never block discovery (fail open)
- **Discovery uses Renovate itself** — a discovery Job runs Renovate with `autodiscover: true` and its JSON logs are parsed to extract repositories
- **Log parsing has two layers** — `RENOVATE_REPORT_TYPE=logging` is injected into every Job, causing Renovate to emit a structured `"Printing report"` entry at the end of each run. `internal/parser` parses this as the primary source for PR activity (branch results, PR numbers, titles). Per-message parsing (`Creating PR`, `Updating PR`, `PR automerged`, `branches info extended`, etc.) runs in parallel as a fallback for older Renovate versions and provides more granular action types (created vs. updated) that the report does not distinguish. When both are present, per-message action type wins; the report backfills missing PR numbers and titles.
- **Executor uses two passes per tick** — Pass 1 (`countRunningProjects`) tallies how many projects are still in `Running` status across all RenovateJobs (purely in-memory, no API calls); Pass 2 (`dispatchScheduled`) collects all Scheduled projects into a flat sorted list and dispatches new k8s Jobs up to the global and per-job parallelism limits. Running project status transitions are handled reactively by the `job_controller` (`ProcessProjectJobResult`) when k8s Jobs complete, not by the executor tick.
- **Global parallelism limit** — `GLOBAL_PARALLELISM_LIMIT` env var (Helm: `config.globalParallelismLimit`, default `0` = unlimited) caps total concurrent executor jobs across all RenovateJobs. Per-job `Spec.Parallelism` is still enforced as an additional gate.
- **Anti-starvation via priority-then-oldest-wait sort** — in Pass 2, candidates are sorted first by `Priority` descending, then by the oldest `LastRun` time among Scheduled projects in their RenovateJob. Among equal-priority candidates, the job that has been waiting longest dispatches first, preventing starvation.
- **UI sub-path (`BASE_PATH`)** — the UI, API, auth and health routes can be served under a sub-path so the operator can be co-hosted with other apps on one hostname. `BASE_PATH` env (Helm: top-level `basePath`, default `""` = root) is normalized in `ui.BasePath()` (leading slash, no trailing slash). `server.go` mounts all routes on a `PathPrefix(basePath)` subrouter and redirects `/` → base path; `ui.go` strips the prefix for static files and injects `<base href>` + `window.__BASE_PATH__` into `index.html`/`logs.html`; the frontend builds all runtime URLs from `BASE`; auth redirects use `withBase()` and cookies are scoped via `cookiePath()`. The Helm `basePath` also drives the Ingress/HTTPRoute path and is appended to auto-detected OAuth/OIDC redirect URLs (`renovate-operator.basePath` helper). OAuth/OIDC redirect URLs registered with the identity provider must include the sub-path.
- **Policy engine master switch** — `policy.enabled` (env `POLICY_ENABLED`) turns every operator-side check into a no-op. It **defaults to off** so a fresh install works immediately — the onboarding and homelab case: get Renovate running first, secure the operator afterwards. Enforcement is opt-in and the operator logs a multi-line warning on every start while it is off. Note the asymmetry this creates: the `Policy` struct's zero value enforces (see below) while `FromConfig` returns a disabled policy, so tests are strict by default and production is not. The `Policy` field is spelled **`Disabled`**, not `Enabled`, so the zero value enforces — tests construct `Policy{}` everywhere and a permissive zero value would silently void enforcement. Short-circuits live in the three leaf checks (`ValidateDestination`, `ValidateReferencedSecret`, `ValidateJobSpec`); anything new that enforces must add one. When off, the reconciler records `Accepted=True` with reason `policy.ReasonPolicyDisabled` via `Policy.AcceptedReason()`, so the unsecured state is visible on every RenovateJob, and `main.go` logs a multi-line startup warning. It deliberately does **not** relax the CRD's CEL invariants (hostPath, privileged, allowPrivilegeEscalation) — those are API-server enforced — nor the config validation, so a malformed value still fails at startup.
- **Destination policy (`internal/policy`)** — the operator acts on a RenovateJob with its own cluster credentials, so every URL taken from a spec (`spec.provider.endpoint`, `spec.provider.publicEndpoint`, `spec.webhook.baseUrl`, plus the `WEBHOOK_BASE_URL` fallback) is checked against `policy.allowedHosts` (env `POLICY_ALLOWED_HOSTS`) before use. Matching is on `url.Hostname()` and is **exact** — no subtree or wildcard form, since that would trust every name anyone can stand up under an internal domain; ports and paths are ignored. Malformed entries (leading dot, wildcard, scheme, port, path) are rejected by `policy.ValidateAllowedHosts` at startup and by the chart's values schema at install time, so a mistyped entry never degrades into silently matching nothing. An empty list denies everything — the chart ships the public platform hosts as its default, so a raw-manifest deployment fails closed while a stock chart install does not. The chart also derives one entry: `renovate-operator.policyAllowedHosts` appends the hostname of the operator's own webhook base URL (`webhook.baseUrl`, or the value `renovate-operator.webhookBaseUrl` derives from `webhook.ingress`/`webhook.route`) whenever `webhook.enabled` is set, deduplicated, because that URL is chart configuration and therefore already admin-approved — requiring it twice only made webhook sync fail on an otherwise complete install. A schemeless override contributes nothing, matching what `url.Hostname()` would resolve. A per-job `spec.webhook.baseUrl` on a different host still needs an explicit entry. `Policy` is resolved once in `main.go` and injected (factory, manager); checks never read the config singleton themselves. New URL-valued spec fields belong in `Policy.ValidateJobDestinations`. Denials increment `renovate_operator_policy_denials_total{check}` (`check="destination"`/`"secret_ref"`; the label never carries the offending value).
- **Policy refusals are visible on the resource** — `Policy.ValidateDestination`/`ValidateReferencedSecret` return a `*policy.Violation` carrying a CamelCase `Reason` (`policy.ReasonFor(err)` extracts it through wrapping), so callers report a reason without parsing messages. The reconciler gates every RenovateJob in `acceptJob` before scheduling anything and records the outcome as the `Accepted` status condition (`api.ConditionAccepted`) via `RenovateJobManager.SetAcceptedCondition` — which **skips the API write when `meta.SetStatusCondition` reports no change**, otherwise the 1-minute requeue churns `resourceVersion` forever. A job that becomes refused also has its schedule removed, or the previously registered cron entry keeps firing. The reconciler is the only condition writer; `CreateDiscoveryJob` and the executor's `acceptedCandidates` refuse independently as defence in depth (the executor skips only the offending job) but never touch status. The UI reads the condition in `ui.acceptedState` — **absent condition means accepted**, so an un-reconciled job is not shown as halted — and `static/index.html` renders a halted banner plus disabled run controls. A halt outranks a missing permission in the disabled-control hint, because the halt blocks everyone and has to be fixed first.
- **Secret-reference opt-in (`internal/policy`)** — any secret the operator reads at a _caller-chosen_ key must carry `renovate-operator.mogenius.com/allow-ref: "true"` (`api.LabelAllowRef`), enforced by `Policy.ValidateReferencedSecret` after the Get and before any key is read, at all three such sites: `NewClientWithTokenRef` (`spec.webhook.sync.secretRef`), `getRenovateJobTokens` (`spec.webhook.authentication.secretRef`) and `readJobCredentials` (`spec.githubAppReference`, three keys). `spec.secretRef` is exempt — it is only read at Renovate's well-known token key names, so it is not an arbitrary-key reference. Toggle via `policy.requireSecretRefOptIn` (env `POLICY_REQUIRE_SECRET_REF_OPT_IN`, default true); the `Policy` field is spelled `AllowUnlabeledSecretRefs` so the zero value is the strict one. Any new code that reads a secret at a key taken from the spec must call `ValidateReferencedSecret` first. Two asymmetries: webhook _removal_ is never gated (the delivery URL is matching input, and a hook written under a hostile base URL must stay deletable), and webhook sync fails open (a denial logs and skips, never blocking discovery).
- **Pod privilege containment** — split by whether a rule needs an opt-out. Absolute invariants live in the CRD as CEL (`+kubebuilder:validation:XValidation`) so the API server rejects them at apply time and no install can switch them off: `hostPath` in `spec.extraVolumes`, plus `privileged` and `allowPrivilegeEscalation` on `spec.securityContext.container`. `MaxItems=64` on both volume lists bounds the CEL cost estimate. Because CEL is GA from 1.29, `Chart.yaml` declares `kubeVersion: ">=1.29.0-0"` — on an older API server the rules would be silently ignored. Tunable rules live in `Policy.ValidateJobSpec`: `policy.allowedServiceAccounts` (empty = namespace default only, which is what closes operator-SA impersonation) and `policy.allowRootUser`. `Policy.ValidateJob` is the single gate combining destinations and spec — new whole-spec checks belong there so every gate site picks them up at once.
- **Image allowlist** — `spec.image` must match a `policy.allowedImages` entry (env `POLICY_ALLOWED_IMAGES`), checked in `Policy.validateImage`. Matching is **exact on the repository**: no prefix or subpath form, so an entry never authorises a repository nobody listed (`ghcr.io/renovatebot` does not imply `ghcr.io/renovatebot/renovate`). `parseImageRef` only strips the tag and digest — the tag colon is the one after the last `/`, so a registry port is not mistaken for a tag — and deliberately does **not** resolve implicit registries: every normalization rule is a chance to map an unexpected input onto an entry. The consequence is that each spelling in use must be listed, which is why the chart ships both `renovate/renovate` and `docker.io/renovate/renovate`. Unparseable references are refused, so the failure direction is always "denied". `TestParseImageRef` is the contract for that parser; keep the table green. Empty denies everything; the chart ships the two official repositories. An empty `spec.image` is left to Kubernetes rather than refused. This is the weakest of the pod controls on its own — the official image is a Node runtime whose config `spec.extraEnv` controls — so treat it as raising cost, not closing a hole.
- **Hardened securityContext is merged, not replaced** — `getPodSecurityContext`/`getContainerSecurityContext` (`internal/renovate/jobDefinitions.go`) deep-copy the spec and fill in only the fields it left unset from `hardenedPodSecurityContext`/`hardenedContainerSecurityContext`. Overriding one field must not silently drop `runAsNonRoot`, the seccomp profile or `drop: ALL`. The deep copy matters: the spec comes from the informer cache, so returning it directly would alias — and filling defaults would mutate — the shared object.
- **Two-role UI access model (`spec.access`)**: access to a RenovateJob in the UI resolves to one of `roleNone` / `roleReader` / `roleAdmin` in `ui/access.go`. `resolveAccess` matches the session's groups against `spec.access.adminGroups`, then `spec.access.readerGroups`, then falls back to `anonymousRead`; no match means the job is hidden. Anonymous read is a **floor**, not a sessionless branch: it grants read to everyone and group matches only add to it, so signing in never removes access. `anonymousReadLogs` is a second opt-in because Renovate logs are unredacted, which is why a role name alone cannot decide log access, so the API ships a `permissions[]` array per job (`logs`, `trigger`, `triggerAll`, `cancel`, `discovery`) and the frontend gates purely on that, with `role` used only for the "Read-only" badge. Debug runs need no permission of their own: the debug flag rides along in the trigger request, so `trigger`/`triggerAll` already gate it. Every `spec.access` field inherits the operator-wide default (`DEFAULT_READER_GROUPS`, `DEFAULT_ADMIN_GROUPS`, `DEFAULT_ANONYMOUS_READ`, `DEFAULT_ANONYMOUS_READ_LOGS`; Helm: `auth.defaultAccess.*`) per field, so a job can add to the defaults but not remove them. **Fail closed**: a job with no access configuration is hidden once auth is enabled. When no auth provider is configured at all there is no identity to evaluate, so every request is admin and `spec.access` is ignored. `spec.allowedGroups` and `DEFAULT_ALLOWED_GROUPS` are deprecated aliases for admin groups; a CEL rule on the spec rejects setting `allowedGroups` and `access` together, and `resolveAccess` fails closed if an un-upgraded CRD accepts both. Enforcement is two-tiered: `isAnonymousReadPath` in `auth.go` allowlists the read routes that may be served without a session (everything else still 302/401, so new endpoints are protected by default), and handlers call `requireRead` / `requirePermission`, which answer `404` for unreadable jobs (no existence disclosure) and `403` for a missing permission. GitHub OAuth gains groups via `GITHUB_ORG_GROUPS` (Helm: `auth.github.orgGroups`), which adds the `read:org` scope and maps membership to `org` and `org/team` at login; without it the provider cannot supply groups and `assertAccessRulesEnforceable` in `main.go` refuses to boot when access groups exist (jobs created after boot are not covered and simply fail closed).
- **Chart values are schema-validated** — `charts/renovate-operator/values.schema.json` (JSON Schema draft 2020-12, requires Helm v4+) is the contract for chart values. The root object and every operator-owned block are `additionalProperties: false`, so typos fail at install time; enums mirror what the operator accepts (`crd.mode`, `image.pullPolicy`, `config.logStorage.mode`, `logging.*`, `telemetry.protocol`, the `*Scheme` values). Pass-through blocks stay permissive (`valkey` for the subchart, `resources`/`affinity`/`securityContext.pod|container`, `metrics.dashboard.grafanaDashboard`). Every new value added to `values.yaml` must be declared here as well, otherwise the chart rejects it.

# Verification

Use the following commands to validate the code:

- `just build`
- `just test-unit`
- `just test-helm`
- `just generate`

# Important

Every change to the structure should be adapted here!
