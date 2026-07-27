// Covers the job-card expansion behaviour of the dashboard.
//
// Before this change every JobCard held its own `useState(true)`, so the page
// opened with every card expanded and every project of every job rendered. On a
// realistic install — a handful of RenovateJobs with a dozen repositories each —
// that pushed the second job's title far below the fold and gave the user no way
// back other than collapsing each card by hand on every reload.
//
// The tests below pin the three properties that fix relies on: cards start
// collapsed, an explicit expansion survives a reload, and the bulk controls only
// touch the jobs the current stat filter leaves visible.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import {
  buildMultiJobDashboard,
  buildDashboardWithTwoFailingJobs,
} from "../fixtures/renovateJobsFixture.mjs";

test.describe("job card expansion", () => {
  test("opens collapsed so the whole dashboard stays scannable", async ({ dashboard }) => {
    const renovateJobs = buildMultiJobDashboard({ jobCount: 5, projectsPerJob: 12 });

    await dashboard.open(renovateJobs);

    // The old always-open card rendered all 5 x 12 projects immediately.
    await expect(dashboard.projectRows).toHaveCount(0);

    // Measured at the 1280x900 viewport this suite pins: 1.3 screens with the
    // cards collapsed, 5.8 screens with the 60 project rows expanded.
    expect(await dashboard.heightInViewports()).toBeLessThan(2);

    // The first four job titles now share the opening screen.
    for (const renovateJob of renovateJobs.slice(0, 4)) {
      await expect(dashboard.jobHeading(renovateJob.name)).toBeInViewport();
    }
  });

  test("remembers which jobs the user expanded across a reload", async ({ dashboard }) => {
    const renovateJobs = buildMultiJobDashboard({ jobCount: 5, projectsPerJob: 12 });

    await dashboard.open(renovateJobs);
    await dashboard.toggle("team-02");

    await expect(dashboard.projectCell("acme/service-2-1")).toBeVisible();
    expect(await dashboard.readStoredExpandedJobKeys()).toEqual(["renovate/team-02"]);

    await dashboard.reload();

    expect(await dashboard.isExpanded("team-02")).toBe(true);
    await expect(dashboard.projectCell("acme/service-2-1")).toBeVisible();

    // Only the explicitly expanded job comes back open.
    expect(await dashboard.isExpanded("team-01")).toBe(false);
    expect(await dashboard.isExpanded("team-03")).toBe(false);
  });

  test("expand all and collapse all only touch the filtered jobs", async ({ dashboard }) => {
    const renovateJobs = buildDashboardWithTwoFailingJobs();

    // ?filter=failed hides job-all-green: it has no failing project.
    await dashboard.open(renovateJobs, { statFilter: "failed", expectedJobCount: 2 });
    await expect(dashboard.jobHeading("job-all-green")).toHaveCount(0);

    await dashboard.expandAllButton.click();
    expect(await dashboard.isExpanded("job-failing-a")).toBe(true);
    expect(await dashboard.isExpanded("job-failing-b")).toBe(true);
    await expect(dashboard.expandAllButton).toBeDisabled();

    await dashboard.clearStatFilterButton.click();
    await expect(dashboard.jobHeadings).toHaveCount(3);

    // The job that was filtered out was never expanded behind the user's back.
    expect(await dashboard.isExpanded("job-all-green")).toBe(false);
    expect(await dashboard.readStoredExpandedJobKeys()).toEqual([
      "renovate/job-failing-a",
      "renovate/job-failing-b",
    ]);

    // Collapse all now sees all three, so it clears the two that are open.
    await dashboard.collapseAllButton.click();
    expect(await dashboard.isExpanded("job-failing-a")).toBe(false);
    expect(await dashboard.isExpanded("job-failing-b")).toBe(false);
    await expect(dashboard.collapseAllButton).toBeDisabled();
    expect(await dashboard.readStoredExpandedJobKeys()).toEqual([]);
  });

  test("expand all follows the search box, not the full job list", async ({ dashboard }) => {
    const renovateJobs = buildDashboardWithTwoFailingJobs();

    // The search box narrows to projects matching "broken", which only the two
    // job-failing-* jobs have.
    await dashboard.open(renovateJobs, { search: "broken", expectedJobCount: 2 });
    await expect(dashboard.jobHeading("job-all-green")).toHaveCount(0);

    await dashboard.expandAllButton.click();
    await expect(dashboard.expandAllButton).toBeDisabled();

    // Reopening without the search term brings the third job back untouched.
    await dashboard.open(renovateJobs);
    expect(await dashboard.isExpanded("job-failing-a")).toBe(true);
    expect(await dashboard.isExpanded("job-all-green")).toBe(false);
    expect(await dashboard.readStoredExpandedJobKeys()).toEqual([
      "renovate/job-failing-a",
      "renovate/job-failing-b",
    ]);
  });
});
