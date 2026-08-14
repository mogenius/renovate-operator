// Playwright fixture that boots the dashboard against a stubbed operator API.
//
// The frontend is decoupled from the backend — everything it renders arrives as
// JSON from /api/v1/* — so a route stub is enough to drive the real index.html
// through any state, without a Kubernetes cluster.

import { test as base, expect } from "@playwright/test";

const OPERATOR_VERSION = "test-version";
const BASE_PATH = (process.env.BASE_PATH ?? "").trim().replace(/^\/+|\/+$/g, "");
const DASHBOARD_PATH = BASE_PATH === "" ? "/" : `/${BASE_PATH}/`;

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
    await this.page.goto(query.size ? `${DASHBOARD_PATH}?${query}` : DASHBOARD_PATH);
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

  get searchInput() {
    return this.page.getByLabel("Search projects by name");
  }

  /** The bar holding the stat badges, the search box and the bulk controls. */
  get toolbar() {
    return this.page.getByTestId("dashboard-toolbar");
  }

  /** The logo link in the brand strip, which the toolbar is meant to outlive. */
  get brandLogo() {
    return this.page.getByAltText("Renovate Operator Logo");
  }

  async scrollToBottom() {
    await this.page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    // The toolbar's shadow follows a scroll listener, so wait for it to catch up
    // rather than asserting against the pre-scroll frame.
    await expect(this.toolbar).toHaveAttribute("data-stuck", "true");
  }

  /** How far the element's top edge sits below the top of the viewport. */
  async distanceFromViewportTop(locator) {
    const box = await locator.boundingBox();
    return box === null ? null : box.y;
  }

  /**
   * A raw mouse click at the centre of where an element sits, delivered to whatever
   * is topmost at that point. `locator.click()` would refuse the shot when something
   * covers the element, which is exactly the case these assertions are about.
   */
  async clickTopmostElementAt(locator) {
    const box = await locator.boundingBox();
    await this.page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
  }

  /** One of the counters in the page header, e.g. "Open PRs" or "With Issues". */
  statBadge(label) {
    return this.page.getByRole("button", { name: new RegExp(`^${label}\\b`) });
  }

  /** The number a header counter shows — its second span. */
  async statValue(label) {
    const value = await this.statBadge(label).locator("span").last().textContent();
    return Number(value);
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

  /**
   * The project names the expanded cards currently show, in render order. Scoped to
   * the table so the `lg:hidden` card layout — which stays in the DOM at this
   * viewport — is not counted twice.
   */
  async visibleProjectNames() {
    return this.page.locator('table tbody [data-testid="project-name"]').allTextContents();
  }

  /**
   * The gear button in a job card's title row. Located from the heading because the
   * button's aria-label is identical on every card.
   */
  executionOptionsButton(jobName) {
    return this.jobHeading(jobName).locator(
      'xpath=../following-sibling::div[1]//button[@aria-label="Execution options"]',
    );
  }

  /** The popover the gear opens — execution options on top, "Hide Projects" below. */
  executionOptionsMenu(jobName) {
    return this.jobHeading(jobName).locator("xpath=../following-sibling::div[1]");
  }

  /** The heading that is only on screen while the popover is open. */
  executionOptionsPopover(jobName) {
    return this.executionOptionsMenu(jobName).getByRole("heading", { name: "Hide Projects" });
  }

  async openExecutionOptions(jobName) {
    await this.executionOptionsButton(jobName).click();
    await expect(this.executionOptionsPopover(jobName)).toBeVisible();
  }

  /**
   * A checkbox in the "Hide Projects" section. The three renovateResultStatus
   * entries append a count to their label, so match on the leading label text.
   */
  hideProjectsCheckbox(jobName, label) {
    return this.executionOptionsMenu(jobName).getByRole("checkbox", {
      name: new RegExp(`^${label.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}( \\(\\d+\\))?$`),
    });
  }

  /** Checks the named "Hide Projects" boxes and closes the popover again. */
  async hideProjects(jobName, labels) {
    await this.openExecutionOptions(jobName);
    for (const label of labels) {
      await this.hideProjectsCheckbox(jobName, label).check();
    }
    await this.closeExecutionOptions(jobName);
  }

  /**
   * While the popover is open it lays a full-page overlay over everything else, so
   * a click anywhere outside it — including on the gear — lands on that overlay and
   * dismisses the menu.
   */
  async closeExecutionOptions(jobName) {
    await this.page.mouse.click(5, 5);
    await expect(this.executionOptionsPopover(jobName)).toBeHidden();
  }

  /** What the job card persisted for its "Hide Projects" selection. */
  async readStoredProjectFilters(jobKey) {
    return this.page.evaluate((key) => ({
      hiddenStatuses: JSON.parse(localStorage.getItem(`hiddenStatuses:${key}`) || "[]"),
      hideNoPRs: localStorage.getItem(`hideNoPRs:${key}`) === "true",
      hideNoIssues: localStorage.getItem(`hideNoIssues:${key}`) === "true",
    }), jobKey);
  }

  /**
   * The persisted deviations from JOB_CARDS_EXPANDED_BY_DEFAULT, not the expanded
   * jobs — with the default expanded, these are the jobs the user collapsed.
   */
  async readStoredToggledJobCardKeys() {
    return this.page.evaluate(() =>
      JSON.parse(localStorage.getItem("toggledJobCards") || "[]"),
    );
  }
}
