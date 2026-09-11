// Covers which projects the job card's trigger button acts on.
//
// The button is the only bulk action on the dashboard, and the list it sits above
// is filterable three ways: the stat badges and the search box in the toolbar both
// narrow every card, and each card's "Hide Projects" menu narrows its own rows.
// Triggering more repositories than the filtered list shows is not recoverable —
// the runs are already queued — so the scope has to be the rows on screen
// (mogenius/renovate-operator#626).
//
// The page therefore names the projects it means in the request body. Only when
// nothing is filtered does it leave them out, which is what tells the operator to
// take every project it knows of, including ones discovered after this page load.

import { test, expect } from "../fixtures/dashboardFixture.mjs";
import {
  buildDashboardWithEveryProjectState,
  buildDashboardWithNestedProjectPaths,
} from "../fixtures/renovateJobsFixture.mjs";

test.describe("trigger all scope", () => {
  test("triggers every project when nothing is filtered", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithNestedProjectPaths());

    await dashboard.triggerAll("team-platform");

    // No project list at all: the operator decides what "all" means, so a project
    // discovered since this page loaded is not silently left out.
    expect(dashboard.triggerAllRequests).toEqual([
      { renovateJob: "team-platform", namespace: "renovate" },
    ]);
    await expect(dashboard.triggerAllButton("team-platform")).toHaveText("Trigger All");
  });

  test("triggers only the projects the search box left visible", async ({ dashboard }) => {
    // Both jobs hold a project matching "api", but only team-platform has the path.
    await dashboard.open(buildDashboardWithNestedProjectPaths(), {
      search: "platform/api",
      expectedJobCount: 1,
    });

    const visible = await dashboard.projectNamesInJob("team-platform");
    expect(visible).toEqual(["acme/platform/api-docs", "acme/platform/api-gateway"]);

    await dashboard.triggerAll("team-platform");

    expect(dashboard.triggerAllRequests).toEqual([
      {
        renovateJob: "team-platform",
        namespace: "renovate",
        projects: visible,
      },
    ]);
  });

  test("triggers only the projects the stat filter left visible", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithEveryProjectState(), { statFilter: "failed" });

    const visible = await dashboard.projectNamesInJob("job-all-states");
    expect(visible).toEqual(["acme/failed-with-errors"]);

    await dashboard.triggerAll("job-all-states");

    expect(dashboard.triggerAllRequests).toEqual([
      {
        renovateJob: "job-all-states",
        namespace: "renovate",
        projects: ["acme/failed-with-errors"],
      },
    ]);
  });

  test("leaves out the projects a card's Hide Projects menu removed", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithEveryProjectState());

    await dashboard.hideProjects("job-all-states", ["Disabled", "No Config"]);

    const visible = await dashboard.projectNamesInJob("job-all-states");
    expect(visible).not.toContain("acme/disabled");
    expect(visible).not.toContain("acme/no-config");
    expect(visible).toContain("acme/completed-quiet");

    await dashboard.triggerAll("job-all-states");

    expect(dashboard.triggerAllRequests).toEqual([
      {
        renovateJob: "job-all-states",
        namespace: "renovate",
        projects: visible,
      },
    ]);
  });

  test("says how many projects it will trigger while a filter is active", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithNestedProjectPaths(), {
      search: "platform/api",
      expectedJobCount: 1,
    });

    // The label is the only warning a user gets before an unrecoverable bulk action,
    // so it has to state the narrowed scope rather than keep saying "All".
    const triggerButton = dashboard.triggerAllButton("team-platform");
    await expect(triggerButton).toHaveText("Trigger 2");
    await expect(triggerButton).toHaveAttribute(
      "aria-label",
      "Trigger 2 filtered projects for team-platform",
    );

    await dashboard.searchInput.fill("");
    await expect(triggerButton).toHaveText("Trigger All");
  });

  test("keeps the debug half of the button on the same scope", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithNestedProjectPaths(), {
      search: "tooling/",
      expectedJobCount: 1,
    });

    await dashboard.triggerAll("team-platform", { debug: true });

    expect(dashboard.triggerAllRequests).toEqual([
      {
        renovateJob: "team-platform",
        namespace: "renovate",
        projects: ["acme/tooling/cli"],
        executionOptions: { debug: true },
      },
    ]);
  });

  test("cannot be pressed when a filter leaves a job without projects", async ({ dashboard }) => {
    await dashboard.open(buildDashboardWithEveryProjectState());

    // "Onboarding Closed", "Disabled" and "No Config" are the only checkboxes, so
    // the emptiest a card gets this way still leaves rows — drive it from the search
    // box instead, which can empty a card completely.
    await dashboard.searchInput.fill("acme/disabled");
    await expect(dashboard.triggerAllButton("job-all-states")).toHaveText("Trigger 1");

    await dashboard.searchInput.fill("acme/nothing-matches-this");
    await expect(dashboard.jobHeadings).toHaveCount(0);
    expect(dashboard.triggerAllRequests).toEqual([]);
  });
});
