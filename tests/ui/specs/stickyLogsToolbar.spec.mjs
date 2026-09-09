// Covers the logs toolbar staying reachable while the log is scrolled.
//
// A Renovate run logs thousands of lines, so the page is far taller than the
// dashboard ever gets: by the time a user has scrolled to the error they came for,
// controls left in the document flow are long gone. The counters, the search box
// and the level/copy/download controls therefore live in a bar pinned to the top of
// the viewport, while the brand strip above it scrolls away like ordinary content.

import { test, expect } from "../fixtures/logsFixture.mjs";
import {
  MESSAGE_SEARCH_TERM,
  MESSAGE_SEARCH_TERM_ENTRY,
  buildRenovateRunLog,
  countEntriesWithLevel,
} from "../fixtures/renovateLogsFixture.mjs";

// Tall enough that the bottom of the log is far outside the 900px viewport.
const A_LONG_RUN = () => buildRenovateRunLog({ fillerEntryCount: 120 });

test.describe("sticky logs toolbar", () => {
  test("keeps the search box and the level filter on screen while the brand strip scrolls away", async ({
    logs,
  }) => {
    const entries = A_LONG_RUN();

    await logs.open(entries);
    // A run this long loads already scrolled to its newest line, so the top of
    // the page is somewhere the user has to come back to.
    await logs.scrollToTop();

    // Up here both are on screen, the toolbar directly below the brand strip.
    await expect(logs.brandLogo).toBeInViewport();
    expect(await logs.distanceFromViewportTop(logs.toolbar)).toBeGreaterThan(0);

    await logs.scrollToBottom();

    // The header did its job and left; the toolbar stayed, pinned to the very top.
    await expect(logs.brandLogo).not.toBeInViewport();
    expect(await logs.distanceFromViewportTop(logs.toolbar)).toBe(0);
    await expect(logs.searchInput).toBeInViewport();
    await expect(logs.levelFilterButton).toBeInViewport();
    await expect(logs.copyButton).toBeInViewport();
    await expect(logs.downloadButton).toBeInViewport();

    // The counters are still on screen, which is what makes them worth reading
    // while the stream is still growing.
    expect(await logs.statValue("Errors")).toBe(
      countEntriesWithLevel(entries, "ERROR") + countEntriesWithLevel(entries, "FATAL"),
    );
  });

  test("searches from the bottom of the log without scrolling back up", async ({ logs }) => {
    const entries = A_LONG_RUN();

    await logs.open(entries);
    await logs.scrollToBottom();

    // The box is usable where the user already is, not after a trip to the top.
    await expect(logs.searchInput).toBeInViewport();
    await logs.searchInput.fill(MESSAGE_SEARCH_TERM);

    await expect(logs.row(MESSAGE_SEARCH_TERM_ENTRY)).toBeVisible();
    await expect(logs.rowsAtLevel("DEBUG")).toHaveCount(0);

    // One match makes the page shorter than the viewport, so the toolbar is no
    // longer holding itself in place — it has to be visible either way.
    await expect(logs.toolbar).toBeInViewport();
  });

  test("opens the level filter over the log rows while the bar is pinned", async ({ logs }) => {
    const entries = A_LONG_RUN();

    await logs.open(entries);
    await logs.scrollToBottom();

    // Pinning the bar puts it in the same stack as the rows it now floats over. If
    // the rows won that stack, this click would land on a log row instead of the
    // checkbox and Playwright would refuse it.
    await logs.toggleLevels(["DEBUG"]);

    await expect(logs.rowsAtLevel("DEBUG")).toHaveCount(0);
    await expect(logs.rowsAtLevel("ERROR")).toHaveCount(countEntriesWithLevel(entries, "ERROR"));
  });
});
