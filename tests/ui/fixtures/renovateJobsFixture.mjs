// Builders for the JSON the dashboard consumes.
//
// The shapes mirror ui.RenovateJobInfo and crdManager.RenovateProjectStatus in
// src/ui/renovateController.go and src/internal/crdManager/renovateJobManager.go.
// When a field is added there, add it here too — the tests are only as honest as
// these payloads.

// Timestamps are fixed rather than derived from the current time so that the
// relative "x ago" labels the page renders stay stable between runs.
const FIXED_LAST_TRANSITION = "2026-07-27T08:00:00Z";
const FIXED_NEXT_SCHEDULE = "2026-07-28T02:00:00Z";

/** Minutes before FIXED_LAST_TRANSITION, so the "Since" column has a spread to sort by. */
function minutesBeforeLastTransition(minutes) {
  return new Date(Date.parse(FIXED_LAST_TRANSITION) - minutes * 60_000).toISOString();
}

/** api.RenovateProjectStatus — the operator serializes these lowercase. */
export const PROJECT_STATUSES = {
  scheduled: "scheduled",
  running: "running",
  completed: "completed",
  failed: "failed",
  cancelled: "cancelled",
};

/**
 * The `renovateResultStatus` values parser.ParseRenovateLogs derives from the
 * "Repository finished" log line (src/internal/parser/logParser.go). "done" is the
 * uneventful case the UI treats as "nothing to report"; the three the job card's
 * "Hide Projects" menu offers as checkboxes are marked below.
 */
export const RENOVATE_RESULT_STATUSES = {
  done: "done",
  onboardingClosed: "Onboarding Closed", // filterable
  disabled: "Disabled", // filterable
  noConfig: "No Config", // filterable
  onboarding: "Onboarding",
  unknown: "Unknown",
};

/** api.PRAction */
export const PR_ACTIONS = {
  automerged: "automerged",
  created: "created",
  updated: "updated",
  needsApproval: "needs-approval",
  unchanged: "unchanged",
};

/** api.LogIssue levels — pino levels, 40 renders as "warn", 50 as "error". */
export const LOG_LEVELS = { warn: 40, error: 50 };

export function buildProject({
  name,
  status = PROJECT_STATUSES.completed,
  priority = 0,
  renovateResultStatus = null,
  duration = "42s",
  prActivity = null,
  logIssues = null,
  lastTransition = FIXED_LAST_TRANSITION,
} = {}) {
  const project = { name, status, lastTransition };
  if (priority) project.priority = priority;
  if (renovateResultStatus) project.renovateResultStatus = renovateResultStatus;
  if (duration) project.duration = duration;
  if (prActivity) project.prActivity = prActivity;
  if (logIssues) project.logIssues = logIssues;
  return project;
}

/** api.PRActivity — the counts carry the badges, `prs` the expandable detail row. */
export function buildPRActivity({
  automerged = 0,
  created = 0,
  updated = 0,
  needsApproval = 0,
  unchanged = 0,
  prs = [],
  truncated = false,
} = {}) {
  const prActivity = { automerged, created, updated, needsApproval, unchanged };
  if (prs.length) prActivity.prs = prs;
  if (truncated) prActivity.truncated = true;
  return prActivity;
}

/** api.PRDetail */
export function buildPRDetail({ branch, number = 0, title = "", action }) {
  const pullRequest = { branch, action };
  if (number) pullRequest.number = number;
  if (title) pullRequest.title = title;
  return pullRequest;
}

/** api.LogIssues */
export function buildLogIssues({
  warnCount = 0,
  errorCount = 0,
  issues = [],
  truncated = false,
} = {}) {
  const logIssues = { warnCount, errorCount };
  if (issues.length) logIssues.issues = issues;
  if (truncated) logIssues.truncated = true;
  return logIssues;
}

/** api.LogIssue */
export function buildLogIssue({ level = LOG_LEVELS.warn, message }) {
  return { level, message };
}

/**
 * Every project state the dashboard renders differently, one entry each.
 *
 * This is the catalogue the "Hide Projects" menu is exercised against: it holds at
 * least one project for every checkbox in that menu (the three renovateResultStatus
 * values, plus projects with and without PR activity and with and without log
 * issues), and one project that no checkbox touches. `key` doubles as the project
 * name in buildDashboardWithEveryProjectState, so a spec can name the state it
 * expects to survive a filter.
 */
export const PROJECT_STATE_VARIANTS = [
  {
    key: "running",
    description: "mid-run, no result parsed yet",
    project: {
      status: PROJECT_STATUSES.running,
      duration: null,
      lastTransition: minutesBeforeLastTransition(2),
    },
  },
  {
    key: "scheduled",
    description: "queued behind the parallelism limit",
    project: {
      status: PROJECT_STATUSES.scheduled,
      duration: null,
      lastTransition: minutesBeforeLastTransition(5),
    },
  },
  {
    key: "scheduled-prioritized",
    description: "queued after a manual trigger, so the trigger button is spent",
    project: {
      status: PROJECT_STATUSES.scheduled,
      priority: 1,
      duration: null,
      lastTransition: minutesBeforeLastTransition(1),
    },
  },
  {
    key: "cancelled",
    description: "cancelled by the user — no badge colour of its own",
    project: {
      status: PROJECT_STATUSES.cancelled,
      duration: "8s",
      lastTransition: minutesBeforeLastTransition(90),
    },
  },
  {
    key: "failed-with-errors",
    description: "the run died; errors but no PR activity",
    project: {
      status: PROJECT_STATUSES.failed,
      duration: "11s",
      lastTransition: minutesBeforeLastTransition(30),
      logIssues: buildLogIssues({
        errorCount: 2,
        warnCount: 1,
        issues: [
          buildLogIssue({ level: LOG_LEVELS.error, message: "Failed to look up npm package @acme/ui" }),
          buildLogIssue({ level: LOG_LEVELS.error, message: "Error updating branch: update failure" }),
          buildLogIssue({ level: LOG_LEVELS.warn, message: "Package lookup failures" }),
        ],
      }),
    },
  },
  {
    key: "completed-quiet",
    description: "nothing to do — hidden by both 'Without Open PRs' and 'Without Issues'",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.done,
      duration: "19s",
      lastTransition: minutesBeforeLastTransition(240),
    },
  },
  {
    key: "completed-with-created-prs",
    description: "fresh PRs, clean logs — survives 'Without Open PRs', hidden by 'Without Issues'",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.done,
      duration: "1m4s",
      lastTransition: minutesBeforeLastTransition(180),
      prActivity: buildPRActivity({
        created: 2,
        prs: [
          buildPRDetail({ branch: "renovate/react-19.x", number: 412, title: "chore(deps): update react to v19", action: PR_ACTIONS.created }),
          buildPRDetail({ branch: "renovate/go-1.24", number: 413, title: "chore(deps): update golang to 1.24", action: PR_ACTIONS.created }),
        ],
      }),
    },
  },
  {
    key: "completed-with-updated-and-unchanged-prs",
    description: "long-lived PR backlog, no new work",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.done,
      duration: "58s",
      lastTransition: minutesBeforeLastTransition(200),
      prActivity: buildPRActivity({
        updated: 1,
        unchanged: 4,
        prs: [
          buildPRDetail({ branch: "renovate/postgres-17.x", number: 388, title: "chore(deps): update postgres to v17", action: PR_ACTIONS.updated }),
          buildPRDetail({ branch: "renovate/lodash-4.x", number: 301, title: "chore(deps): update lodash", action: PR_ACTIONS.unchanged }),
        ],
      }),
    },
  },
  {
    key: "completed-needs-approval-only",
    description:
      "an open PR waiting on approval and nothing else — the 'Pending' stat counts it " +
      "and 'Without Open PRs' keeps it",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.done,
      duration: "33s",
      lastTransition: minutesBeforeLastTransition(300),
      prActivity: buildPRActivity({
        needsApproval: 1,
        prs: [
          buildPRDetail({ branch: "renovate/major-express-5.x", number: 501, title: "chore(deps): update express to v5", action: PR_ACTIONS.needsApproval }),
        ],
      }),
    },
  },
  {
    key: "completed-automerged-only",
    description:
      "everything merged itself — PR activity happened but nothing is open, so " +
      "'Without Open PRs' hides it just like the 'Open PRs' stat ignores it",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.done,
      duration: "47s",
      lastTransition: minutesBeforeLastTransition(320),
      prActivity: buildPRActivity({
        automerged: 3,
        prs: [
          buildPRDetail({ branch: "renovate/patch-updates", number: 495, title: "fix(deps): patch updates", action: PR_ACTIONS.automerged }),
        ],
      }),
    },
  },
  {
    key: "completed-with-warnings",
    description: "warnings only, no PRs — survives 'Without Issues', hidden by 'Without Open PRs'",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.done,
      duration: "26s",
      lastTransition: minutesBeforeLastTransition(400),
      logIssues: buildLogIssues({
        warnCount: 3,
        issues: [
          buildLogIssue({ level: LOG_LEVELS.warn, message: "Package file has unsupported registry" }),
          buildLogIssue({ level: LOG_LEVELS.warn, message: "Dependency lookup failure for internal/tooling" }),
          buildLogIssue({ level: LOG_LEVELS.warn, message: "Branch has been modified externally" }),
        ],
      }),
    },
  },
  {
    key: "completed-with-errors-and-prs",
    description: "the busiest row — PR badges and issue badges side by side",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.done,
      duration: "2m11s",
      lastTransition: minutesBeforeLastTransition(420),
      prActivity: buildPRActivity({
        created: 1,
        updated: 2,
        needsApproval: 1,
        prs: [
          buildPRDetail({ branch: "renovate/kubernetes-1.34", number: 620, title: "chore(deps): update kubernetes to 1.34", action: PR_ACTIONS.created }),
          buildPRDetail({ branch: "renovate/otel-monorepo", number: 611, title: "chore(deps): update opentelemetry monorepo", action: PR_ACTIONS.updated }),
          buildPRDetail({ branch: "renovate/major-vite-7.x", number: 604, title: "chore(deps): update vite to v7", action: PR_ACTIONS.needsApproval }),
        ],
      }),
      logIssues: buildLogIssues({
        errorCount: 1,
        warnCount: 2,
        issues: [
          buildLogIssue({ level: LOG_LEVELS.error, message: "Artifact update error in package-lock.json" }),
          buildLogIssue({ level: LOG_LEVELS.warn, message: "Registry rate limit reached, retrying" }),
          buildLogIssue({ level: LOG_LEVELS.warn, message: "Ignoring pinned dependency internal/base" }),
        ],
      }),
    },
  },
  {
    key: "completed-with-truncated-detail",
    description: "more PRs and issues than the CRD keeps — both detail rows show the cut-off note",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.done,
      duration: "3m2s",
      lastTransition: minutesBeforeLastTransition(440),
      prActivity: buildPRActivity({
        created: 12,
        updated: 9,
        unchanged: 21,
        truncated: true,
        prs: Array.from({ length: 5 }, (_, index) =>
          buildPRDetail({
            branch: `renovate/bulk-update-${index + 1}`,
            number: 700 + index,
            title: `chore(deps): bulk update ${index + 1}`,
            action: PR_ACTIONS.created,
          }),
        ),
      }),
      logIssues: buildLogIssues({
        warnCount: 34,
        errorCount: 6,
        truncated: true,
        issues: Array.from({ length: 5 }, (_, index) =>
          buildLogIssue({
            level: index % 2 === 0 ? LOG_LEVELS.warn : LOG_LEVELS.error,
            message: `Dependency lookup failure for vendor/module-${index + 1}`,
          }),
        ),
      }),
    },
  },
  {
    key: "onboarding-closed",
    description: "the onboarding PR was closed — 'Onboarding Closed' checkbox",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.onboardingClosed,
      duration: "6s",
      lastTransition: minutesBeforeLastTransition(1440),
    },
  },
  {
    key: "onboarding-open",
    description: "onboarding PR still open — reads like a normal PR, no checkbox of its own",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.onboarding,
      duration: "14s",
      lastTransition: minutesBeforeLastTransition(1500),
      prActivity: buildPRActivity({
        created: 1,
        prs: [
          buildPRDetail({ branch: "renovate/configure", number: 1, title: "Configure Renovate", action: PR_ACTIONS.created }),
        ],
      }),
    },
  },
  {
    key: "disabled",
    description: "renovate disabled in the repo config — 'Disabled' checkbox",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.disabled,
      duration: "4s",
      lastTransition: minutesBeforeLastTransition(2880),
    },
  },
  {
    key: "no-config",
    description: "no renovate config found — 'No Config' checkbox",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.noConfig,
      duration: "5s",
      lastTransition: minutesBeforeLastTransition(2900),
    },
  },
  {
    key: "unknown-result",
    description: "the log line parsed but the result was not recognised",
    project: {
      renovateResultStatus: RENOVATE_RESULT_STATUSES.unknown,
      duration: "9s",
      lastTransition: minutesBeforeLastTransition(3000),
    },
  },
];

const variantByKey = new Map(PROJECT_STATE_VARIANTS.map((variant) => [variant.key, variant]));

/** @param variantKey a key from PROJECT_STATE_VARIANTS */
export function buildProjectInState(name, variantKey) {
  const variant = variantByKey.get(variantKey);
  if (!variant) {
    throw new Error(`unknown project state variant: ${variantKey}`);
  }
  return buildProject({ name, ...variant.project });
}

/**
 * The states every job card in buildMultiJobDashboard carries, so that each card on
 * its own can drive every checkbox in its "Hide Projects" menu.
 */
const VARIANT_KEYS_IN_EVERY_JOB = [
  "failed-with-errors",
  "onboarding-closed",
  "disabled",
  "no-config",
  "completed-quiet",
  "completed-with-errors-and-prs",
];

const REMAINING_VARIANT_KEYS = PROJECT_STATE_VARIANTS.map((variant) => variant.key).filter(
  (key) => !VARIANT_KEYS_IN_EVERY_JOB.includes(key),
);

export function buildRenovateJob({
  name,
  namespace = "renovate",
  projects = [],
  cronExpression = "0 2 * * *",
  discoveryStatus = "Completed",
  platform = "github",
  platformEndpoint = "https://api.github.com",
  debug = false,
} = {}) {
  return {
    name,
    namespace,
    cronExpression,
    nextSchedule: FIXED_NEXT_SCHEDULE,
    discoveryStatus,
    projects,
    platform,
    platformEndpoint,
    executionOptions: { debug },
  };
}

/**
 * A dashboard with several jobs, each holding enough projects that a single
 * expanded card fills the viewport on its own. Enough rows that the difference
 * between an expanded and a collapsed card is unmistakable.
 *
 * Every card leads with VARIANT_KEYS_IN_EVERY_JOB — including one failing project,
 * so the "failed" stat filter selects a predictable subset of each job — and fills
 * the rest from the remaining states, rotated by job index so no two cards look
 * alike. The result is the payload `just ui-dev` serves as well.
 */
export function buildMultiJobDashboard({ jobCount = 5, projectsPerJob = 12 } = {}) {
  return Array.from({ length: jobCount }, (_, jobIndex) =>
    buildRenovateJob({
      name: `team-${String(jobIndex + 1).padStart(2, "0")}`,
      projects: Array.from({ length: projectsPerJob }, (_, projectIndex) => {
        const variantKey =
          projectIndex < VARIANT_KEYS_IN_EVERY_JOB.length
            ? VARIANT_KEYS_IN_EVERY_JOB[projectIndex]
            : REMAINING_VARIANT_KEYS[
                (projectIndex - VARIANT_KEYS_IN_EVERY_JOB.length + jobIndex) %
                  REMAINING_VARIANT_KEYS.length
              ];
        return buildProjectInState(`acme/service-${jobIndex + 1}-${projectIndex + 1}`, variantKey);
      }),
    }),
  );
}

/**
 * One job holding exactly one project per entry in PROJECT_STATE_VARIANTS, named
 * after the variant key. A spec can therefore assert on the states a filter leaves
 * behind by name, without counting rows.
 */
export function buildDashboardWithEveryProjectState({ jobName = "job-all-states" } = {}) {
  return [
    buildRenovateJob({
      name: jobName,
      projects: PROJECT_STATE_VARIANTS.map((variant) =>
        buildProjectInState(`acme/${variant.key}`, variant.key),
      ),
    }),
  ];
}

/**
 * Three jobs of which two have a failing project. Under `?filter=failed` the
 * dashboard shows those two and hides the third entirely, which is the setup the
 * expand-all / collapse-all scoping needs: more than one visible job (so the bulk
 * controls render at all) plus one job that must stay untouched.
 */
export function buildDashboardWithTwoFailingJobs() {
  return [
    buildRenovateJob({
      name: "job-failing-a",
      projects: [
        buildProject({ name: "acme/broken-a", status: PROJECT_STATUSES.failed }),
        buildProject({ name: "acme/fine-a", status: PROJECT_STATUSES.completed }),
      ],
    }),
    buildRenovateJob({
      name: "job-failing-b",
      projects: [
        buildProject({ name: "acme/broken-b", status: PROJECT_STATUSES.failed }),
        buildProject({ name: "acme/fine-b", status: PROJECT_STATUSES.completed }),
      ],
    }),
    buildRenovateJob({
      name: "job-all-green",
      projects: [
        buildProject({ name: "acme/green-one", status: PROJECT_STATUSES.completed }),
        buildProject({ name: "acme/green-two", status: PROJECT_STATUSES.completed }),
      ],
    }),
  ];
}
