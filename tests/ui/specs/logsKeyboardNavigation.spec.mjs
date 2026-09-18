// Covers the keyboard shortcuts added to the logs page: j/k to move row focus,
// Enter to expand a row's detail, / to reach the search box, Esc to clear it,
// and ? for the shortcuts modal.
//
// A log row is "focused" when its outer div carries ring-primary/60. The outer
// div is the ancestor with cursor-pointer — the text span that getByText matches
// lives one level down, so tests climb up with an xpath before asserting the
// class.

import { test, expect } from "../fixtures/logsFixture.mjs";
import { buildRenovateRunLog } from "../fixtures/renovateLogsFixture.mjs";

// Climbs from the matched text node to the LogRow's outer div (cursor-pointer
// is the discriminating class shared by every row and nothing else nearby).
function rowContainer(page, message) {
  return page
    .getByText(message, { exact: true })
    .locator('xpath=ancestor-or-self::div[contains(@class,"cursor-pointer")][1]');
}

test.describe("keyboard navigation — logs", () => {
  test("? opens the shortcuts modal and ? again closes it", async ({ logs }) => {
    await logs.open(buildRenovateRunLog());

    await expect(logs.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toHaveCount(0);

    await logs.page.keyboard.press("?");
    await expect(logs.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();

    await logs.page.keyboard.press("?");
    await expect(logs.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toHaveCount(0);
  });

  test("Escape closes the shortcuts modal", async ({ logs }) => {
    await logs.open(buildRenovateRunLog());
    await logs.page.keyboard.press("?");
    await expect(logs.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();

    await logs.page.keyboard.press("Escape");
    await expect(logs.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toHaveCount(0);
  });

  test("the ? toolbar button opens the modal", async ({ logs }) => {
    await logs.open(buildRenovateRunLog());
    await logs.page.getByRole("button", { name: "Keyboard shortcuts" }).click();
    await expect(logs.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();
  });

  test("j moves focus to the first row, then to the next", async ({ logs }) => {
    await logs.open(buildRenovateRunLog());
    await expect(logs.page.locator(".ring-primary\\/60")).toHaveCount(0);

    await logs.page.keyboard.press("j");
    await expect(rowContainer(logs.page, "Repository started")).toHaveClass(/ring-primary/);

    await logs.page.keyboard.press("j");
    await expect(rowContainer(logs.page, "Resolving package registries")).toHaveClass(/ring-primary/);
    await expect(rowContainer(logs.page, "Repository started")).not.toHaveClass(/ring-primary/);
  });

  test("k moves focus back to the previous row", async ({ logs }) => {
    await logs.open(buildRenovateRunLog());

    await logs.page.keyboard.press("j");
    await logs.page.keyboard.press("j");
    await expect(rowContainer(logs.page, "Resolving package registries")).toHaveClass(/ring-primary/);

    await logs.page.keyboard.press("k");
    await expect(rowContainer(logs.page, "Repository started")).toHaveClass(/ring-primary/);
    await expect(rowContainer(logs.page, "Resolving package registries")).not.toHaveClass(/ring-primary/);
  });

  test("Enter expands the detail of the focused row", async ({ logs }) => {
    await logs.open(buildRenovateRunLog());

    await expect(logs.expandedDetail("Repository started")).toHaveCount(0);

    await logs.page.keyboard.press("j");
    await expect(rowContainer(logs.page, "Repository started")).toHaveClass(/ring-primary/);

    await logs.page.keyboard.press("Enter");
    await expect(logs.expandedDetail("Repository started")).toBeVisible();
  });

  test("/ focuses the search input", async ({ logs }) => {
    await logs.open(buildRenovateRunLog());
    await logs.page.keyboard.press("/");
    await expect(logs.searchInput).toBeFocused();
  });

  test("Esc clears the search query while the input is focused", async ({ logs }) => {
    await logs.open(buildRenovateRunLog());

    await logs.searchInput.fill("Repository");
    await expect(logs.searchInput).toHaveValue("Repository");

    await logs.page.keyboard.press("Escape");
    await expect(logs.searchInput).toHaveValue("");
  });

  test("row shortcuts are suppressed while the search input is active", async ({ logs }) => {
    await logs.open(buildRenovateRunLog());

    await logs.searchInput.focus();
    await logs.page.keyboard.press("j");

    // j typed into the input — no row gained focus.
    await expect(logs.searchInput).toHaveValue("j");
    await expect(logs.page.locator(".ring-primary\\/60")).toHaveCount(0);
  });

  test("row shortcuts are suppressed while the shortcuts modal is open", async ({ logs }) => {
    await logs.open(buildRenovateRunLog());

    await logs.page.keyboard.press("?");
    await expect(logs.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();

    await logs.page.keyboard.press("j");

    // Modal still open; no row focused.
    await expect(logs.page.locator(".ring-primary\\/60")).toHaveCount(0);
    await expect(logs.page.getByRole("dialog", { name: "Keyboard shortcuts" })).toBeVisible();
  });
});
