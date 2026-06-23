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

### DCO — Developer Certificate of Origin

Every commit must include a `Signed-off-by` trailer certifying that you have the right to contribute the code under its license. Add it with:

```
git commit --signoff
```

This appends the following line to your commit message:

```
Signed-off-by: Your Name <you@example.com>
```

By signing off you agree to the [DCO](https://developercertificate.org/).

### AI-Assisted Contributions

If any part of a commit was written or materially shaped by an AI tool (e.g. GitHub Copilot, Claude, ChatGPT), add a `Co-Authored-By` footer that names the model:

```
Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
Co-Authored-By: GitHub Copilot <noreply@github.com>
```

Use the model name and version that was active during the session. This keeps authorship transparent in the git history.

### Examples

```
feat(operator): add parallel job execution

Signed-off-by: Your Name <you@example.com>
```

```
fix(helm): correct resource limits in deployment template

Signed-off-by: Your Name <you@example.com>
Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

Breaking change:
```
feat(operator)!: remove v1alpha1 CRD support

BREAKING CHANGE: v1alpha1 CRDs are no longer served. Migrate all
RenovateJob resources to v1beta1 before upgrading.

Signed-off-by: Your Name <you@example.com>
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
