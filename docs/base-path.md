# Serving the UI under a Sub-Path

By default the operator serves its web UI, API, authentication and health
endpoints from the root path (`/`). If you want to co-host the operator with
other applications on the same hostname, you can serve everything under a
configurable sub-path (for example `https://example.com/renovate`).

## Configuration

Set the top-level `basePath` value in the Helm chart:

```yaml
basePath: /renovate
```

Leaving `basePath` empty (the default) keeps everything on the root path, so
existing installations are unaffected.

The value is normalized to have a leading slash and no trailing slash, so
`renovate/`, `/renovate` and `renovate` all resolve to `/renovate`.

## What the sub-path affects

When `basePath` is set, the chart wires it into several places:

- The `BASE_PATH` environment variable on the operator deployment. The backend
  mounts all UI, API, auth and health routes under this prefix and redirects
  `/` to the sub-path.
- The `Ingress` / `HTTPRoute` path, so traffic to the sub-path is routed to the
  operator.
- The auto-detected OAuth / OIDC redirect URLs, which get the sub-path appended
  (e.g. `https://example.com/renovate/auth/callback`).

The frontend reads the injected base path (`<base href>` and
`window.__BASE_PATH__`) and builds all runtime URLs relative to it, so no
additional rewrite rules are required on the Ingress.

## Authentication note

> **Important:** OAuth / OIDC redirect URLs registered with your identity
> provider must include the sub-path. For example, register
> `https://example.com/renovate/auth/callback` instead of
> `https://example.com/auth/callback`.

## Running without Helm

If you run the operator directly, set the `BASE_PATH` environment variable:

```sh
BASE_PATH=/renovate
```
