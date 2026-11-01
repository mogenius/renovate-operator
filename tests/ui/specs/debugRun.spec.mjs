// Covers the debug-run button on a project row and its dialog
// (mogenius/renovate-operator#685).
//
// One button opens a "Debug or dry run" dialog. On the default branch it queues a normal run with
// debug logging. On a specific branch it queues one dry run of that branch, using the
// Renovate config found on it, so a config change can be tried before it is merged. Debug
// logging starts ticked; it can only be unticked for a specific branch, since a plain run of
// the default branch is what the Trigger button already does. The options only travel in
// that trigger's executionOptions; nothing about the job or the project changes.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import {
  buildProject,
  buildRenovateJob,
  PROJECT_STATUSES,
  READER_PERMISSIONS,
} from "../fixtures/renovateJobsFixture.mjs";

function buildSingleProjectDashboard({ project = {}, job = {} } = {}) {
  return [
    buildRenovateJob({
      name: "team-platform",
      projects: [buildProject({ name: "acme/api", status: PROJECT_STATUSES.completed, ...project })],
      ...job,
    }),
  ];
}

async function submit(dashboard) {
  const sent = dashboard.page.waitForRequest(
    (request) => request.method() === "POST" && /\/api\/v1\/renovate$/.test(request.url()),
  );
  await dashboard.debugRunDialog.getByRole("button", { name: /^Start (debug|dry) run$/ }).click();
  await sent;
  await expect(dashboard.debugRunDialog).toBeHidden();
}

test.describe("debug run", () => {
  test("starts on the default branch with debug logging locked on", async ({ dashboard }) => {
    await dashboard.open(buildSingleProjectDashboard());

    await dashboard.debugRunButton("acme/api").click();
    const dialog = dashboard.debugRunDialog;

    await expect(dialog.getByRole("radio", { name: /on the default branch/ })).toBeChecked();
    await expect(dialog.getByLabel("Branch name")).toHaveCount(0);
    const debug = dialog.getByRole("checkbox", { name: /Debug log level/ });
    await expect(debug).toBeChecked();
    await expect(debug).toBeDisabled();
    // the dialog names what each option does, and the button what will start
    await expect(dialog.getByRole("radio", { name: /on the default branch/ })).toHaveAccessibleName(/Debug run on the default branch/);
    await expect(dialog.getByRole("radio", { name: /on a specific branch/ })).toHaveAccessibleName(/Dry run on a specific branch/);
    await expect(dialog.getByRole("button", { name: "Start debug run" })).toBeVisible();

    await submit(dashboard);
    expect(dashboard.triggerRequests).toEqual([
      { renovateJob: "team-platform", namespace: "renovate", project: "acme/api", executionOptions: { debug: true } },
    ]);
  });

  test("on a specific branch sends a dry run with debug logging", async ({ dashboard }) => {
    await dashboard.open(buildSingleProjectDashboard());

    await dashboard.debugRunButton("acme/api").click();
    const dialog = dashboard.debugRunDialog;
    await dialog.getByRole("radio", { name: /on a specific branch/ }).check();
    await dialog.getByLabel("Branch name").fill("  test/renovate-config ");

    await expect(dialog.getByText(/This is always a dry run/)).toBeVisible();
    await expect(dialog.getByRole("checkbox", { name: /Debug log level/ })).toBeChecked();
    await expect(dialog.getByRole("button", { name: "Start dry run" })).toBeVisible();

    await submit(dashboard);
    expect(dashboard.triggerRequests).toEqual([
      {
        renovateJob: "team-platform",
        namespace: "renovate",
        project: "acme/api",
        executionOptions: { baseBranch: "test/renovate-config", dryRun: true, debug: true },
      },
    ]);
  });

  test("on a specific branch debug logging can be switched off", async ({ dashboard }) => {
    await dashboard.open(buildSingleProjectDashboard());

    await dashboard.debugRunButton("acme/api").click();
    const dialog = dashboard.debugRunDialog;
    await dialog.getByRole("radio", { name: /on a specific branch/ }).check();
    await dialog.getByLabel("Branch name").fill("feature/x");
    const debug = dialog.getByRole("checkbox", { name: /Debug log level/ });
    await expect(debug).toBeEnabled();
    await debug.uncheck();

    await submit(dashboard);
    expect(dashboard.triggerRequests[0].executionOptions).toEqual({ baseBranch: "feature/x", dryRun: true });
  });

  test("will not submit a branch the operator would reject", async ({ dashboard }) => {
    await dashboard.open(buildSingleProjectDashboard());

    await dashboard.debugRunButton("acme/api").click();
    const dialog = dashboard.debugRunDialog;
    await dialog.getByRole("radio", { name: /on a specific branch/ }).check();
    const runButton = dialog.getByRole("button", { name: "Start dry run" });
    const branchField = dialog.getByLabel("Branch name");

    await expect(runButton).toBeDisabled();

    // A Renovate regex pattern would match more than the one branch being tested.
    await branchField.fill("/^release-.*/");
    await expect(runButton).toBeDisabled();
    await expect(branchField).toHaveAttribute("aria-invalid", "true");

    await branchField.fill("feature/../main");
    await expect(runButton).toBeDisabled();

    await branchField.press("Escape");
    await expect(dialog).toBeHidden();
    expect(dashboard.triggerRequests).toEqual([]);
  });

  test("explains on hover that a branch can be chosen", async ({ dashboard }) => {
    await dashboard.open(buildSingleProjectDashboard());

    const tooltip = dashboard.page.getByRole("tooltip").filter({ hasText: "Debug or dry run" }).first();
    await expect(tooltip).toBeHidden();
    await dashboard.debugRunButton("acme/api").hover();
    await expect(tooltip).toBeVisible();
    await expect(tooltip).toContainText("Debug run on the default branch");
    await expect(tooltip).toContainText("Dry run on a specific branch");
  });

  test("says what is queued", async ({ dashboard }) => {
    await dashboard.open(
      buildSingleProjectDashboard({
        project: {
          status: PROJECT_STATUSES.scheduled,
          priority: 2,
          executionOptions: { baseBranch: "test/renovate-config", dryRun: true, debug: true },
        },
      }),
    );

    await expect(dashboard.debugRunButton("acme/api")).toHaveAttribute(
      "aria-label",
      "Queued: dry run against branch test/renovate-config with debug logs",
    );
  });

  test("says when a debug run on the default branch is queued", async ({ dashboard }) => {
    await dashboard.open(
      buildSingleProjectDashboard({
        project: { status: PROJECT_STATUSES.scheduled, priority: 2, executionOptions: { debug: true } },
      }),
    );

    await expect(dashboard.debugRunButton("acme/api")).toHaveAttribute(
      "aria-label",
      "Queued: debug run on the default branch",
    );
  });

  test("is not offered to a reader", async ({ dashboard }) => {
    await dashboard.open(
      buildSingleProjectDashboard({ job: { role: "reader", permissions: READER_PERMISSIONS } }),
    );

    await expect(dashboard.debugRunButton("acme/api")).toBeDisabled();
  });
});
