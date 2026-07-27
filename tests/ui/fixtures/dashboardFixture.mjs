// Playwright fixture that boots the dashboard against a stubbed operator API.
//
// The frontend is decoupled from the backend — everything it renders arrives as
// JSON from /api/v1/* — so a route stub is enough to drive the real index.html
// through any state, without a Kubernetes cluster.

import { test as base, expect } from "@playwright/test";

const OPERATOR_VERSION = "test-version";

export const test = base.extend({
  /**
   * dashboard.open(jobs, { statFilter }) stubs the API and loads the page,
   * resolving once the jobs have rendered.
   */
  dashboard: async ({ page }, use) => {
    await use(new DashboardPage(page));
  },
});

export { expect };

class DashboardPage {
  constructor(page) {
    this.page = page;
  }

  async stubApi(renovateJobs) {
    // Matches with and without a BASE_PATH prefix.
    await this.page.route("**/api/v1/auth/status", (route) =>
      route.fulfill({ json: { enabled: false } }),
    );
    await this.page.route("**/api/v1/version", (route) =>
      route.fulfill({ json: { version: OPERATOR_VERSION } }),
    );
    await this.page.route("**/api/v1/renovatejobs", (route) =>
      route.fulfill({ json: renovateJobs }),
    );
  }

  /**
   * @param renovateJobs payload served for /api/v1/renovatejobs
   * @param statFilter   value for the ?filter= query parameter
   * @param search       value for the ?search= query parameter
   * @param expectedJobCount how many cards should render; defaults to every job,
   *        which is not the case when a filter or search hides some of them.
   */
  async open(
    renovateJobs,
    { statFilter = null, search = null, expectedJobCount = renovateJobs.length } = {},
  ) {
    await this.stubApi(renovateJobs);
    const query = new URLSearchParams();
    if (statFilter) query.set("filter", statFilter);
    if (search) query.set("search", search);
    await this.page.goto(query.size ? `/?${query}` : "/");
    // Babel compiles the page's JSX in the browser, so wait for real content
    // rather than a load event.
    await expect(this.jobHeadings.first()).toBeVisible();
    await expect(this.jobHeadings).toHaveCount(expectedJobCount);
  }

  async reload() {
    await this.page.reload();
    await expect(this.jobHeadings.first()).toBeVisible();
  }

  /** One <h2> per job card — the only h2 elements on the page. */
  get jobHeadings() {
    return this.page.locator("h2");
  }

  jobHeading(jobName) {
    return this.page.getByRole("heading", { name: jobName, exact: true });
  }

  /**
   * The chevron button in a job card's title row. It carries aria-hidden, so it
   * has to be matched by CSS rather than by an accessibility query.
   */
  expandToggle(jobName) {
    return this.jobHeading(jobName).locator("xpath=../preceding-sibling::button[1]");
  }

  async isExpanded(jobName) {
    const label = await this.expandToggle(jobName).getAttribute("aria-label");
    return label === "Collapse job details";
  }

  /** Clicking the title row is what a user does to open a card. */
  async toggle(jobName) {
    await this.jobHeading(jobName).click();
  }

  /** The "All" stat badge, the only way back from an active stat filter. */
  get clearStatFilterButton() {
    return this.page.getByTitle("Show all projects");
  }

  /**
   * Total page height expressed in screens. A dashboard that is taller than a
   * screen or two cannot be taken in at a glance, which is what the collapsed
   * default is for.
   */
  async heightInViewports() {
    return this.page.evaluate(
      () => document.documentElement.scrollHeight / window.innerHeight,
    );
  }

  get expandAllButton() {
    return this.page.getByRole("button", { name: "Expand all job cards" });
  }

  get collapseAllButton() {
    return this.page.getByRole("button", { name: "Collapse all job cards" });
  }

  /** Project rows only exist inside expanded cards. */
  get projectRows() {
    return this.page.locator("table tbody tr");
  }

  projectCell(projectName) {
    return this.page.getByRole("cell", { name: projectName, exact: true });
  }

  async readStoredExpandedJobKeys() {
    return this.page.evaluate(() =>
      JSON.parse(localStorage.getItem("expandedJobs") || "[]"),
    );
  }
}
