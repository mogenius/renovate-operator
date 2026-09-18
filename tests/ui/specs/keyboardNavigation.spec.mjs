// Covers the keyboard shortcuts added to the dashboard: j/k to move card focus,
// Space to toggle expansion, arrow keys to walk projects within a card, e/c to
// expand/collapse all, / to reach the search box, and ? for the shortcuts modal.
//
// A card is "focused" when the dashboard applies border-primary to its outer
// wrapper. A project row is "focused" when its table row carries ring-primary.
// Both classes come from Tailwind utility names so the assertions use regexes
// against the full class string rather than exact class checks.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import { buildMultiJobDashboard } from "../fixtures/renovateJobsFixture.mjs";

const SMALL = () => buildMultiJobDashboard({ jobCount: 3, projectsPerJob: 4 });

test.describe("keyboard navigation — dashboard", () => {
  test("? opens the shortcuts modal and ? again closes it", async ({ dashboard }) => {
    await dashboard.open(SMALL());

    await expect(dashboard.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toHaveCount(0);

    await dashboard.page.keyboard.press("?");
    await expect(dashboard.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();

    await dashboard.page.keyboard.press("?");
    await expect(dashboard.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toHaveCount(0);
  });

  test("Escape closes the shortcuts modal", async ({ dashboard }) => {
    await dashboard.open(SMALL());
    await dashboard.page.keyboard.press("?");
    await expect(dashboard.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();

    await dashboard.page.keyboard.press("Escape");
    await expect(dashboard.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toHaveCount(0);
  });

  test("the ? toolbar button opens the modal", async ({ dashboard }) => {
    await dashboard.open(SMALL());
    await dashboard.page.getByRole("button", { name: "Keyboard shortcuts" }).click();
    await expect(dashboard.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();
  });

  test("j moves focus to the first card, then the next", async ({ dashboard }) => {
    await dashboard.open(SMALL());
    // The "All" stat badge is always selected (ring-primary/30 on a <button>);
    // scope to <div> to check only job cards.
    await expect(dashboard.page.locator("div.ring-primary\\/30")).toHaveCount(0);

    await dashboard.page.keyboard.press("j");
    await expect(dashboard.jobCard("team-01")).toHaveClass(/border-primary/);

    await dashboard.page.keyboard.press("j");
    await expect(dashboard.jobCard("team-02")).toHaveClass(/border-primary/);
    await expect(dashboard.jobCard("team-01")).not.toHaveClass(/border-primary/);
  });

  test("k moves focus back to the previous card", async ({ dashboard }) => {
    await dashboard.open(SMALL());

    await dashboard.page.keyboard.press("j");
    await dashboard.page.keyboard.press("j");
    await expect(dashboard.jobCard("team-02")).toHaveClass(/border-primary/);

    await dashboard.page.keyboard.press("k");
    await expect(dashboard.jobCard("team-01")).toHaveClass(/border-primary/);
    await expect(dashboard.jobCard("team-02")).not.toHaveClass(/border-primary/);
  });

  test("j stops at the last card and k stops at the first", async ({ dashboard }) => {
    await dashboard.open(buildMultiJobDashboard({ jobCount: 2, projectsPerJob: 2 }));

    // Navigate to the last card.
    await dashboard.page.keyboard.press("j");
    await dashboard.page.keyboard.press("j");
    await expect(dashboard.jobCard("team-02")).toHaveClass(/border-primary/);

    // Another j stays at team-02.
    await dashboard.page.keyboard.press("j");
    await expect(dashboard.jobCard("team-02")).toHaveClass(/border-primary/);

    // Back to the first card; another k stays at team-01.
    await dashboard.page.keyboard.press("k");
    await expect(dashboard.jobCard("team-01")).toHaveClass(/border-primary/);
    await dashboard.page.keyboard.press("k");
    await expect(dashboard.jobCard("team-01")).toHaveClass(/border-primary/);
  });

  test("Space toggles expansion on the focused card without touching others", async ({ dashboard }) => {
    await dashboard.open(SMALL());

    // Cards are expanded by default.
    expect(await dashboard.isExpanded("team-01")).toBe(true);

    await dashboard.page.keyboard.press("j");
    await expect(dashboard.jobCard("team-01")).toHaveClass(/border-primary/);

    await dashboard.page.keyboard.press("Space");
    expect(await dashboard.isExpanded("team-01")).toBe(false);
    // Other cards are untouched.
    expect(await dashboard.isExpanded("team-02")).toBe(true);

    await dashboard.page.keyboard.press("Space");
    expect(await dashboard.isExpanded("team-01")).toBe(true);
  });

  test("ArrowDown highlights the first project in the focused card and ArrowUp moves back", async ({ dashboard }) => {
    await dashboard.open(SMALL());

    await dashboard.page.keyboard.press("j");
    await expect(dashboard.jobCard("team-01")).toHaveClass(/border-primary/);

    // Use nth-based row selection: some project variants render a status badge
    // inside the name cell, changing that cell's accessible name and breaking
    // getByRole('cell', { exact: true }) for those rows.
    const cardRows = dashboard.jobCard("team-01").locator("table tbody tr");

    await dashboard.page.keyboard.press("ArrowDown");
    await expect(cardRows.nth(0)).toHaveClass(/ring-primary/);
    await expect(cardRows.nth(1)).not.toHaveClass(/ring-primary/);

    await dashboard.page.keyboard.press("ArrowDown");
    await expect(cardRows.nth(1)).toHaveClass(/ring-primary/);
    await expect(cardRows.nth(0)).not.toHaveClass(/ring-primary/);

    await dashboard.page.keyboard.press("ArrowUp");
    await expect(cardRows.nth(0)).toHaveClass(/ring-primary/);
  });

  test("c collapses all visible cards and e expands them again", async ({ dashboard }) => {
    await dashboard.open(SMALL());

    // Default is expanded; collapse all.
    await dashboard.page.keyboard.press("c");
    expect(await dashboard.isExpanded("team-01")).toBe(false);
    expect(await dashboard.isExpanded("team-02")).toBe(false);
    expect(await dashboard.isExpanded("team-03")).toBe(false);

    await dashboard.page.keyboard.press("e");
    expect(await dashboard.isExpanded("team-01")).toBe(true);
    expect(await dashboard.isExpanded("team-02")).toBe(true);
    expect(await dashboard.isExpanded("team-03")).toBe(true);
  });

  test("/ focuses the search input", async ({ dashboard }) => {
    await dashboard.open(SMALL());
    await dashboard.page.keyboard.press("/");
    await expect(dashboard.searchInput).toBeFocused();
  });

  test("card shortcuts are suppressed while the search input is active", async ({ dashboard }) => {
    await dashboard.open(SMALL());

    await dashboard.searchInput.focus();
    await dashboard.page.keyboard.press("j");

    // j typed into the input — no card gained focus.
    await expect(dashboard.searchInput).toHaveValue("j");
    await expect(dashboard.page.locator("div.ring-primary\\/30")).toHaveCount(0);
  });

  test("card shortcuts are suppressed while the shortcuts modal is open", async ({ dashboard }) => {
    await dashboard.open(SMALL());

    await dashboard.page.keyboard.press("?");
    await expect(dashboard.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();

    await dashboard.page.keyboard.press("j");

    // Modal still open; no card focused.
    await expect(dashboard.page.locator("div.ring-primary\\/30")).toHaveCount(0);
    await expect(dashboard.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();
  });
});
