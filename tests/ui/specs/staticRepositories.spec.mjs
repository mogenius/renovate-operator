// Covers spec.repositories: a job with a static repository list skips
// autodiscovery, so the card offers no "Run Discovery" button and the config
// tooltip lists the repositories instead of discovery filters.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import { buildProject, buildRenovateJob } from "../fixtures/renovateJobsFixture.mjs";

function buildDashboardWithStaticRepositories() {
  return [
    buildRenovateJob({
      name: "job-static",
      repositories: ["acme/one", "acme/two"],
      projects: [buildProject({ name: "acme/one" }), buildProject({ name: "acme/two" })],
    }),
    buildRenovateJob({
      name: "job-discovery",
      projects: [buildProject({ name: "acme/three" })],
    }),
  ];
}

test.describe("static repositories", () => {
  test("hides Run Discovery and lists the repositories in the config tooltip", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithStaticRepositories());

    await expect(
      dashboard.jobCardHeader("job-static").getByRole("button", { name: "Run discovery for job-static" }),
    ).toHaveCount(0);
    await expect(
      dashboard.jobCardHeader("job-discovery").getByRole("button", { name: "Run discovery for job-discovery" }),
    ).toBeEnabled();

    await dashboard
      .jobCardHeader("job-static")
      .locator('svg[aria-label="Job configuration info"]')
      .hover();

    const tooltip = dashboard.page.getByTestId("job-config-tooltip");
    await expect(tooltip).toBeVisible();
    await expect(tooltip.getByText("Repositories", { exact: true })).toBeVisible();
    await expect(tooltip.getByText("acme/one", { exact: true })).toBeVisible();
    await expect(tooltip.getByText("acme/two", { exact: true })).toBeVisible();
  });
});
