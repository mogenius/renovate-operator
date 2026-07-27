# UI browser tests

Playwright tests for the operator's frontend in `src/static`.

The frontend is a client-side React app: it is served as-is and gets everything it
renders as JSON from `/api/v1/*`. That makes it testable without a Kubernetes
cluster — the tests serve the real `index.html` and stub the API with
`page.route`, so a spec can put the dashboard into any state it needs.

These tests live outside `src/static` on purpose: the Dockerfile copies that whole
tree into the image (`COPY --from=builder /workspace/static /app/static`), so
anything placed there would ship to production and be served by the file server.

## Clicking through the UI by hand

The dashboard renders nothing but an error state without an API — it fetches
`/api/v1/renovatejobs` on mount and every 30s — so a plain file server gives you
zero job cards. `just ui-dev` serves the real `src/static` with a mocked API
instead, no cluster and no Go build:

```sh
just ui-dev                                     # http://127.0.0.1:8099
MOCK_JOB_COUNT=12 MOCK_PROJECTS_PER_JOB=40 just ui-dev   # a heavier dashboard
BASE_PATH=/renovate just ui-dev                 # exercise the sub-path
```

Edit `src/static/index.html` and reload the browser; there is no build step.

The mock (`mockOperatorApi.mjs`) answers the eight endpoints the dashboard calls.
The five POST endpoints return an empty `200` without changing anything, so
triggering a renovate run will not flip a project to Running. It is there to
exercise layout and client-side state — expansion, filters, sorting, theming — not
the operator's job lifecycle. Anything unmocked returns a JSON 404 naming the
route, so a missing endpoint is obvious rather than silent.

## Running

```sh
just test-ui                      # whole suite, headless
just test-ui --headed             # watch it in a browser
just test-ui --ui                 # Playwright's interactive runner
just test-ui --grep "collapsed"   # single test
```

`just test-ui` depends on `just jsInstall`, which downloads the vendored Tailwind,
Babel and React bundles that `index.html` loads. It installs npm dependencies and
the Chromium build on first run.

## Confirming a spec has teeth

A UI test that passes both before and after a change proves nothing.
`test-ui-baseline` serves `index.html` from another git revision while keeping the
current specs, so you can watch a new spec fail against the code it was written
for:

```sh
just test-ui-baseline HEAD                        # before the working-tree change
just test-ui-baseline v5.4.0 --grep "collapsed"   # before a release
```

Only `index.html` is swapped; `components/`, `css/` and `js/` still come from the
working tree.

## Layout

```
tests/ui/
├── playwright.config.mjs        # 1280x900 chromium, starts the static server
├── staticFrontendServer.mjs     # stand-in for src/ui/ui.go — see note below
├── mockOperatorApi.mjs          # mock /api/v1 for `just ui-dev`, off by default
├── fixtures/
│   ├── dashboardFixture.mjs     # API stubs + dashboard page object
│   └── renovateJobsFixture.mjs  # /api/v1/renovatejobs payload builders
└── specs/
    └── jobCardExpansion.spec.mjs
```

`staticFrontendServer.mjs` mirrors `serveHTML` and `registerUiRoutes` in
`src/ui/ui.go` rather than being a generic file server. The frontend only learns
its sub-path from the `<base>` tag and `window.__BASE_PATH__` that the Go server
splices into `<head>`, so serving the files plainly would test a page the operator
never ships. Keep it in sync when those handlers change; setting `BASE_PATH` runs
the suite under a sub-path.

## Writing specs

`renovateJobsFixture.mjs` mirrors `ui.RenovateJobInfo` and
`crdManager.RenovateProjectStatus`. When a field is added to either Go struct, add
it there too — the tests are only as honest as those payloads.

The page carries very few test-friendly hooks, so the page object leans on what is
already meaningful: the one `<h2>` per job card, the `aria-label` on the chevron
(`Expand job details` / `Collapse job details`), and the `title` on the stat
badges. Prefer adding a stable hook to a new component over matching Tailwind
classes. Note that the chevron button is marked `aria-hidden="true"` despite
carrying an `aria-label`, so it has to be matched with a CSS selector rather than
`getByLabel`.
