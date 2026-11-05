import { test, expect } from "../fixtures/dashboardFixture.mjs";
import {
  buildRenovateJob,
  buildProject,
  buildPRActivity,
  buildPRDetail,
  PR_ACTIONS,
} from "../fixtures/renovateJobsFixture.mjs";

// The Tooltip wraps its trigger in a flex <span>. A flex item will not shrink
// below its content width unless it is `min-w-0`, so without that the `truncate`
// on a long PR title stops working and the title pushes the row wider than the
// table cell it sits in.
// Comfortably wider than the 1280px test viewport, so it has to truncate.
const LONG_TITLE =
  "chore(deps): update dependency @some-scope/some-very-long-package-name-that-keeps-going to v42.17.3 " +
  "and refresh every lockfile in the monorepo, " +
  Array.from({ length: 6 }, (_, i) => `bump transitive-dependency-number-${i + 1} as well`).join(", ");

const dashboardWithLongPRTitle = () => [
  buildRenovateJob({
    name: "job-long-pr-title",
    projects: [
      buildProject({
        name: "acme/long-titles",
        prActivity: buildPRActivity({
          created: 1,
          prs: [buildPRDetail({ branch: "renovate/everything", number: 4242, title: LONG_TITLE, action: PR_ACTIONS.created })],
        }),
      }),
    ],
  }),
];

test("a long PR title truncates inside its tooltip wrapper instead of widening the row", async ({ dashboard }) => {
  await dashboard.open(dashboardWithLongPRTitle());
  const page = dashboard.page;

  const row = page.locator('[data-project-row="acme/long-titles"]');
  await row.getByRole("button", { expanded: false }).click();

  const detailRow = page.locator("#pr-details-acme-long-titles");
  const title = detailRow.getByRole("link", { name: LONG_TITLE });
  await expect(title).toBeVisible();

  // The detail row's own width is its budget; the title must not spill past it.
  const [rowBox, titleBox] = await Promise.all([detailRow.boundingBox(), title.boundingBox()]);
  expect(titleBox.width).toBeLessThan(rowBox.width);
  expect(titleBox.x + titleBox.width).toBeLessThanOrEqual(rowBox.x + rowBox.width + 1);
  await expect(title).toHaveCSS("text-overflow", "ellipsis");
  const overflows = await title.evaluate((el) => el.scrollWidth > el.clientWidth);
  expect(overflows).toBe(true);
});
