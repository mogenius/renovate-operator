// Refines the "With Issues" stat filter (mogenius/renovate-operator#804).
//
// "With Issues" means "any log line at level 40 or above", so on its own it lumps together a
// repository that is not onboarded yet, a broken config, a registry outage and a deprecation
// notice. Once that badge is selected, three facets narrow it down:
//
//   Severity    errors only (fatal counts as an error, as in the operator's log parser)
//   Category    what the issues are about: LogIssue.category / LogIssues.categoryCounts,
//               set by the parser (src/internal/parser/issueCategory.go)
//   Repository  the renovateResultStatus of the run that produced the issues
//
// Facets combine with AND, values within one facet with OR. All of them are decided per
// project: categoryCounts covers every warning and error line but is not split by level, so
// "errors only" plus a category means "has errors, and has issues in that category".

// Keys match api.LogIssueCategory; the order is the display order.
const ISSUE_CATEGORIES = [
  { key: "config", short: "Config", label: "Config", hint: "The repository's Renovate config or a preset it extends" },
  { key: "deprecation", short: "Deprecation", label: "Deprecations", hint: "Works today, will stop working in a later Renovate version" },
  { key: "platform", short: "Platform", label: "Platform & access", hint: "Authentication, permissions or rate limits of the Git platform or a registry" },
  { key: "lookup", short: "Lookup", label: "Lookup failures", hint: "A dependency's versions could not be looked up" },
  { key: "branches", short: "Branches", label: "Branches & artifacts", hint: "Creating or updating a branch, a lock file or another artifact failed" },
  { key: "other", short: "Other", label: "Other", hint: "Not recognised, or counted without a message" },
];
const ISSUE_CATEGORY_BY_KEY = Object.fromEntries(ISSUE_CATEGORIES.map((c) => [c.key, c]));
const FALLBACK_ISSUE_CATEGORY = "other";

// The known category of one stored issue, or null for an issue stored before the operator
// categorised issues.
function issueCategory(issue) {
  return ISSUE_CATEGORY_BY_KEY[issue?.category] ? issue.category : null;
}

// How many warning and error lines of a project's last run fall into each category. A status
// written before the operator counted categories has issues but no counts; all of them are
// then "other" until the project runs again, so the project never drops out of every category.
function projectIssueCategoryCounts(project) {
  const logIssues = project?.logIssues;
  if (!logIssues) return {};
  const total = (logIssues.warnCount || 0) + (logIssues.errorCount || 0);
  const counts = {};
  for (const [category, count] of Object.entries(logIssues.categoryCounts || {})) {
    if (count > 0) {
      const key = ISSUE_CATEGORY_BY_KEY[category] ? category : FALLBACK_ISSUE_CATEGORY;
      counts[key] = (counts[key] || 0) + count;
    }
  }
  if (Object.keys(counts).length === 0 && total > 0) counts[FALLBACK_ISSUE_CATEGORY] = total;
  return counts;
}

const REPOSITORY_STATES = [
  { key: "active", label: "Onboarded", hint: "Renovate ran on its config" },
  { key: "onboarding", label: "Onboarding", hint: "The onboarding PR is still open" },
  { key: "onboardingClosed", label: "Onboarding closed", hint: "The onboarding PR was closed without merging" },
  { key: "disabled", label: "Disabled", hint: "Renovate is disabled in the repository's config" },
  { key: "noConfig", label: "No config", hint: "No Renovate config found" },
];
const REPOSITORY_STATE_BY_RESULT = {
  "Onboarding": "onboarding",
  "Onboarding Closed": "onboardingClosed",
  "Disabled": "disabled",
  "No Config": "noConfig",
};

function projectRepositoryState(project) {
  return REPOSITORY_STATE_BY_RESULT[project?.renovateResultStatus] || "active";
}

const EMPTY_ISSUE_REFINEMENT = Object.freeze({ errorsOnly: false, categories: [], repositoryStates: [] });

function issueRefinementIsActive(refinement) {
  return !!refinement && (refinement.errorsOnly || refinement.categories.length > 0 || refinement.repositoryStates.length > 0);
}

// `skip` leaves one facet out, which is how each facet's chip counts are computed: a chip
// says how many projects selecting it would show, given the other facets.
function projectMatchesIssueRefinement(project, refinement, skip = null) {
  if (!refinement) return true;
  if (skip !== "repositoryStates" && refinement.repositoryStates.length > 0
      && !refinement.repositoryStates.includes(projectRepositoryState(project))) {
    return false;
  }
  if (skip !== "errorsOnly" && refinement.errorsOnly && (project?.logIssues?.errorCount || 0) === 0) {
    return false;
  }
  if (skip !== "categories" && refinement.categories.length > 0) {
    const counts = projectIssueCategoryCounts(project);
    if (!refinement.categories.some((category) => counts[category] > 0)) return false;
  }
  return true;
}

const ISSUE_REFINEMENT_PARAMS = { categories: "issues", errorsOnly: "severity", repositoryStates: "repo" };

function readIssueRefinementFromUrl() {
  try {
    const params = new URLSearchParams(window.location.search);
    const list = (name, allowed) => (params.get(name) || "").split(",").filter((v) => allowed.includes(v));
    return {
      errorsOnly: params.get(ISSUE_REFINEMENT_PARAMS.errorsOnly) === "error",
      categories: list(ISSUE_REFINEMENT_PARAMS.categories, ISSUE_CATEGORIES.map((c) => c.key)),
      repositoryStates: list(ISSUE_REFINEMENT_PARAMS.repositoryStates, REPOSITORY_STATES.map((s) => s.key)),
    };
  } catch {
    return EMPTY_ISSUE_REFINEMENT;
  }
}

function writeIssueRefinementToUrl(refinement) {
  try {
    const url = new URL(window.location.href);
    const set = (name, value) => (value ? url.searchParams.set(name, value) : url.searchParams.delete(name));
    set(ISSUE_REFINEMENT_PARAMS.errorsOnly, refinement.errorsOnly ? "error" : "");
    set(ISSUE_REFINEMENT_PARAMS.categories, refinement.categories.join(","));
    set(ISSUE_REFINEMENT_PARAMS.repositoryStates, refinement.repositoryStates.join(","));
    window.history.replaceState(null, "", url.pathname + url.search + url.hash);
  } catch {}
}

function describeIssueRefinement(refinement) {
  const parts = [];
  if (refinement.errorsOnly) parts.push("errors only");
  if (refinement.categories.length) parts.push(refinement.categories.map((k) => ISSUE_CATEGORY_BY_KEY[k].label).join(" or "));
  if (refinement.repositoryStates.length) {
    parts.push(refinement.repositoryStates.map((k) => REPOSITORY_STATES.find((s) => s.key === k).label).join(" or "));
  }
  return parts.join(" · ");
}

function RefinementChip({ label, count, selected, onClick, hint, tone = "default" }) {
  const toneClass = selected
    ? (tone === "error"
      ? "border-red-400 bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300 dark:border-red-600"
      : "border-primary bg-primary/10 text-primary dark:bg-primary/20")
    : "border-gray-200 bg-white text-gray-700 hover:border-gray-300 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-300";
  const empty = count === 0 && !selected;
  return (
    <Tooltip content={hint}>
      <button
        type="button"
        onClick={onClick}
        aria-pressed={selected}
        disabled={empty}
        className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors disabled:cursor-default disabled:opacity-40 ${toneClass}`}
      >
        {label}
        <span className="tabular-nums text-[0.688rem] opacity-70">{count}</span>
      </button>
    </Tooltip>
  );
}

// Shown under the stat badges while "With Issues" is selected. `projects` are every project
// that has issues, before any refinement.
function IssueRefinementBar({ projects, refinement, onChange }) {
  const count = (skip, extra) => projects.filter((p) => projectMatchesIssueRefinement(p, refinement, skip) && extra(p)).length;
  const toggle = (field, key) => {
    const current = refinement[field];
    onChange({ ...refinement, [field]: current.includes(key) ? current.filter((k) => k !== key) : [...current, key] });
  };
  const groupLabel = "text-[0.625rem] uppercase tracking-wider text-gray-500 dark:text-slate-400 whitespace-nowrap";

  return (
    <div
      className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-amber-200 bg-amber-50/60 px-3 py-2 dark:border-amber-700/60 dark:bg-amber-900/10"
      role="group"
      aria-label="Refine With Issues"
      data-testid="issue-refinement"
    >
      <div className="flex items-center gap-1.5">
        <span className={groupLabel}>Severity</span>
        <RefinementChip
          label="Errors only"
          tone="error"
          count={count("errorsOnly", (p) => (p.logIssues?.errorCount || 0) > 0)}
          selected={refinement.errorsOnly}
          onClick={() => onChange({ ...refinement, errorsOnly: !refinement.errorsOnly })}
          hint="Hide projects that only warned. Fatal counts as an error."
        />
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <span className={groupLabel}>Category</span>
        {ISSUE_CATEGORIES.map((category) => (
          <RefinementChip
            key={category.key}
            label={category.label}
            count={count("categories", (p) => projectIssueCategoryCounts(p)[category.key] > 0)}
            selected={refinement.categories.includes(category.key)}
            onClick={() => toggle("categories", category.key)}
            hint={category.hint}
          />
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <span className={groupLabel}>Repository</span>
        {REPOSITORY_STATES.map((state) => (
          <RefinementChip
            key={state.key}
            label={state.label}
            count={count("repositoryStates", (p) => projectRepositoryState(p) === state.key)}
            selected={refinement.repositoryStates.includes(state.key)}
            onClick={() => toggle("repositoryStates", state.key)}
            hint={state.hint}
          />
        ))}
      </div>
      {issueRefinementIsActive(refinement) && (
        <button
          type="button"
          onClick={() => onChange(EMPTY_ISSUE_REFINEMENT)}
          className="ml-auto text-xs font-medium text-primary hover:text-primary-hover"
        >
          Reset
        </button>
      )}
    </div>
  );
}

// The categories a row's issues fall into, as small chips next to the warn/error badges.
function IssueCategoryChips({ project }) {
  const counts = projectIssueCategoryCounts(project);
  const categories = ISSUE_CATEGORIES.filter((category) => counts[category.key] > 0);
  if (categories.length === 0) return null;
  return (
    <span className="flex flex-wrap items-center gap-1" data-testid="issue-categories">
      {categories.map((category) => (
        <span
          key={category.key}
          title={`${counts[category.key]} × ${category.label}: ${category.hint}`}
          className="whitespace-nowrap rounded border border-gray-200 px-1.5 py-px text-[0.625rem] font-medium text-gray-600 dark:border-slate-600 dark:text-slate-400"
        >
          {category.short}
        </span>
      ))}
    </span>
  );
}

Object.assign(window, {
  ISSUE_CATEGORIES,
  ISSUE_CATEGORY_BY_KEY,
  REPOSITORY_STATES,
  EMPTY_ISSUE_REFINEMENT,
  issueCategory,
  projectIssueCategoryCounts,
  projectRepositoryState,
  issueRefinementIsActive,
  projectMatchesIssueRefinement,
  readIssueRefinementFromUrl,
  writeIssueRefinementToUrl,
  describeIssueRefinement,
  IssueRefinementBar,
  IssueCategoryChips,
});
