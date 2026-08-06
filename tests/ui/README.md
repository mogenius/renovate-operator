# UI browser tests

Playwright tests for the client-side React frontend in `src/static`. The tests
serve the real `index.html` and stub its `/api/v1/*` requests, so they do not need
a Kubernetes cluster.

These tests live outside `src/static` on purpose: the Dockerfile copies that whole
tree into the image (`COPY --from=builder /workspace/static /app/static`), so
anything placed there would ship to production and be served by the file server.

## Clicking through the UI by hand

Use `just ui-dev` to serve `src/static` with a mocked API:

```sh
just ui-dev                                     # http://127.0.0.1:8098
MOCK_JOB_COUNT=12 MOCK_PROJECTS_PER_JOB=40 just ui-dev   # a heavier dashboard
BASE_PATH=/renovate just ui-dev                 # exercise the sub-path
```

The development server uses port 8098 while the specs use 8099. Edit
`src/static/index.html` and reload the browser; there is no build step.

The mock supports the dashboard's API endpoints, but POST requests return `200`
without changing fixture state. It is intended for layout and client-side
behavior, not the operator's job lifecycle. Unhandled routes return a JSON 404.

The log links on a project row work as well: `/api/v1/logs` is mocked as a real
event stream whose entries arrive on a timer, so the page streams them in rather
than showing a finished list.

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

`test-ui-baseline` serves `index.html` and `pages/logs.html` from another git
revision while keeping the current specs. Use it to verify that a new spec fails
without its corresponding change:

```sh
just test-ui-baseline HEAD                        # before the working-tree change
just test-ui-baseline v5.4.0 --grep "expansion"   # before a release
```

Only those two pages are swapped; `components/`, `css/` and `js/` still come from
the working tree.

## Layout

```
tests/ui/
├── playwright.config.mjs        # 1280x900 chromium, starts the static server
├── staticFrontendServer.mjs     # stand-in for src/ui/ui.go — see note below
├── mockOperatorApi.mjs          # mock /api/v1 for `just ui-dev`, off by default
├── fixtures/
│   ├── dashboardFixture.mjs     # API stubs + dashboard page object
│   ├── renovateJobsFixture.mjs  # /api/v1/renovatejobs payload builders
│   ├── logsFixture.mjs          # log-stream stub + logs page object
│   └── renovateLogsFixture.mjs  # /api/v1/logs event-stream builders
└── specs/
    ├── jobCardExpansion.spec.mjs
    ├── stickyToolbar.spec.mjs
    └── logsFiltering.spec.mjs
```

`staticFrontendServer.mjs` mirrors `serveHTML` and `registerUiRoutes` in
`src/ui/ui.go`, including its `<base>` and `window.__BASE_PATH__` injection. Keep
it in sync when those handlers change. Set `BASE_PATH` to run under a sub-path.

## Writing specs

`renovateJobsFixture.mjs` mirrors `ui.RenovateJobInfo` and
`crdManager.RenovateProjectStatus`. When a field is added to either Go struct, add
it there too — the tests are only as honest as those payloads.

`PROJECT_STATE_VARIANTS` in that file is the catalogue of project states the
dashboard renders differently: every `renovateResultStatus` the log parser emits,
each combination of PR activity and log issues, and the run states in between. Add
a variant when the UI grows a case it treats specially — the two dashboards built
from it follow along:

- `buildDashboardWithEveryProjectState()` — one job, one project per variant, each
  named after its variant key, so a spec can say which states a filter keeps
- `buildMultiJobDashboard()` — the many-jobs payload the expansion specs and
  `just ui-dev` use; every card leads with the states the "Hide Projects" menu can
  act on and fills the rest rotated by job index

`renovateLogsFixture.mjs` does the same for the logs page. `/api/v1/logs` is an
event stream, so the fixture builds the frames `getRenovateJobLogs` writes and the
spec serves the whole body in one `route.fulfill` — the browser's `EventSource`
parses it frame by frame exactly as it would a live stream.

Prefer semantic locators or stable hooks (`aria-label`, `title`, or
`data-testid`) over Tailwind classes. The job-card chevron is `aria-hidden`, so
the page object locates it with CSS rather than `getByLabel`. A log row has no
role of its own, so `logsFixture.mjs` locates rows by their message and their
level label — which means the level dropdown has to be closed before counting
rows, since it repeats those same words.
