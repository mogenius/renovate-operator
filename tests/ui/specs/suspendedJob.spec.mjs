// Covers how the dashboard shows a RenovateJob with spec.suspend set.
//
// A suspended job starts nothing new: the operator removes its schedule and
// refuses discovery, but triggering still queues projects for when it is resumed.
// So the card says so, has no countdown to show, disables the discovery button,
// and leaves the trigger button alone.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import { buildProject, buildRenovateJob } from "../fixtures/renovateJobsFixture.mjs";

function buildDashboardWithSuspendedJob() {
  return [
    buildRenovateJob({
      name: "job-suspended",
      suspended: true,
      projects: [buildProject({ name: "acme/paused" })],
    }),
    buildRenovateJob({
      name: "job-active",
      projects: [buildProject({ name: "acme/running-normally" })],
    }),
  ];
}

test.describe("suspended job", () => {
  test("says the job is suspended instead of counting down to a run", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithSuspendedJob());

    const suspendedHeader = dashboard.jobCardHeader("job-suspended");
    await expect(dashboard.jobCard("job-suspended").getByRole("status")).toContainText(
      "Suspended: no new run starts for this job",
    );
    await expect(suspendedHeader.getByText("Suspended", { exact: true })).toBeVisible();

    await expect(dashboard.jobCard("job-active").getByRole("status")).toHaveCount(0);
    await expect(dashboard.jobCardHeader("job-active").getByText("Suspended", { exact: true })).toHaveCount(0);
  });

  test("blocks discovery but still lets projects be queued", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithSuspendedJob());

    await expect(
      dashboard
        .jobCardHeader("job-suspended")
        .getByRole("button", { name: "Suspended: resume the job to run a discovery" }),
    ).toBeDisabled();
    await expect(dashboard.triggerAllButton("job-suspended")).toBeEnabled();

    await expect(
      dashboard.jobCardHeader("job-active").getByRole("button", { name: "Run discovery for job-active" }),
    ).toBeEnabled();
  });
});
