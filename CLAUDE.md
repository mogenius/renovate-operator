# Renovate Operator — Claude Code Guide

## Project Overview

A Kubernetes-native operator that runs [Renovate](https://github.com/renovatebot/renovate) for automated dependency updates. It manages CRDs, cron scheduling, parallel job execution, and a web UI — all while abstracting over multiple Git platforms.

## Directory Structure

```
src/
├── api/v1alpha1/          # Kubernetes CRD API types (RenovateJob, specs, status)
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
│   ├── forgejo/           # Forgejo-specific webhook sync
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
  - `gitProviderClients.GitProviderClient` — fork detection, webhooks, repo search
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

## Technology Stack

| Concern | Library |
|---|---|
| Operator framework | `sigs.k8s.io/controller-runtime` |
| Scheduling | `github.com/netresearch/go-cron` |
| HTTP routing | `github.com/gorilla/mux` |
| Metrics | `github.com/prometheus/client_golang` |
| Logging | `github.com/go-logr/logr` + `go.uber.org/zap` |
| OIDC auth | `github.com/coreos/go-oidc` + `golang.org/x/oauth2` |

## Key Architectural Decisions

- **Leader election** — only the leader runs the executor and scheduler to prevent duplicate job launches
- **Platform credentials** live in Kubernetes Secrets, referenced from the CRD — never hardcoded
- **Webhook servers** are platform-specific (`/webhook/v1/gitlab`, `/github`, `/forgejo`) but trigger the same internal scheduling interface
- **Discovery uses Renovate itself** — a discovery Job runs Renovate with `autodiscover: true` and its JSON logs are parsed to extract repositories
- **Executor uses two global passes per tick** — Pass 1 (`reconcileRunning`) checks all Running projects across ALL RenovateJobs and updates their statuses; Pass 2 (`dispatchScheduled`) collects all Scheduled projects into a flat sorted list and dispatches up to limits. This avoids per-job isolation and enables fair global scheduling.
- **Global parallelism limit** — `GLOBAL_PARALLELISM_LIMIT` env var (Helm: `config.globalParallelismLimit`, default `0` = unlimited) caps total concurrent executor jobs across all RenovateJobs. Per-job `Spec.Parallelism` is still enforced as an additional gate.
- **Anti-starvation via priority-then-oldest-wait sort** — in Pass 2, candidates are sorted first by `Priority` descending, then by the oldest `LastRun` time among Scheduled projects in their RenovateJob. Among equal-priority candidates, the job that has been waiting longest dispatches first, preventing starvation.

# Important

Every change to the structure should be adapted here!
