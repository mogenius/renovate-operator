# Contributing to Renovate Operator

## Commit Conventions

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification. Commits drive our automated versioning and changelog via semantic-release, so well-formed messages are important.

### Format

```
<type>(<scope>): <description>
```

**Breaking changes** require both:
1. `!` after the type/scope in the subject line
2. A `BREAKING CHANGE:` footer in the commit body explaining what breaks and why

```
feat(operator)!: remove v1alpha1 CRD support

BREAKING CHANGE: v1alpha1 CRDs are no longer served. Migrate all
RenovateJob resources to v1beta1 before upgrading.
```

### Types

| Type | When to use |
|------|-------------|
| `feat` | Introduces new functionality |
| `fix` | Fixes a bug |
| `docs` | Documentation-only changes |
| `chore` | Maintenance, dependency bumps, tooling |
| `refactor` | Code change without behavior change |
| `test` | Adding or updating tests |
| `perf` | Performance improvement |
| `ci` | CI/CD pipeline changes |
| `build` | Build system or Dockerfile changes |
| `revert` | Reverts a previous commit |

### Scopes

Scopes are optional but encouraged. Use one of the following to indicate which part of the project is affected:

| Scope      | Area                                                      |
|------------|-----------------------------------------------------------|
| `operator` | Core operator logic (controllers, scheduler, webhooks, …) |
| `helm`     | Helm chart and templates                                  |
| `docs`     | Documentation in `docs/`                                  |
| `deps`     | Dependency updates (`go.mod`, …)                          |
| `actions`  | Changes to the GitHub Actions                             |

### Examples

```
feat(operator): add parallel job execution
fix(helm): correct resource limits in deployment template
docs: update installation guide for v2
chore(deps): update golang to 1.26
refactor(operator): extract scheduler into separate package
ci: add conventional commit validation on PRs
```

Breaking change:
```
feat(operator)!: remove v1alpha1 CRD support

BREAKING CHANGE: v1alpha1 CRDs are no longer served. Migrate all
RenovateJob resources to v1beta1 before upgrading.
```

---

## Branch Conventions

Branch names should be short and descriptive. Prefix with the change type:

```
feat/<short-description>
fix/<short-description>
chore/<short-description>
docs/<short-description>
```

Examples: `feat/parallel-execution`, `fix/helm-resource-limits`, `docs/webhook-setup`.

---

## Pull Requests

- Keep PRs **focused and small** — one logical change per PR.
- The PR title should itself follow the conventional commits format.
- Describe **what** changed and **why**, not just how.
- All CI checks must pass before merging.
- At least one approval is required before merging.

---

## Code Style

- Format Go code with `gofmt` before committing.
- Run `just check` locally (tests + lint + generate) before opening a PR.
- New operator features should include unit tests.
- Avoid dead code — remove unused exports, types, and helpers.

---

## Documentation

- New user-facing features should come with documentation in `docs/`.
- Update the relevant section of `README.md` if the feature affects the quick-start or feature list.
- Helm value changes should be reflected in `charts/renovate-operator/values.yaml` comments.
