// Playwright fixture that boots the logs page against a stubbed log stream.
//
// pages/logs.html reads everything it shows from /api/v1/logs, an SSE endpoint, so
// fulfilling that one route with a canned stream drives the real page through any
// state without a cluster. The whole body arrives in one response, which the
// browser's EventSource parses frame by frame exactly as it would a live stream —
// the page has therefore already seen the `done` event by the time a spec starts.

import { test as base, expect } from "@playwright/test";

import { encodeLogStream } from "./renovateLogsFixture.mjs";

const OPERATOR_VERSION = "test-version";
const BASE_PATH = (process.env.BASE_PATH ?? "").trim().replace(/^\/+|\/+$/g, "");
const LOGS_PATH = BASE_PATH === "" ? "/logs" : `/${BASE_PATH}/logs`;

const DEFAULT_TARGET = {
  namespace: "renovate",
  renovate: "team-01",
  project: "acme/service-1-1",
};

export const test = base.extend({
  /** logs.open(entries) stubs the stream and loads the page. */
  logs: async ({ page }, use) => {
    await use(new LogsPage(page));
  },
});

export { expect };

class LogsPage {
  constructor(page) {
    this.page = page;
  }

  async stubApi(entries) {
    await this.page.route("**/api/v1/auth/status", (route) =>
      route.fulfill({ json: { enabled: false } }),
    );
    await this.page.route("**/api/v1/version", (route) =>
      route.fulfill({ json: { version: OPERATOR_VERSION } }),
    );
    // Matched by path rather than by glob: the page appends its query parameters,
    // and the content type is what makes EventSource accept the response.
    await this.page.route(
      (url) => url.pathname.endsWith("/api/v1/logs"),
      (route) =>
        route.fulfill({
          status: 200,
          headers: { "Content-Type": "text/event-stream", "Cache-Control": "no-cache" },
          body: encodeLogStream(entries),
        }),
    );
  }

  /**
   * @param entries the log entries the stubbed stream delivers
   * @param target  the namespace/renovate/project triple the page is opened for;
   *        all three are required by the page, which errors out without them
   * @param levels  value for the ?levels= query parameter
   */
  async open(entries, { target = DEFAULT_TARGET, levels = null } = {}) {
    await this.stubApi(entries);
    const query = new URLSearchParams(target);
    if (levels) query.set("levels", levels);
    await this.page.goto(`${LOGS_PATH}?${query}`);
    // Babel compiles the page's JSX in the browser, so wait for real content
    // rather than a load event.
    await expect(this.streamCompletedIndicator).toBeVisible();
  }

  async reload() {
    await this.page.reload();
    await expect(this.streamCompletedIndicator).toBeVisible();
  }

  /** Shown once the `done` event closed the EventSource. */
  get streamCompletedIndicator() {
    return this.page.getByText("Completed", { exact: true });
  }

  get searchInput() {
    return this.page.getByPlaceholder("Search logs…");
  }

  get levelFilterButton() {
    return this.page.getByRole("button", { name: "Levels" });
  }

  get copyButton() {
    return this.page.getByRole("button", { name: "Copy logs" });
  }

  get downloadButton() {
    return this.page.getByRole("button", { name: "Download" });
  }

  /**
   * The level labels the rendered rows carry, e.g. every "ERROR" row. The level
   * filter repeats those same words, so this only counts rows while the filter
   * dropdown is closed — which is what toggleLevels leaves behind.
   */
  rowsAtLevel(levelLabel) {
    return this.page.getByText(levelLabel, { exact: true });
  }

  /** A row located by its message, which is the part of an entry the page shows. */
  row(message) {
    return this.page.getByText(message, { exact: true });
  }

  /** The pretty-printed entry a row reveals when it is open. */
  expandedDetail(message) {
    return this.row(message).locator("xpath=../following-sibling::pre");
  }

  /** The parts of a row the search box highlighted. */
  highlightsIn(message) {
    return this.row(message).locator("mark");
  }

  /** Opens the level filter, flips the named levels, and closes it again. */
  async toggleLevels(levelLabels) {
    await this.levelFilterButton.click();
    for (const levelLabel of levelLabels) {
      await this.page.getByRole("checkbox", { name: levelLabel }).click();
    }
    await this.closeLevelFilter();
  }

  /** The dropdown closes on a mousedown anywhere outside it. */
  async closeLevelFilter() {
    await this.page.mouse.click(5, 5);
    await expect(this.page.getByRole("checkbox", { name: "DEBUG" })).toBeHidden();
  }

  /** The number a header counter shows — its second span. */
  async statValue(label) {
    const badge = this.page.getByText(label, { exact: true }).locator("xpath=..");
    return Number(await badge.locator("span").last().textContent());
  }

  /** The level selection the page persisted for the next visit. */
  async readStoredLevels() {
    return this.page.evaluate(() => localStorage.getItem("logs.selectedLevels"));
  }
}
