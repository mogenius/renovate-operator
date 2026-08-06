// Covers the dashboard toolbar staying reachable while the job list is scrolled.
//
// With a realistic number of projects the page is many viewports tall. The stat
// badges, the search box and the bulk expand/collapse controls therefore live in
// their own bar pinned to the top of the viewport, while the brand strip above it
// scrolls away like ordinary content — so filtering never costs a trip back to the
// top of the page.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import {
  buildMultiJobDashboard,
  buildDashboardWithTwoFailingJobs,
} from "../fixtures/renovateJobsFixture.mjs";

// Tall enough that the bottom of the page is far outside the 900px viewport.
const A_LONG_DASHBOARD = () => buildMultiJobDashboard({ jobCount: 6, projectsPerJob: 20 });

test.describe("sticky dashboard toolbar", () => {
  test("keeps the filters and the search box on screen while the brand strip scrolls away", async ({
    dashboard,
  }) => {
    await dashboard.open(A_LONG_DASHBOARD());

    // Before scrolling both are on screen, the toolbar directly below the brand strip.
    await expect(dashboard.brandLogo).toBeInViewport();
    expect(await dashboard.distanceFromViewportTop(dashboard.toolbar)).toBeGreaterThan(0);

    await dashboard.scrollToBottom();

    // The header did its job and left; the toolbar stayed, pinned to the very top.
    await expect(dashboard.brandLogo).not.toBeInViewport();
    expect(await dashboard.distanceFromViewportTop(dashboard.toolbar)).toBe(0);
    await expect(dashboard.searchInput).toBeInViewport();
    await expect(dashboard.statBadge("Failed")).toBeInViewport();
    await expect(dashboard.expandAllButton).toBeInViewport();
  });

  test("filters and searches from the bottom of the page without scrolling back up", async ({
    dashboard,
  }) => {
    const renovateJobs = A_LONG_DASHBOARD();

    await dashboard.open(renovateJobs);
    await dashboard.scrollToBottom();

    // Both controls are usable where the user already is.
    await dashboard.searchInput.fill("service-1-1");
    await expect(dashboard.jobHeadings).toHaveCount(1);
    await expect(dashboard.jobHeading("team-01")).toBeVisible();

    await dashboard.searchInput.fill("");
    await expect(dashboard.jobHeadings).toHaveCount(renovateJobs.length);

    await dashboard.collapseAllButton.click();
    await expect(dashboard.projectRows).toHaveCount(0);

    // Collapsing shrank the page below one viewport, so the toolbar is no longer
    // holding itself in place — it has to be visible either way.
    await expect(dashboard.toolbar).toBeInViewport();
  });

  // Pinning the toolbar put it in the same stack as the execution-options popover
  // and the full-page overlay that dismisses it. The overlay has to stay on top of
  // the toolbar: cards < toolbar < overlay < menu. With the toolbar above it, the
  // first click on a badge would filter the list *and* leave the popover open,
  // hanging over cards it no longer belongs to.
  test("a click on the filters dismisses an open execution-options popover before filtering", async ({
    dashboard,
  }) => {
    const renovateJobs = buildDashboardWithTwoFailingJobs();

    await dashboard.open(renovateJobs);
    await dashboard.openExecutionOptions("job-failing-a");

    await dashboard.clickTopmostElementAt(dashboard.statBadge("Failed"));

    // The overlay took the click: the popover closed and nothing was filtered.
    await expect(dashboard.executionOptionsPopover("job-failing-a")).toBeHidden();
    await expect(dashboard.statBadge("Failed")).toHaveAttribute("aria-pressed", "false");
    await expect(dashboard.jobHeadings).toHaveCount(renovateJobs.length);

    // With the popover gone the badge is a badge again.
    await dashboard.statBadge("Failed").click();
    await expect(dashboard.statBadge("Failed")).toHaveAttribute("aria-pressed", "true");
    await expect(dashboard.jobHeadings).toHaveCount(2);
  });
});
