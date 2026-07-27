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

export function buildProject({
  name,
  status = "Completed",
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
 * expanded card fills the viewport on its own. This is the shape that makes the
 * collapsed-by-default behaviour observable.
 */
export function buildMultiJobDashboard({
  jobCount = 5,
  projectsPerJob = 12,
} = {}) {
  return Array.from({ length: jobCount }, (_, jobIndex) =>
    buildRenovateJob({
      name: `team-${String(jobIndex + 1).padStart(2, "0")}`,
      projects: Array.from({ length: projectsPerJob }, (_, projectIndex) =>
        buildProject({
          name: `acme/service-${jobIndex + 1}-${projectIndex + 1}`,
          // One failing project per job so the "failed" stat filter selects a
          // predictable subset of each job's projects.
          status: projectIndex === 0 ? "Failed" : "Completed",
        }),
      ),
    }),
  );
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
        buildProject({ name: "acme/broken-a", status: "Failed" }),
        buildProject({ name: "acme/fine-a", status: "Completed" }),
      ],
    }),
    buildRenovateJob({
      name: "job-failing-b",
      projects: [
        buildProject({ name: "acme/broken-b", status: "Failed" }),
        buildProject({ name: "acme/fine-b", status: "Completed" }),
      ],
    }),
    buildRenovateJob({
      name: "job-all-green",
      projects: [
        buildProject({ name: "acme/green-one", status: "Completed" }),
        buildProject({ name: "acme/green-two", status: "Completed" }),
      ],
    }),
  ];
}
