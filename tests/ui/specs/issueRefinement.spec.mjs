// Covers the facets that narrow the "With Issues" stat filter (mogenius/renovate-operator#804).
//
// "With Issues" selects every project whose last run logged a warning or an error. Once it
// is selected, a bar narrows it by severity (errors only), by category (what the operator's
// log parser filed the issues under, LogIssues.categoryCounts) and by repository state
// (renovateResultStatus). Facets combine with AND, values within a facet with OR, and the
// choice lives in the URL so a view can be shared.
//
// Every test opens buildDashboardWithEveryProjectState, one project per catalogue entry,
// named acme/<variant key>, so a test can name the projects a facet should leave.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import { buildDashboardWithEveryProjectState } from "../fixtures/renovateJobsFixture.mjs";

const JOB = "job-all-states";

/** Opens the dashboard with "With Issues" selected and the given refinement in the URL. */
async function openWithIssues(dashboard, query = {}) {
  await dashboard.open(buildDashboardWithEveryProjectState({ jobName: JOB }), { statFilter: "withIssues", query });
  await expect(dashboard.issueRefinement).toBeVisible();
}

async function expectProjects(dashboard, variantKeys) {
  await expect
    .poll(async () => [...(await dashboard.projectNamesInJob(JOB))].sort())
    .toEqual(variantKeys.map((key) => `acme/${key}`).sort());
}

const PROJECTS_WITH_LOOKUP_FAILURES = [
  "failed-with-errors",
  "completed-with-warnings",
  "completed-with-truncated-detail",
  "onboarding-with-lookup-failures",
];

test.describe("With Issues refinement", () => {
  test("is only offered while With Issues is selected", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithEveryProjectState({ jobName: JOB }));
    await expect(dashboard.issueRefinement).toHaveCount(0);

    await dashboard.statBadge("With Issues").click();
    await expect(dashboard.issueRefinement).toBeVisible();
    await expect(dashboard.issueRefinementChip("Lookup failures")).toBeVisible();
  });

  test("a category keeps the projects with issues in it", async ({ dashboard }) => {
    await openWithIssues(dashboard);

    await dashboard.issueRefinementChip("Lookup failures").click();

    await expectProjects(dashboard, PROJECTS_WITH_LOOKUP_FAILURES);
    await expect(dashboard.issueRefinementChip("Lookup failures")).toHaveAttribute("aria-pressed", "true");
    expect(dashboard.currentQuery().get("issues")).toBe("lookup");
  });

  test("categories combine with OR", async ({ dashboard }) => {
    await openWithIssues(dashboard);

    await dashboard.issueRefinementChip("Lookup failures").click();
    await dashboard.issueRefinementChip("Config").click();

    await expectProjects(dashboard, [...PROJECTS_WITH_LOOKUP_FAILURES, "completed-with-config-warnings"]);
    expect(dashboard.currentQuery().get("issues")).toBe("lookup,config");
  });

  test("facets combine with AND, and each chip counts what it would leave", async ({ dashboard }) => {
    await openWithIssues(dashboard);
    expect(await dashboard.issueRefinementChipCount("Lookup failures")).toBe(PROJECTS_WITH_LOOKUP_FAILURES.length);

    await dashboard.issueRefinementChip("Errors only").click();

    // the lookup projects that only warned drop out, and the chip says so before it is clicked
    await expect.poll(() => dashboard.issueRefinementChipCount("Lookup failures")).toBe(2);
    await dashboard.issueRefinementChip("Lookup failures").click();
    await expectProjects(dashboard, ["failed-with-errors", "completed-with-truncated-detail"]);
    expect(dashboard.currentQuery().get("severity")).toBe("error");
  });

  test("a repository state separates onboarding repositories from onboarded ones", async ({ dashboard }) => {
    await openWithIssues(dashboard);

    await dashboard.issueRefinementChip("Onboarding").click();

    await expectProjects(dashboard, ["onboarding-with-lookup-failures"]);
    expect(dashboard.currentQuery().get("repo")).toBe("onboarding");
  });

  test("a chip that would leave nothing cannot be picked", async ({ dashboard }) => {
    await openWithIssues(dashboard);

    // no project in the catalogue warns while Renovate is disabled for it
    await expect(dashboard.issueRefinementChip("Disabled")).toBeDisabled();
    expect(await dashboard.issueRefinementChipCount("Disabled")).toBe(0);
  });

  test("files issues the operator did not categorise under Other", async ({ dashboard }) => {
    await openWithIssues(dashboard);

    await dashboard.issueRefinementChip("Other").click();

    await expectProjects(dashboard, [
      "completed-with-errors-and-prs", // categorised as other by the operator
      "completed-with-counts-but-no-messages", // counted, no message
      "completed-with-issues-before-categories", // written before categories existed
    ]);
  });

  test("restores the refinement from the URL and ignores values it does not know", async ({ dashboard }) => {
    await openWithIssues(dashboard, { issues: "lookup,nonsense", severity: "error", repo: "bogus" });

    await expect(dashboard.issueRefinementChip("Lookup failures")).toHaveAttribute("aria-pressed", "true");
    await expect(dashboard.issueRefinementChip("Errors only")).toHaveAttribute("aria-pressed", "true");
    await expectProjects(dashboard, ["failed-with-errors", "completed-with-truncated-detail"]);
    // the unknown values are dropped from the URL rather than carried along
    expect(dashboard.currentQuery().get("issues")).toBe("lookup");
    expect(dashboard.currentQuery().has("repo")).toBe(false);
  });

  test("Reset clears the facets but keeps With Issues", async ({ dashboard }) => {
    await openWithIssues(dashboard, { issues: "deprecation" });
    await expectProjects(dashboard, ["completed-with-deprecation"]);

    await dashboard.issueRefinement.getByRole("button", { name: "Reset" }).click();

    await expect(dashboard.issueRefinementChip("Deprecations")).toHaveAttribute("aria-pressed", "false");
    expect(dashboard.currentQuery().has("issues")).toBe(false);
    expect(dashboard.currentQuery().get("filter")).toBe("withIssues");
  });

  test("leaving With Issues drops the refinement", async ({ dashboard }) => {
    await openWithIssues(dashboard, { issues: "lookup", severity: "error" });

    await dashboard.statBadge("With Issues").click();

    await expect(dashboard.issueRefinement).toHaveCount(0);
    expect(dashboard.currentQuery().has("issues")).toBe(false);
    expect(dashboard.currentQuery().has("severity")).toBe(false);

    // selecting it again starts unrefined
    await dashboard.statBadge("With Issues").click();
    await expect(dashboard.issueRefinementChip("Lookup failures")).toHaveAttribute("aria-pressed", "false");
  });

  test("Escape clears With Issues and its refinement", async ({ dashboard }) => {
    await openWithIssues(dashboard, { issues: "platform" });

    await dashboard.page.locator("body").click({ position: { x: 5, y: 5 } });
    await dashboard.page.keyboard.press("Escape");

    await expect(dashboard.issueRefinement).toHaveCount(0);
    expect(dashboard.currentQuery().has("issues")).toBe(false);
  });

  test("the banner says what the list is narrowed to", async ({ dashboard }) => {
    await openWithIssues(dashboard, { issues: "platform", severity: "error" });

    await expect(dashboard.page.getByRole("status")).toContainText("(errors only · Platform & access)");
  });

  test("rows show their categories, and the detail shows each message's category and fatal level", async ({ dashboard }) => {
    await openWithIssues(dashboard, { issues: "platform", severity: "error" });

    const row = dashboard.page.locator("table tbody tr").filter({ hasText: "acme/failed-with-fatal-auth-error" });
    await expect(row.getByTestId("issue-categories")).toHaveText("Platform");

    await row.getByRole("button", { name: /2 errors/ }).click();
    const detail = dashboard.page.locator("#issue-details-" + "acme-failed-with-fatal-auth-error");
    const fatalIssue = detail.getByRole("link").filter({ hasText: "Authentication failure" });
    await expect(fatalIssue).toContainText("fatal");
    await expect(fatalIssue).toContainText("Platform & access");
  });

  test("a message stored before categories existed shows no category of its own", async ({ dashboard }) => {
    await openWithIssues(dashboard, { issues: "other" });

    const row = dashboard.page.locator("table tbody tr").filter({ hasText: "acme/completed-with-issues-before-categories" });
    await expect(row.getByTestId("issue-categories")).toHaveText("Other");
    await row.getByRole("button", { name: /1 warn/ }).click();
    const issue = dashboard.page.getByRole("link").filter({ hasText: "Package lookup failures" });
    await expect(issue).toContainText("warn");
    await expect(issue).not.toContainText("Other");
  });
});
