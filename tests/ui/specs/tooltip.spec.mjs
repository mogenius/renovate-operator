import { test, expect } from "../fixtures/dashboardFixture.mjs";
import { buildDashboardWithEveryProjectState } from "../fixtures/renovateJobsFixture.mjs";

test("tooltip shows only on hover and hides on leave", async ({ dashboard }) => {
  await dashboard.open(buildDashboardWithEveryProjectState());

  const tooltip = dashboard.page.getByRole("tooltip", { name: /all job cards/ });
  await expect(tooltip).toHaveCount(0);

  const toggle = dashboard.page.getByRole("button", { name: /(Expand|Collapse) all job cards/ });
  await toggle.hover();
  await expect(tooltip).toHaveCount(1);
  await expect(tooltip).toBeVisible();

  await dashboard.page.mouse.move(0, 0);
  await expect(tooltip).toHaveCount(0);
});
