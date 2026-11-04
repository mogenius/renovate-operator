// Covers how the dashboard shows a RenovateJob with spec.suspend set.
//
// A suspended job starts nothing new: the operator removes its schedule and
// refuses discovery, but triggering still queues projects for when it is resumed.
// So the card says so, has no countdown to show, disables the discovery button,
// and leaves the trigger button alone. Where the operator offers it, admins flip
// the state from the card itself, or from the toolbar's single toggle, which
// suspends whatever is still active unless every job is already suspended, in
// which case it offers to resume them instead.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import {
  READER_PERMISSIONS,
  buildProject,
  buildRenovateJob,
} from "../fixtures/renovateJobsFixture.mjs";

function buildDashboardWithSuspendedJob({ suspendFromUI = false } = {}) {
  return [
    buildRenovateJob({
      name: "job-suspended",
      suspended: true,
      suspendFromUI,
      projects: [buildProject({ name: "acme/paused" })],
    }),
    buildRenovateJob({
      name: "job-active",
      suspendFromUI,
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

test.describe("suspend toggle", () => {
  test("is not offered unless the operator enables it", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithSuspendedJob());

    await expect(dashboard.suspendToggle("job-active")).toHaveCount(0);
    await expect(dashboard.suspendToggle("job-suspended")).toHaveCount(0);
  });

  test("an admin suspends an active job from its card", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithSuspendedJob({ suspendFromUI: true }));

    await expect(dashboard.suspendToggle("job-active")).toHaveText("Suspend");
    await dashboard.toggleSuspend("job-active");

    expect(dashboard.suspendRequests).toEqual([
      { renovateJobs: [{ name: "job-active", namespace: "renovate" }], suspend: true },
    ]);
  });

  test("an admin resumes a suspended job", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithSuspendedJob({ suspendFromUI: true }));

    await expect(dashboard.suspendToggle("job-suspended")).toHaveText("Resume");
    await dashboard.toggleSuspend("job-suspended");

    expect(dashboard.suspendRequests).toEqual([
      { renovateJobs: [{ name: "job-suspended", namespace: "renovate" }], suspend: false },
    ]);
  });

  test("a reader cannot suspend a job", async ({ dashboard }) => {
    await dashboard.open([
      buildRenovateJob({
        name: "job-read-only",
        suspendFromUI: true,
        role: "reader",
        permissions: READER_PERMISSIONS,
        projects: [buildProject({ name: "acme/watched" })],
      }),
    ]);

    const toggle = dashboard.suspendToggle("job-read-only");
    await expect(toggle).toBeDisabled();
    await toggle.hover();
    await expect(dashboard.page.getByRole("tooltip", { name: "Requires admin access" })).toBeVisible();
  });
});

test.describe("suspend/resume toggle", () => {
  test("is not offered unless the operator enables it", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithSuspendedJob());

    await expect(dashboard.suspendResumeAllButton).toHaveCount(0);
  });

  test("an admin suspends every active job with one request", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithSuspendedJob({ suspendFromUI: true }));

    // job-active is still running, so the toggle offers to suspend it.
    await expect(dashboard.suspendResumeAllButton).toHaveAccessibleName("Suspend all jobs");
    await dashboard.toggleSuspendAll();

    // One request naming them all, so a tab dying mid-way cannot leave some
    // running; the job already suspended is left out.
    expect(dashboard.suspendRequests).toEqual([
      { renovateJobs: [{ name: "job-active", namespace: "renovate" }], suspend: true },
    ]);
  });

  test("still applies to the jobs a search hides", async ({ dashboard }) => {
    // Only the suspended job matches, but the active one still needs suspending.
    await dashboard.open(buildDashboardWithSuspendedJob({ suspendFromUI: true }), {
      search: "paused",
      expectedJobCount: 1,
    });

    await dashboard.suspendResumeAllButton.hover();
    await expect(
      dashboard.page.getByRole("tooltip", { name: "Stop new runs for 1 job, including any the filter hides" }),
    ).toBeVisible();
    await dashboard.toggleSuspendAll();

    expect(dashboard.suspendRequests).toEqual([
      { renovateJobs: [{ name: "job-active", namespace: "renovate" }], suspend: true },
    ]);
  });

  test("offers to resume only once every job you can suspend already is", async ({ dashboard }) => {
    // One of two jobs is still active: a partial mix suspends, it does not resume.
    await dashboard.open(buildDashboardWithSuspendedJob({ suspendFromUI: true }));
    await expect(dashboard.suspendResumeAllButton).toHaveAccessibleName("Suspend all jobs");

    // Every job suspended: the toggle flips to offer resuming them instead.
    await dashboard.open(
      ["job-a", "job-b"].map((name) =>
        buildRenovateJob({
          name,
          suspended: true,
          suspendFromUI: true,
          projects: [buildProject({ name: `acme/${name}` })],
        }),
      ),
    );
    await expect(dashboard.suspendResumeAllButton).toHaveAccessibleName("Resume all jobs");
  });

  test("an admin resumes every suspended job with one request", async ({ dashboard }) => {
    await dashboard.open(
      ["job-a", "job-b"].map((name) =>
        buildRenovateJob({
          name,
          suspended: true,
          suspendFromUI: true,
          projects: [buildProject({ name: `acme/${name}` })],
        }),
      ),
    );

    await dashboard.toggleSuspendAll();

    expect(dashboard.suspendRequests).toEqual([
      {
        renovateJobs: [
          { name: "job-a", namespace: "renovate" },
          { name: "job-b", namespace: "renovate" },
        ],
        suspend: false,
      },
    ]);
  });

  test("still resumes the jobs a search hides", async ({ dashboard }) => {
    const renovateJobs = ["job-a", "job-b"].map((name) =>
      buildRenovateJob({
        name,
        suspended: true,
        suspendFromUI: true,
        projects: [buildProject({ name: `acme/${name}` })],
      }),
    );

    // Only job-a matches, but job-b still needs resuming.
    await dashboard.open(renovateJobs, { search: "job-a", expectedJobCount: 1 });

    await dashboard.suspendResumeAllButton.hover();
    await expect(
      dashboard.page.getByRole("tooltip", {
        name: "Resume 2 suspended jobs, including any the filter hides, whatever suspended them",
      }),
    ).toBeVisible();
    await dashboard.toggleSuspendAll();

    expect(dashboard.suspendRequests).toEqual([
      {
        renovateJobs: [
          { name: "job-a", namespace: "renovate" },
          { name: "job-b", namespace: "renovate" },
        ],
        suspend: false,
      },
    ]);
  });

  test("a reader cannot suspend or resume", async ({ dashboard }) => {
    await dashboard.open(
      ["job-a", "job-b"].map((name) =>
        buildRenovateJob({
          name,
          suspendFromUI: true,
          role: "reader",
          permissions: READER_PERMISSIONS,
          projects: [buildProject({ name: `acme/${name}` })],
        }),
      ),
    );

    await expect(dashboard.suspendResumeAllButton).toBeDisabled();
    await dashboard.suspendResumeAllButton.hover();
    await expect(dashboard.page.getByRole("tooltip", { name: "Requires admin access" })).toBeVisible();
  });
});
