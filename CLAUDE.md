# Renovate Operator — Claude Code Guide

A Kubernetes-native operator that runs [Renovate](https://github.com/renovatebot/renovate) for automated dependency updates: CRDs, cron scheduling, parallel Job execution, and a web UI, abstracted over multiple Git platforms.

## Directory Structure

```
src/
├── api/v1alpha1/        # CRD types; well-known label/annotation/finalizer keys (see Invariants)
├── assert/              # panic-on-critical-error helpers for startup/init failures
├── clientProvider/      # Kubernetes client factory
├── cmd/                 # main.go — wires everything together
├── config/              # singleton env-var config, schema-validated
├── controllers/         # controller-runtime reconciliation loop
├── github/              # GitHub App secret naming helpers
├── gitProviderClients/  # Git provider abstraction
│   ├── factory/         # creates the right provider from job.Spec.Provider
│   └── {github,gitlab,gitea,forgejo,bitbucket}Provider/
├── health/              # thread-safe health state, exposed on the HTTP health endpoint
├── integration/         # over-the-wire tests, build tag `integration`
├── internal/
│   ├── crdManager/      # CRUD on RenovateJob CRDs and Kubernetes Jobs
│   ├── kvstore/         # KV store abstraction (Valkey/Redis-backed)
│   ├── logStore/        # most-recent log output per (namespace, job, project)
│   ├── objectstore/     # S3 client for log archival
│   ├── podLogs/         # streams logs from executor Job pods
│   ├── webhookSync/     # provider-agnostic repo webhook sync
│   ├── policy/          # install-wide guard rails on RenovateJob specs
│   ├── parser/          # extracts discovered repos and dependency issues from Renovate logs
│   ├── renovate/        # discovery, executor, job definitions (jobDefinitions.go)
│   ├── telemetry/       # OpenTelemetry wiring
│   ├── types/           # shared internal types
│   └── utils/           # platform endpoints, job naming helpers
├── metricStore/         # Prometheus metrics
├── scheduler/           # cron scheduling wrapper
├── static/              # frontend assets (components/, css/, js/, pages/)
├── ui/                  # web UI server, auth (OIDC, GitHub OAuth), API endpoints
└── webhook/             # HTTP webhook server (per-platform trigger endpoints)
charts/                  # Helm chart
docs/                    # user-facing docs — see Docs below
tests/ui/                  # Playwright browser tests for src/static (see its README)
```

## Conventions

1. **Interfaces first.** Anything touching a Git provider, scheduler, executor, or external system is an interface, implemented per platform — never branch shared code on provider type. Extend existing interfaces (`gitProviderClients.GitProviderClient` — repo info incl. fork/pending-deletion detection in one call, webhooks, repo search; `ui.AuthProvider`; `renovate.DiscoveryAgent`/`RenovateExecutor`; `scheduler.Scheduler`); don't replace them.
2. **Git provider factory.** New provider: package under `gitProviderClients/<name>Provider/`, implement `GitProviderClient` fully, register in `gitProviderClients/factory/`. Never instantiate a provider client outside the factory.
3. **Jobs, not Pods.** Renovate runs as Kubernetes Jobs for TTL cleanup and restart semantics. Extend templates in `internal/renovate/jobDefinitions.go`, never build job specs inline elsewhere.
4. **Config via `config.GetValue()`.** Never read `os.Getenv` outside `config/`; declare new values in the config schema.
5. **Errors:** wrap with `fmt.Errorf("...: %w", err)`. `assert.NoError`/`assert.Assert` only for unrecoverable startup failures. External API errors fail open where exclusion would be worse than inclusion (e.g. fork filtering keeps repos on API failure).
6. **Concurrency:** `sync.Mutex`/`RWMutex` for shared state; channel semaphores to cap external API parallelism (pattern in `forkFilter.go`, max 10). Executor polls every 10s, respecting the CRD's `parallelism`.
7. **Logging:** `logr.Logger`, injected — never a global. `Info` for normal events, `V(1).Info` for verbose, `Error` for errors. Never `fmt.Println`/`log.Print`.
8. **Reconcilers:** idempotent; `ctrl.Result{RequeueAfter: ...}` for scheduled requeues, `ctrl.Result{}, err` for immediate retry.
9. **Health state** lives in `health/`, not locally in components.
10. **Naming:** Job names via `internal/utils/jobNames.go`, platform base URLs via `internal/utils/platformEndpoints.go`. Labels: `renovate-operator.mogenius.com/<key>`.

## Invariants

Non-obvious rules that silently break security or state if skipped:

- A new `Policy`/`AccessDefaults` field that gates behavior must be spelled so its **zero value enforces** (e.g. `Disabled`, `AllowUnlabeledSecretRefs`, `AuthorizationDisabled`) — tests construct these structs bare, and a permissive zero value would silently void enforcement.
- Adding a field to `sessionData` that access control reads requires bumping `currentSessionVersion` in `ui/auth.go`, or sessions minted before the field existed resolve to no access instead of failing back to login.
- Secrets read at a caller-chosen key (i.e. not Renovate's well-known token names) require the `api.LabelAllowRef` label and a `Policy.ValidateReferencedSecret` call before any key is read.
- New URL-valued spec fields need a check in `Policy.ValidateJobDestinations`; host matching is exact, no wildcards or subtrees.
- New label/annotation/finalizer keys go in `api/v1alpha1` (`well_known_labels.go`/`annotation_key_constants.go`) and must be added to `ownedKeys` in `well_known_labels_test.go`.
- Every new `values.yaml` key needs a matching entry in `charts/renovate-operator/values.schema.json` (`additionalProperties: false` throughout) or installs reject it.
- ConfigMaps/Secrets read by key from a spec bypass the informer cache (`main.go`'s `DisableFor`) — fetch live, don't assume they're cached.

## Docs

`docs/` (start at `docs/README.md`) is the source of truth for user-facing behavior, configuration options, platform setup, and security/access-control rationale. This file covers only what's needed to edit the code correctly — don't duplicate `docs/` content here.

## Verification

Run `just check` before finishing (regenerates CRDs, lints, unit tests,  Helm unit tests). Run `just generate` alone after changing CRD types or well-known keys. Run `just test-ui` (only when `src/static` changed — browser tests, needs Chromium).

Keep the directory map and invariants above in sync with the code as it changes; put design rationale in `docs/`, not here.
