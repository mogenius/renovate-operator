// Covers the job-card expansion behaviour of the dashboard.
//
// A job card opens according to the single `JOB_CARDS_EXPANDED_BY_DEFAULT`
// constant in `src/static/index.html`, which is `true`: landing on the dashboard
// shows every job with its projects already visible, so nothing is hidden behind
// a click the user has to discover.
//
// What is persisted is not the expanded jobs but the ones the user toggled *away*
// from that default — with the default expanded, that means the collapsed ones.
// The page reloads itself every 30s, so without that a card the user closed would
// spring back open on its own.
//
// The tests below pin the three properties that relies on: cards start expanded,
// an explicit collapse survives a reload, and the bulk controls only touch the
// jobs the current stat filter or search box leaves visible.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import {
  buildMultiJobDashboard,
  buildDashboardWithTwoFailingJobs,
} from "../fixtures/renovateJobsFixture.mjs";

test.describe("job card expansion", () => {
  test("opens expanded so every job's projects are visible up front", async ({ dashboard }) => {
    const renovateJobs = buildMultiJobDashboard({ jobCount: 5, projectsPerJob: 12 });

    await dashboard.open(renovateJobs);

    // All 5 x 12 projects render without the user touching anything.
    await expect(dashboard.projectRows).toHaveCount(60);
    await expect(dashboard.projectCell("acme/service-1-1")).toBeVisible();

    // Nothing was persisted, so this is the default rather than a remembered state.
    expect(await dashboard.readStoredToggledJobCardKeys()).toEqual([]);
    for (const renovateJob of renovateJobs) {
      expect(await dashboard.isExpanded(renovateJob.name)).toBe(true);
    }
  });

  test("remembers which jobs the user collapsed across a reload", async ({ dashboard }) => {
    const renovateJobs = buildMultiJobDashboard({ jobCount: 5, projectsPerJob: 12 });

    await dashboard.open(renovateJobs);
    await dashboard.toggle("team-02");

    // A collapsed card unmounts its rows entirely.
    await expect(dashboard.projectCell("acme/service-2-1")).toHaveCount(0);
    expect(await dashboard.readStoredToggledJobCardKeys()).toEqual(["renovate/team-02"]);

    await dashboard.reload();

    expect(await dashboard.isExpanded("team-02")).toBe(false);
    await expect(dashboard.projectCell("acme/service-2-1")).toHaveCount(0);

    // Only the explicitly collapsed job comes back closed.
    expect(await dashboard.isExpanded("team-01")).toBe(true);
    expect(await dashboard.isExpanded("team-03")).toBe(true);
  });

  test("expand all and collapse all only touch the filtered jobs", async ({ dashboard }) => {
    const renovateJobs = buildDashboardWithTwoFailingJobs();

    // ?filter=failed hides job-all-green: it has no failing project.
    await dashboard.open(renovateJobs, { statFilter: "failed", expectedJobCount: 2 });
    await expect(dashboard.jobHeading("job-all-green")).toHaveCount(0);

    // Both visible cards are already open, so there is nothing left to expand.
    await expect(dashboard.expandAllButton).toBeDisabled();

    await dashboard.collapseAllButton.click();
    expect(await dashboard.isExpanded("job-failing-a")).toBe(false);
    expect(await dashboard.isExpanded("job-failing-b")).toBe(false);
    await expect(dashboard.collapseAllButton).toBeDisabled();
    expect(await dashboard.readStoredToggledJobCardKeys()).toEqual([
      "renovate/job-failing-a",
      "renovate/job-failing-b",
    ]);

    await dashboard.clearStatFilterButton.click();
    await expect(dashboard.jobHeadings).toHaveCount(3);

    // The job that was filtered out was never collapsed behind the user's back.
    expect(await dashboard.isExpanded("job-all-green")).toBe(true);

    // Expand all now sees all three, so it clears the two that are closed.
    await dashboard.expandAllButton.click();
    expect(await dashboard.isExpanded("job-failing-a")).toBe(true);
    expect(await dashboard.isExpanded("job-failing-b")).toBe(true);
    await expect(dashboard.expandAllButton).toBeDisabled();
    expect(await dashboard.readStoredToggledJobCardKeys()).toEqual([]);
  });

  test("collapse all follows the search box, not the full job list", async ({ dashboard }) => {
    const renovateJobs = buildDashboardWithTwoFailingJobs();

    // The search box narrows to projects matching "broken", which only the two
    // job-failing-* jobs have.
    await dashboard.open(renovateJobs, { search: "broken", expectedJobCount: 2 });
    await expect(dashboard.jobHeading("job-all-green")).toHaveCount(0);

    await dashboard.collapseAllButton.click();
    await expect(dashboard.collapseAllButton).toBeDisabled();

    // Reopening without the search term brings the third job back untouched.
    await dashboard.open(renovateJobs);
    expect(await dashboard.isExpanded("job-failing-a")).toBe(false);
    expect(await dashboard.isExpanded("job-all-green")).toBe(true);
    expect(await dashboard.readStoredToggledJobCardKeys()).toEqual([
      "renovate/job-failing-a",
      "renovate/job-failing-b",
    ]);
  });
});
