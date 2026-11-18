// Covers the abandoned-dependency badge in the Issues column: Renovate's
// abandonmentThreshold count shows even when the run had no warnings or errors,
// and the (capped) package list is in the badge's tooltip.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import { buildRenovateJob, buildProject, buildLogIssues } from "../fixtures/renovateJobsFixture.mjs";

test("shows the abandoned count and lists the packages in its tooltip", async ({ dashboard }) => {
  const renovateJobs = [buildRenovateJob({
    name: "job-abandoned",
    projects: [
      buildProject({
        name: "acme/legacy",
        logIssues: buildLogIssues({
          abandonedCount: 3,
          abandonedDependencies: ["npm/left-pad", "npm/request"],
        }),
      }),
      buildProject({ name: "acme/clean", logIssues: buildLogIssues({ abandonedCount: 0 }) }),
    ],
  })];

  await dashboard.open(renovateJobs);

  const badge = dashboard.page.locator("table tbody").getByText("3 abandoned");
  await expect(badge).toBeVisible();
  await expect(badge).toHaveAttribute("title", /npm\/left-pad\nnpm\/request\n… and 1 more/);
  await expect(dashboard.page.locator("table tbody").getByText(/abandoned/)).toHaveCount(1);
});
