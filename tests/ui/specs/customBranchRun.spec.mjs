// Covers the "Run with custom branch" action on a project row
// (mogenius/renovate-operator#685).
//
// It queues one run of one repository against a branch the user names, so a Renovate
// config change can be tried before it is merged. The branch only travels in that
// trigger's executionOptions; nothing about the job or the project changes.

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

test.describe("run with custom branch", () => {
  test("sends the branch and the chosen options with a single-project trigger", async ({ dashboard }) => {
    await dashboard.open(buildSingleProjectDashboard());

    await dashboard.customBranchRunButton("acme/api").click();
    const dialog = dashboard.customBranchRunDialog;
    await dialog.getByLabel("Branch", { exact: true }).fill("  test/renovate-config ");
    await dialog.getByLabel(/Dry run/).check();

    const sent = dashboard.page.waitForRequest(
      (request) => request.method() === "POST" && /\/api\/v1\/renovate$/.test(request.url()),
    );
    await dialog.getByRole("button", { name: "Run", exact: true }).click();
    await sent;

    await expect(dialog).toBeHidden();
    expect(dashboard.triggerRequests).toEqual([
      {
        renovateJob: "team-platform",
        namespace: "renovate",
        project: "acme/api",
        executionOptions: { baseBranch: "test/renovate-config", dryRun: true },
      },
    ]);
  });

  test("will not submit a branch the operator would reject", async ({ dashboard }) => {
    await dashboard.open(buildSingleProjectDashboard());

    await dashboard.customBranchRunButton("acme/api").click();
    const dialog = dashboard.customBranchRunDialog;
    const runButton = dialog.getByRole("button", { name: "Run", exact: true });

    await expect(runButton).toBeDisabled();

    // A Renovate regex pattern would match more than the one branch being tested.
    await dialog.getByLabel("Branch", { exact: true }).fill("/^release-.*/");
    await expect(runButton).toBeDisabled();
    await expect(dialog.getByLabel("Branch", { exact: true })).toHaveAttribute("aria-invalid", "true");

    await dialog.getByLabel("Branch", { exact: true }).fill("feature/../main");
    await expect(runButton).toBeDisabled();

    await dialog.getByLabel("Branch", { exact: true }).press("Escape");
    await expect(dialog).toBeHidden();
    expect(dashboard.triggerRequests).toEqual([]);
  });

  test("warns that a run without dry run opens real pull requests", async ({ dashboard }) => {
    await dashboard.open(buildSingleProjectDashboard());

    await dashboard.customBranchRunButton("acme/api").click();
    const dialog = dashboard.customBranchRunDialog;
    const warning = dialog.getByText(/opens and updates real pull requests/);

    await expect(warning).toBeVisible();
    await dialog.getByLabel(/Dry run/).check();
    await expect(warning).toBeHidden();
  });

  test("names the queued branch while the run waits", async ({ dashboard }) => {
    await dashboard.open(
      buildSingleProjectDashboard({
        project: {
          status: PROJECT_STATUSES.scheduled,
          priority: 2,
          executionOptions: { baseBranch: "test/renovate-config", dryRun: true },
        },
      }),
    );

    await expect(dashboard.customBranchRunButton("acme/api")).toHaveAttribute(
      "aria-label",
      "Queued against branch test/renovate-config (dry run)",
    );
  });

  test("is not offered to a reader", async ({ dashboard }) => {
    await dashboard.open(
      buildSingleProjectDashboard({ job: { role: "reader", permissions: READER_PERMISSIONS } }),
    );

    await expect(dashboard.customBranchRunButton("acme/api")).toBeDisabled();
  });
});
