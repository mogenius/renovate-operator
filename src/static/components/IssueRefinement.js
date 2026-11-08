// PROTOTYPE (mockup/with-issues-filters): refines the "With Issues" stat filter.
//
// "With Issues" means "any log line at level 40 or above", so it lumps together a repo that
// is not onboarded yet, a broken config, a registry outage and a deprecation notice. This
// file sketches the refinement proposed for that badge: three facets, shown once the badge
// is selected, that narrow the projects it selects.
//
//   Severity    warnings and errors | errors only (fatal counts as an error, as in the parser)
//   Category    what the message is about (see ISSUE_CATEGORIES)
//   Repository  the renovateResultStatus of the run that produced the issues
//
// The category is guessed here from the message text. That is a stand-in: the proposal is
// for the operator's log parser to classify each issue (LogIssue.category) and count per
// category, because the UI only sees the first 20 distinct messages, while the counts
// cover every line.

const ISSUE_CATEGORIES = [
  { key: "config", short: "Config", label: "Config", hint: "The repository's Renovate config or a preset it extends", pattern: /\bconfig\b|preset|validation/i },
  { key: "deprecation", short: "Deprecation", label: "Deprecations", hint: "Works today, will stop working in a later Renovate version", pattern: /deprecat|no longer supported|will be removed/i },
  { key: "platform", short: "Platform", label: "Platform & access", hint: "Authentication, permissions or rate limits of the Git platform or a registry", pattern: /rate limit|authenticat|unauthori[sz]ed|forbidden|\b40[13]\b|permission|not accessible|token/i },
  { key: "lookup", short: "Lookup", label: "Lookup failures", hint: "A dependency's versions could not be looked up", pattern: /look ?up|registry|datasource|package file/i },
  { key: "branches", short: "Branches", label: "Branches & artifacts", hint: "Creating or updating a branch, a lock file or another artifact failed", pattern: /branch|artifact|lock ?file|rebase|conflict|command failed|npm ERR/i },
  { key: "other", short: "Other", label: "Other", hint: "Not recognised, or counted without a stored message", pattern: null },
];
// Deprecations are matched before config so that "...config option X is deprecated" lands
// there; the list order is the display order.
const ISSUE_CATEGORY_MATCH_ORDER = ["deprecation", "config", "platform", "lookup", "branches"];
const ISSUE_CATEGORY_BY_KEY = Object.fromEntries(ISSUE_CATEGORIES.map((c) => [c.key, c]));

function categorizeIssueMessage(message) {
  for (const key of ISSUE_CATEGORY_MATCH_ORDER) {
    if (ISSUE_CATEGORY_BY_KEY[key].pattern.test(message || "")) return key;
  }
  return "other";
}

// The issues a project's row can be filtered on, each with its category. A project whose
// counts are non-zero but whose messages were all empty gets one synthetic "other" issue, so
// it never drops out of every category.
function categorizedProjectIssues(project) {
  const logIssues = project?.logIssues;
  if (!logIssues) return [];
  const issues = (logIssues.issues || []).map((issue) => ({ ...issue, category: categorizeIssueMessage(issue.message) }));
  if (issues.length === 0 && ((logIssues.warnCount || 0) > 0 || (logIssues.errorCount || 0) > 0)) {
    issues.push({ level: (logIssues.errorCount || 0) > 0 ? 50 : 40, message: "", category: "other" });
  }
  return issues;
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
  const errorsOnly = skip !== "errorsOnly" && refinement.errorsOnly;
  const categories = skip === "categories" ? [] : refinement.categories;
  if (!errorsOnly && categories.length === 0) return true;
  if (errorsOnly && (project?.logIssues?.errorCount || 0) === 0) return false;
  return categorizedProjectIssues(project).some((issue) =>
    (!errorsOnly || issue.level >= 50) && (categories.length === 0 || categories.includes(issue.category)));
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
            count={count("categories", (p) => projectMatchesIssueRefinement(p, { ...refinement, categories: [category.key] }, "repositoryStates"))}
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
      <span className="basis-full text-[0.688rem] italic text-gray-500 dark:text-slate-400">
        Prototype: categories are guessed from the message text in the browser. Proposed: the operator classifies each issue.
      </span>
    </div>
  );
}

// The categories a row's issues fall into, as small chips next to the warn/error badges.
function IssueCategoryChips({ project }) {
  const keys = [...new Set(categorizedProjectIssues(project).map((issue) => issue.category))];
  if (keys.length === 0) return null;
  return (
    <span className="flex flex-wrap items-center gap-1">
      {ISSUE_CATEGORIES.filter((c) => keys.includes(c.key)).map((category) => (
        <span
          key={category.key}
          title={category.hint}
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
  categorizeIssueMessage,
  categorizedProjectIssues,
  projectRepositoryState,
  issueRefinementIsActive,
  projectMatchesIssueRefinement,
  readIssueRefinementFromUrl,
  writeIssueRefinementToUrl,
  describeIssueRefinement,
  IssueRefinementBar,
  IssueCategoryChips,
});
