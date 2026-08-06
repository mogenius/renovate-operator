// Covers what the logs page does with a stream once it has arrived: the header
// counters, the level filter and the search box.
//
// The page is the only place a user can see why a project failed, and a Renovate
// run logs thousands of lines, so narrowing that list is the whole job. The level
// selection is persisted because it is a preference, not a per-visit choice — the
// user who only ever wants to see errors should not have to say so on every visit.

import { test, expect } from "../fixtures/logsFixture.mjs";
import {
  DETAIL_ONLY_SEARCH_TERM,
  MESSAGE_SEARCH_TERM,
  MESSAGE_SEARCH_TERM_ENTRY,
  buildRenovateRunLog,
  countEntriesWithLevel,
} from "../fixtures/renovateLogsFixture.mjs";

test.describe("logs page", () => {
  test("renders the streamed entries and counts errors and warnings in the header", async ({
    logs,
  }) => {
    const entries = buildRenovateRunLog();

    await logs.open(entries);

    await expect(logs.row("Repository started")).toBeVisible();
    await expect(logs.row("Repository finished")).toBeVisible();

    // The counters follow the page's own thresholds: everything from level 50 up
    // is an error, so the FATAL entry is counted as one too.
    expect(await logs.statValue("Errors")).toBe(
      countEntriesWithLevel(entries, "ERROR") + countEntriesWithLevel(entries, "FATAL"),
    );
    expect(await logs.statValue("Warnings")).toBe(countEntriesWithLevel(entries, "WARN"));

    // Every level of the run made it onto the page.
    await expect(logs.rowsAtLevel("DEBUG")).toHaveCount(countEntriesWithLevel(entries, "DEBUG"));
    await expect(logs.rowsAtLevel("ERROR")).toHaveCount(countEntriesWithLevel(entries, "ERROR"));
    await expect(logs.rowsAtLevel("FATAL")).toHaveCount(countEntriesWithLevel(entries, "FATAL"));
  });

  test("the level filter narrows the list and is remembered on the next visit", async ({
    logs,
  }) => {
    const entries = buildRenovateRunLog();

    await logs.open(entries);
    await logs.toggleLevels(["DEBUG", "INFO"]);

    await expect(logs.rowsAtLevel("DEBUG")).toHaveCount(0);
    await expect(logs.rowsAtLevel("INFO")).toHaveCount(0);
    await expect(logs.rowsAtLevel("ERROR")).toHaveCount(countEntriesWithLevel(entries, "ERROR"));
    expect(await logs.readStoredLevels()).toBe("FATAL,ERROR,WARN");

    await logs.reload();

    // The selection survives, so a user who only wants to see failures keeps that
    // view across visits instead of re-picking it every time.
    await expect(logs.rowsAtLevel("DEBUG")).toHaveCount(0);
    await expect(logs.rowsAtLevel("ERROR")).toHaveCount(countEntriesWithLevel(entries, "ERROR"));
  });

  test("an explicit ?levels= overrides the remembered selection", async ({ logs }) => {
    const entries = buildRenovateRunLog();

    await logs.open(entries);
    await logs.toggleLevels(["ERROR"]);
    expect(await logs.readStoredLevels()).toBe("FATAL,WARN,INFO,DEBUG");

    // A link that asks for errors gets errors, whatever the last visit settled on.
    await logs.open(entries, { levels: "ERROR" });

    await expect(logs.rowsAtLevel("ERROR")).toHaveCount(countEntriesWithLevel(entries, "ERROR"));
    await expect(logs.rowsAtLevel("DEBUG")).toHaveCount(0);
    await expect(logs.rowsAtLevel("WARN")).toHaveCount(0);
  });

  test("search keeps the matching entries and highlights what matched", async ({ logs }) => {
    const entries = buildRenovateRunLog();

    await logs.open(entries);
    await logs.searchInput.fill(MESSAGE_SEARCH_TERM);

    await expect(logs.row(MESSAGE_SEARCH_TERM_ENTRY)).toBeVisible();
    await expect(logs.highlightsIn(MESSAGE_SEARCH_TERM_ENTRY)).toHaveCount(1);
    await expect(logs.row("Repository started")).toHaveCount(0);

    await logs.searchInput.fill("");
    await expect(logs.row("Repository started")).toBeVisible();
  });

  test("a match below the message opens the row that carries it", async ({ logs }) => {
    const entries = buildRenovateRunLog();

    await logs.open(entries);

    const rowWithTheMatch = "Resolving package registries";
    await expect(logs.expandedDetail(rowWithTheMatch)).toHaveCount(0);

    await logs.searchInput.fill(DETAIL_ONLY_SEARCH_TERM);

    // The message says nothing about the match, so the row shows the entry it
    // matched on rather than leaving the user to click every hit.
    await expect(logs.row(rowWithTheMatch)).toBeVisible();
    await expect(logs.expandedDetail(rowWithTheMatch)).toBeVisible();
    await expect(logs.expandedDetail(rowWithTheMatch)).toContainText(DETAIL_ONLY_SEARCH_TERM);
    await expect(logs.row("Repository started")).toHaveCount(0);
  });

  test("says so when nothing matches instead of showing an empty list", async ({ logs }) => {
    await logs.open(buildRenovateRunLog());

    await logs.searchInput.fill("a-term-no-entry-carries");

    await expect(
      logs.page.getByText('No log entries match "a-term-no-entry-carries".'),
    ).toBeVisible();
  });
});
