// Covers how a job card copes with a job holding many projects.
//
// A RenovateJob can discover thousands of repositories. The card used to render
// every one of them, twice (the table and the narrow-screen cards, one hidden by
// CSS), with a React root on <body> per row for its tooltip, and it rebuilt every
// row from scratch on each re-render, the 30s refresh included. Expanding such a
// job froze the browser.
//
// The tests below pin what replaced that: a long list only renders the projects
// near the viewport while the page keeps the height of the whole list, keyboard
// navigation and Trigger All still reach the projects not rendered, rows survive
// a re-render instead of being rebuilt, the columns hold still while scrolling,
// and only one layout is in the DOM.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import {
  buildPRActivity,
  buildProject,
  buildRenovateJob,
  PROJECT_STATUSES,
} from "../fixtures/renovateJobsFixture.mjs";

// Must match VIRTUALIZE_ABOVE in src/static/index.html.
const VIRTUALIZE_ABOVE = 200;

const JOB_NAME = "big-job";

// Every project completed and zero-padded, so the card's default sort (status,
// then name) lists them in numeric order.
const projectName = (n) => `acme/repo-${String(n).padStart(4, "0")}`;

function buildLargeJob(
  projectCount,
  { status = () => PROJECT_STATUSES.completed, prActivity = () => null } = {},
) {
  return [
    buildRenovateJob({
      name: JOB_NAME,
      projects: Array.from({ length: projectCount }, (_, i) =>
        buildProject({ name: projectName(i + 1), status: status(i + 1), prActivity: prActivity(i + 1) }),
      ),
    }),
  ];
}

/** The project rows actually in the DOM, spacers excluded. */
const renderedRows = (page) => page.locator("tr[data-project-row]");

const scrollTo = (page, fraction) =>
  page.evaluate((f) => window.scrollTo(0, f * document.documentElement.scrollHeight), fraction);

test.describe("large job rendering", () => {
  test("renders only the projects near the viewport", async ({ dashboard }) => {
    const page = dashboard.page;
    await dashboard.open(buildLargeJob(2000));

    const atTop = await renderedRows(page).count();
    expect(atTop).toBeGreaterThan(5);
    expect(atTop).toBeLessThan(100);
    await expect(dashboard.projectCell(projectName(1))).toBeInViewport();
    await expect(dashboard.projectCell(projectName(2000))).toHaveCount(0);

    // The page is as tall as the whole list, so the end of it can be scrolled to.
    await scrollTo(page, 1);
    await expect(dashboard.projectCell(projectName(2000))).toBeInViewport();
    await expect(dashboard.projectCell(projectName(1))).toHaveCount(0);
    expect(await renderedRows(page).count()).toBeLessThan(100);
  });

  test("leaves no gap in the viewport wherever the page is scrolled", async ({ dashboard }) => {
    const page = dashboard.page;
    await dashboard.open(buildLargeJob(2000));

    // Below the sticky toolbar, every probed point has to land on a project row
    // rather than on a spacer standing in for rows not rendered.
    const probe = () =>
      page.evaluate(() =>
        [250, 500, 850].map((y) => !!document.elementFromPoint(640, y)?.closest("tr[data-project-row]")),
      );
    for (const fraction of [0.25, 0.5, 0.75]) {
      await scrollTo(page, fraction);
      await expect.poll(probe).toEqual([true, true, true]);
    }
  });

  test("renders a short list whole", async ({ dashboard }) => {
    await dashboard.open(buildLargeJob(VIRTUALIZE_ABOVE));

    await expect(dashboard.projectRows).toHaveCount(VIRTUALIZE_ABOVE);
    await expect(dashboard.projectCell(projectName(VIRTUALIZE_ABOVE))).toHaveCount(1);
  });

  test("Trigger All covers the projects that are not rendered", async ({ dashboard }) => {
    // The search keeps every project but makes the list a filtered one, so the
    // page has to name the projects in the request rather than send "all".
    await dashboard.open(buildLargeJob(2000), { search: "acme/repo-" });
    await expect(dashboard.projectCell(projectName(2000))).toHaveCount(0);

    await expect(dashboard.triggerAllButton(JOB_NAME)).toHaveText("Trigger 2000");
    await dashboard.triggerAll(JOB_NAME);

    expect(dashboard.triggerAllRequests).toHaveLength(1);
    expect(dashboard.triggerAllRequests[0].projects).toHaveLength(2000);
    expect(dashboard.triggerAllRequests[0].projects).toContain(projectName(2000));
  });

  test("keyboard navigation brings a project that is not rendered into view", async ({ dashboard }) => {
    const page = dashboard.page;
    await dashboard.open(buildLargeJob(2000));
    const row = (n) => page.locator(`tr[data-project-row="${projectName(n)}"]`);

    await page.keyboard.press("j");
    await expect(dashboard.jobCard(JOB_NAME)).toHaveClass(/border-primary/);
    await page.keyboard.press("ArrowDown");
    await expect(row(1)).toHaveClass(/ring-primary/);

    // Scrolled away, the focused project's neighbours are no longer rendered.
    await scrollTo(page, 1);
    await expect(row(2)).toHaveCount(0);
    await page.keyboard.press("ArrowDown");
    await expect(row(2)).toHaveClass(/ring-primary/);
    await expect(row(2)).toBeInViewport();

    // Stepping on past the rows rendered at the time keeps up as well.
    for (let i = 0; i < 40; i++) {
      await page.keyboard.press("ArrowDown");
    }
    await expect(row(42)).toHaveClass(/ring-primary/);
    await expect(row(42)).toBeInViewport();
  });

  test("keeps the columns where they are while scrolling", async ({ dashboard }) => {
    const page = dashboard.page;
    // Only the second half carries PR activity, five badges wide: a table sized
    // to its rendered rows would widen the Activity column on the way down.
    await dashboard.open(
      buildLargeJob(2000, {
        prActivity: (n) =>
          n > 1000
            ? buildPRActivity({ automerged: 3, created: 12, updated: 9, needsApproval: 2, unchanged: 21 })
            : null,
      }),
    );
    const columnWidths = () =>
      page.evaluate(() =>
        [...document.querySelectorAll("thead th")].map((th) => Math.round(th.getBoundingClientRect().width)),
      );

    const atTop = await columnWidths();
    await scrollTo(page, 0.75);
    const rowMidViewportHasBadges = () =>
      page.evaluate(() =>
        !!document.elementFromPoint(640, 500)?.closest("tr[data-project-row]")?.textContent.includes("21 unchanged"),
      );
    await expect.poll(rowMidViewportHasBadges).toBe(true);
    expect(await columnWidths()).toEqual(atTop);
  });

  test("keeps rows mounted across a re-render and a refresh", async ({ dashboard }) => {
    const page = dashboard.page;
    await dashboard.open(buildLargeJob(300));

    // Tag two row elements: a rebuilt row would come back without the tag. Rows
    // are found through their name cell, so this also holds against a page that
    // predates data-project-row.
    const rowMarkers = (tag) =>
      page.evaluate(({ names, tag }) =>
        names.map((name) => {
          const row = [...document.querySelectorAll('table [data-testid="project-name"]')]
            .find((cell) => cell.textContent === name)
            ?.closest("tr");
          if (row && tag) row.__marker = name;
          return row?.__marker ?? null;
        }),
      { names: [projectName(1), projectName(2)], tag });
    await rowMarkers(true);
    const taggedRows = () => rowMarkers(false);

    // Opening the card's menu re-renders the card.
    await dashboard.openExecutionOptions(JOB_NAME);
    await dashboard.closeExecutionOptions(JOB_NAME);
    expect(await taggedRows()).toEqual([projectName(1), projectName(2)]);

    // A refresh replaces every project object; one of them also changes.
    await page.route("**/api/v1/renovatejobs", (route) =>
      route.fulfill({
        json: buildLargeJob(300, {
          status: (n) => (n === 2 ? PROJECT_STATUSES.failed : PROJECT_STATUSES.completed),
        }),
      }),
    );
    await page.keyboard.press("r");
    const changedRow = page.locator("tr", { has: dashboard.projectCell(projectName(2)) });
    await expect(changedRow).toContainText("failed");
    expect(await taggedRows()).toEqual([projectName(1), projectName(2)]);
  });

  test("renders a single layout for the viewport", async ({ dashboard }) => {
    await dashboard.open(buildLargeJob(5));
    const logsLinks = dashboard.page.getByRole("link", { name: `View logs for ${projectName(1)}` });

    await expect(dashboard.page.locator("table")).toHaveCount(1);
    await expect(logsLinks).toHaveCount(1);

    // Below the lg breakpoint the card swaps the table for stacked cards.
    await dashboard.page.setViewportSize({ width: 800, height: 900 });
    await expect(dashboard.page.locator("table")).toHaveCount(0);
    await expect(dashboard.page.locator(`div[data-project-row="${projectName(1)}"]`)).toBeVisible();
    await expect(logsLinks).toHaveCount(1);
  });

  test("shows the last run duration without a portal per row", async ({ dashboard }) => {
    await dashboard.open(buildLargeJob(300));
    const page = dashboard.page;

    // The tooltip used to mount a React root on <body> for every row.
    expect(await page.evaluate(() => document.body.children.length)).toBeLessThan(10);

    await dashboard.projectCell(projectName(3)).hover();
    const tooltip = page.getByText("Last Run Duration");
    await expect(tooltip).toBeVisible();
    await expect(tooltip.locator("..")).toContainText("42s");

    // Cells holding controls keep it out of the way.
    await page.getByRole("link", { name: `View logs for ${projectName(3)}` }).hover();
    await expect(tooltip).toHaveCount(0);
  });
});
