// First-run setup guide rendered by the dashboard while /api/v1/setup/status
// reports an incomplete install. The backend owns the step states; this
// component owns the copy and the copy-paste YAML for each step. It never
// writes to the cluster — the admin applies the generated manifests with
// kubectl, and the 30s dashboard poll ticks the steps off as the cluster
// state changes.
//
// The sub-components live at module scope: defining them inside SetupGuide
// would mint a new component type per render, and React would then remount
// the inputs on every keystroke and drop their focus.

const SETUP_DOCS_BASE = "https://github.com/mogenius/renovate-operator/blob/main/docs";

const SETUP_PROVIDERS = [
  { key: "github", label: "GitHub (PAT)", provider: "github" },
  { key: "github-app", label: "GitHub App", provider: "github" },
  { key: "gitlab", label: "GitLab", provider: "gitlab" },
  { key: "gitea", label: "Gitea", provider: "gitea" },
  { key: "forgejo", label: "Forgejo", provider: "forgejo" },
  { key: "bitbucket", label: "Bitbucket", provider: "bitbucket" },
];

const SETUP_HINTS = {
  auth: {
    title: "Secure the UI",
    body: "The UI is currently open to everyone who can reach it. Configure OIDC or GitHub OAuth, and set authorization.defaults.adminUsers so you keep access.",
    href: `${SETUP_DOCS_BASE}/configuration/auth.md`,
  },
  policy: {
    title: "Turn on the policy engine",
    body: "The policy engine ships disabled so a new install works out of the box. Without it, anyone who can create a RenovateJob can point tokens and webhooks at hosts of their choosing.",
    href: `${SETUP_DOCS_BASE}/security/security.md`,
  },
  logStorage: {
    title: "Keep logs after pods are cleaned up",
    body: "Log storage is disabled, so Renovate logs disappear with their pods. Set config.logStorage.mode to memory, valkey or s3.",
    href: `${SETUP_DOCS_BASE}/README.md`,
  },
};

function SetupStateIcon({ state, index }) {
  if (state === "done") {
    return (
      <span className="flex h-7 w-7 items-center justify-center rounded-full bg-success/15 text-success flex-shrink-0">
        <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20" aria-hidden="true">
          <path
            fillRule="evenodd"
            d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
            clipRule="evenodd"
          />
        </svg>
      </span>
    );
  }
  if (state === "blocked") {
    return (
      <span className="flex h-7 w-7 items-center justify-center rounded-full bg-warning/15 text-warning flex-shrink-0">
        <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20" aria-hidden="true">
          <path
            fillRule="evenodd"
            d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
            clipRule="evenodd"
          />
        </svg>
      </span>
    );
  }
  return (
    <span className="flex h-7 w-7 items-center justify-center rounded-full border border-gray-300 dark:border-slate-600 text-xs font-bold text-gray-500 dark:text-slate-400 flex-shrink-0">
      {index}
    </span>
  );
}

function SetupYamlBlock({ id, text, copiedId, onCopy }) {
  // "Copy kubectl" wraps the manifest in a pipe to kubectl apply, ready to
  // paste into a terminal without saving a file first. printf with a
  // single-quoted multiline string works in fish, bash and zsh alike — a
  // heredoc would not (fish has none) — and the single quotes keep the shell
  // from expanding anything inside the manifest. The generated manifests
  // never contain a single quote (Kubernetes names cannot), so no escaping
  // is needed.
  const kubectlText = `printf '%s\\n' '${text}' | kubectl apply -f -`;
  const buttonClass =
    "px-2 py-1 text-xs font-medium rounded-md bg-gray-700/80 text-gray-200 hover:bg-gray-600 transition-colors";
  return (
    <div className="relative mt-3">
      {/* pt-10 keeps the whole overlay button row in padding space, so even a
          long first line scrolls beneath nothing. */}
      <pre className="rounded-lg bg-gray-900 dark:bg-slate-950 text-gray-100 text-xs p-3 pt-10 overflow-x-auto font-mono leading-relaxed">
        {text}
      </pre>
      <div className="absolute top-2 right-2 flex gap-1.5">
        <button
          type="button"
          onClick={() => onCopy(id, text)}
          className={buttonClass}
          aria-label="Copy the manifest to the clipboard"
          title="Copy the raw manifest"
        >
          {copiedId === id ? "Copied!" : "Copy YAML"}
        </button>
        <button
          type="button"
          onClick={() => onCopy(`${id}-kubectl`, kubectlText)}
          className={buttonClass}
          aria-label="Copy a ready-to-run kubectl apply command to the clipboard"
          title="Copy as kubectl apply command"
        >
          {copiedId === `${id}-kubectl` ? "Copied!" : "Copy kubectl"}
        </button>
      </div>
    </div>
  );
}

function SetupTextInput({ label, value, onChange, width = "w-40" }) {
  return (
    <label className="flex flex-col gap-1 text-xs font-medium text-gray-600 dark:text-slate-300">
      {label}
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={`${width} rounded-md border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-800 px-2 py-1.5 text-sm font-mono text-gray-900 dark:text-slate-100 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary`}
      />
    </label>
  );
}

function SetupBlockedDetail({ detail }) {
  if (!detail) return null;
  return (
    <div className="mt-3 rounded-lg bg-warning/10 border border-warning/30 px-3 py-2 text-sm text-gray-700 dark:text-slate-300 break-words">
      {detail}
    </div>
  );
}

// A done step collapses to its title; the active step and blocked steps show
// their body so the admin always sees exactly one thing to do next.
function SetupStep({ step, index, isLast, expanded, title, doneSummary, children }) {
  return (
    <li className="flex gap-3">
      <div className="flex flex-col items-center">
        <SetupStateIcon state={step.state} index={index} />
        {!isLast && (
          <div className="w-px flex-1 bg-gray-200 dark:bg-slate-700 mt-1" aria-hidden="true"></div>
        )}
      </div>
      <div className={`${isLast ? "pb-1" : "pb-6"} min-w-0 flex-1`}>
        <h4
          className={`text-sm font-semibold leading-7 ${
            step.state === "done"
              ? "text-gray-500 dark:text-slate-400"
              : "text-gray-900 dark:text-slate-100"
          }`}
        >
          {title}
          {step.state === "done" && doneSummary && (
            <span className="ml-2 font-normal text-xs text-gray-400 dark:text-slate-500">
              {doneSummary}
            </span>
          )}
        </h4>
        {expanded && <div className="mt-1">{children}</div>}
      </div>
    </li>
  );
}

function SetupGuide({ setup, jobs, onRunDiscovery, onDismiss }) {
  const { useState, useEffect } = React;

  const [providerKey, setProviderKey] = useState("github");
  // The operator reports its own namespace, so the generated manifests land
  // where it actually runs; the literal is only a fallback for a backend
  // that reports none.
  const [namespace, setNamespace] = useState(setup?.namespace || "renovate-operator");
  const [secretName, setSecretName] = useState("renovate-secret");
  const [jobName, setJobName] = useState("renovate");
  const [schedule, setSchedule] = useState("0 2 * * *");
  const [filter, setFilter] = useState("my-org/*");
  const [copiedId, setCopiedId] = useState(null);
  const [secretCheck, setSecretCheck] = useState(null);

  const provider = SETUP_PROVIDERS.find((p) => p.key === providerKey) || SETUP_PROVIDERS[0];
  const isGithubApp = providerKey === "github-app";

  const steps = Array.isArray(setup?.steps) ? setup.steps : [];
  const stepById = (id) => steps.find((s) => s.id === id) || { id, state: "pending" };
  const jobsExist = Array.isArray(jobs) && jobs.length > 0;

  // Before any RenovateJob exists the backend cannot verify the secret (it
  // does not know which one is meant), but the guide does: it knows the
  // namespace and name the admin typed. Poll for exactly that secret so
  // applying it advances the guide to step 2 without waiting for the job.
  // The polling deliberately keeps running while the backend still reports
  // the step pending — even once jobs exist — because the status is served
  // from a short cache and must not un-tick a step it already confirmed.
  const backendCredentials = stepById("credentials");
  const wantSecretCheck = backendCredentials.state === "pending";
  useEffect(() => {
    if (!wantSecretCheck || !namespace || !secretName) {
      setSecretCheck(null);
      return;
    }
    let cancelled = false;
    const base = window.__BASE_PATH__ || "";
    const check = () => {
      fetch(
        `${base}/api/v1/setup/secret?namespace=${encodeURIComponent(namespace)}&name=${encodeURIComponent(secretName)}`
      )
        .then((res) => (res.ok ? res.json() : null))
        .then((data) => {
          if (!cancelled) setSecretCheck(data);
        })
        .catch(() => {});
    };
    // Debounced first check so typing in the fields does not fire per key,
    // then a 10s poll so an applied secret is picked up promptly.
    const timeout = setTimeout(check, 500);
    const interval = setInterval(check, 10000);
    return () => {
      cancelled = true;
      clearTimeout(timeout);
      clearInterval(interval);
    };
  }, [wantSecretCheck, namespace, secretName]);

  const secretKeysOk =
    !!secretCheck?.found && (isGithubApp ? secretCheck.hasGithubAppKeys : secretCheck.hasToken);
  const credentialsLocallyDone = wantSecretCheck && secretKeysOk;

  // The job list is fetched live while the setup status is served from a
  // short server-side cache. Right after the admin applies the RenovateJob
  // the two disagree for up to a cache period, and without these overrides
  // the guide would briefly reopen every step it had already ticked off.
  const effectiveSteps = steps.map((s) => {
    if (s.id === "credentials" && credentialsLocallyDone) {
      return { ...s, state: "done" };
    }
    if (s.id === "renovatejob" && s.state === "pending" && jobsExist) {
      return { ...s, state: "done" };
    }
    return s;
  });

  const doneCount = effectiveSteps.filter((s) => s.state === "done").length;
  const activeId = (effectiveSteps.find((s) => s.state !== "done") || {}).id;
  const stepProps = (id) => {
    const step = effectiveSteps.find((s) => s.id === id) || { id, state: "pending" };
    return {
      step,
      index: effectiveSteps.findIndex((s) => s.id === id) + 1,
      isLast: effectiveSteps.length > 0 && effectiveSteps[effectiveSteps.length - 1].id === id,
      expanded: step.state === "blocked" || id === activeId,
    };
  };

  const firstJob = Array.isArray(jobs) && jobs.length > 0 ? jobs[0] : null;
  const canRunDiscovery =
    firstJob && Array.isArray(firstJob.permissions) && firstJob.permissions.includes("discovery");

  const copyText = (id, text) => {
    const done = () => {
      setCopiedId(id);
      setTimeout(() => setCopiedId((current) => (current === id ? null : current)), 2000);
    };
    try {
      navigator.clipboard.writeText(text).then(done, () => {});
    } catch {
      // Clipboard API unavailable (plain http): the manual path still works.
    }
  };

  const secretYaml = () => {
    if (isGithubApp) {
      return [
        "apiVersion: v1",
        "kind: Secret",
        "metadata:",
        `  name: ${secretName}`,
        `  namespace: ${namespace}`,
        "  labels:",
        "    # Required once the policy engine is enabled: it marks this secret",
        "    # as intentionally referenced at caller-chosen keys.",
        '    renovate-operator.mogenius.com/allow-ref: "true"',
        "stringData:",
        '  APP_ID: "<your-app-id>"',
        '  INSTALL_ID: "<your-installation-id>"',
        "  PEM: |",
        "    <your-private-key>",
      ].join("\n");
    }
    const lines = [
      "apiVersion: v1",
      "kind: Secret",
      "metadata:",
      `  name: ${secretName}`,
      `  namespace: ${namespace}`,
      "stringData:",
    ];
    if (providerKey === "github") {
      lines.push(
        '  GITHUB_COM_USER: "<your-github-username>"',
        '  GITHUB_COM_TOKEN: "<your-github-pat>"',
        '  RENOVATE_TOKEN: "<your-github-pat>"'
      );
    } else {
      lines.push(`  RENOVATE_TOKEN: "<your-${provider.provider}-token>"`);
    }
    return lines.join("\n");
  };

  const jobYaml = () => {
    const lines = [
      "apiVersion: renovate-operator.mogenius.com/v1alpha1",
      "kind: RenovateJob",
      "metadata:",
      `  name: ${jobName}`,
      `  namespace: ${namespace}`,
      "spec:",
      `  schedule: "${schedule}"`,
    ];
    if (isGithubApp) {
      lines.push(
        "  githubAppReference:",
        `    secretName: ${secretName}`,
        "    appIdSecretKey: APP_ID",
        "    installationIdSecretKey: INSTALL_ID",
        "    pemSecretKey: PEM"
      );
    } else {
      lines.push(`  secretRef: ${secretName}`);
    }
    lines.push(
      "  provider:",
      `    name: ${provider.provider}`,
      "  image: ghcr.io/renovatebot/renovate:latest",
      "  parallelism: 3",
      "  discoveryFilters:",
      `    - "${filter}"`
    );
    return lines.join("\n");
  };

  const hints = Array.isArray(setup?.hints) ? setup.hints : [];
  const openHints = hints.filter((h) => !h.done && SETUP_HINTS[h.id]);

  const acceptedStep = stepById("accepted");
  const discoveryStep = stepById("discovery");
  // Two sources say "busy": the backend's own step detail, and the optimistic
  // flag the click sets — the latter covers the seconds until the next status
  // poll confirms the run, so the button never looks idle mid-launch.
  const discoveryRunning =
    discoveryStep.detail === "discovery is running" || !!firstJob?.discoveryRunning;

  return (
    <div
      data-testid="setup-guide"
      className="mb-6 bg-white dark:bg-slate-800 border border-gray-200 dark:border-slate-700 rounded-lg shadow-sm overflow-hidden"
    >
      <div className="px-4 sm:px-6 py-4 border-b border-gray-200 dark:border-slate-700 flex items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-slate-100">
            Set up Renovate Operator
          </h2>
          <p className="text-sm text-gray-500 dark:text-slate-400 mt-0.5">
            {doneCount} of {steps.length} steps done — this guide updates itself as you apply
            resources to the cluster.
          </p>
        </div>
        <button
          type="button"
          onClick={onDismiss}
          className="text-xs text-gray-400 dark:text-slate-500 hover:text-gray-600 dark:hover:text-slate-300 transition-colors whitespace-nowrap"
        >
          Skip guide
        </button>
      </div>

      <div className="px-4 sm:px-6 py-5 grid gap-8 lg:grid-cols-[1fr_20rem]">
        <ol className="min-w-0">
          <SetupStep
            {...stepProps("credentials")}
            title="Create a credentials Secret"
            doneSummary={
              !credentialsLocallyDone
                ? "credentials verified"
                : isGithubApp && !secretCheck?.hasAllowRefLabel
                ? "secret found (allow-ref label missing)"
                : "secret found"
            }
          >
            <p className="text-sm text-gray-600 dark:text-slate-400">
              The operator reads platform credentials from a Secret in the RenovateJob's
              namespace. Pick your platform:
            </p>
            <div className="flex flex-wrap gap-1.5 mt-2">
              {SETUP_PROVIDERS.map((p) => (
                <button
                  key={p.key}
                  type="button"
                  onClick={() => setProviderKey(p.key)}
                  aria-pressed={providerKey === p.key}
                  className={`px-2.5 py-1 rounded-full text-xs font-medium border transition-colors ${
                    providerKey === p.key
                      ? "border-primary bg-primary/10 text-primary"
                      : "border-gray-300 dark:border-slate-600 text-gray-600 dark:text-slate-300 hover:border-primary/50"
                  }`}
                >
                  {p.label}
                </button>
              ))}
            </div>
            <div className="flex flex-wrap gap-3 mt-3">
              <SetupTextInput label="Namespace" value={namespace} onChange={setNamespace} />
              <SetupTextInput label="Secret name" value={secretName} onChange={setSecretName} />
            </div>
            {isGithubApp && (
              <p className="text-xs text-gray-500 dark:text-slate-400 mt-2">
                No GitHub App yet? Follow the{" "}
                <a
                  href={`${SETUP_DOCS_BASE}/platforms/github-app-setup.md`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-primary hover:underline"
                >
                  GitHub App setup guide
                </a>{" "}
                first.
              </p>
            )}
            <SetupYamlBlock id="secret-yaml" text={secretYaml()} copiedId={copiedId} onCopy={copyText} />
            <p className="text-xs text-gray-500 dark:text-slate-400 mt-2">
              Fill in the placeholders, then apply it — <em>Copy kubectl</em> gives you a
              ready-to-paste <code className="font-mono">kubectl apply</code> command. Other
              platforms and token variables:{" "}
              <a
                href={`${SETUP_DOCS_BASE}/platforms`}
                target="_blank"
                rel="noopener noreferrer"
                className="text-primary hover:underline"
              >
                platform guides
              </a>
              . The guide detects the secret automatically once it exists.
            </p>
            <SetupBlockedDetail
              detail={
                backendCredentials.detail ||
                (wantSecretCheck && secretCheck?.found && !secretKeysOk
                  ? isGithubApp
                    ? `Secret "${secretName}" exists, but is missing APP_ID, INSTALL_ID or PEM.`
                    : `Secret "${secretName}" exists, but holds no platform token (expected one of RENOVATE_TOKEN, GITHUB_COM_TOKEN, GITLAB_TOKEN, BITBUCKET_TOKEN, GITEA_TOKEN, FORGEJO_TOKEN).`
                  : "")
              }
            />
          </SetupStep>

          <SetupStep {...stepProps("renovatejob")} title="Create your first RenovateJob" doneSummary="created">
            <p className="text-sm text-gray-600 dark:text-slate-400">
              The RenovateJob tells the operator what to discover and when to run.
            </p>
            <div className="flex flex-wrap gap-3 mt-3">
              <SetupTextInput label="Job name" value={jobName} onChange={setJobName} />
              <SetupTextInput label="Cron schedule" value={schedule} onChange={setSchedule} />
              <SetupTextInput label="Discovery filter" value={filter} onChange={setFilter} width="w-48" />
            </div>
            <SetupYamlBlock id="job-yaml" text={jobYaml()} copiedId={copiedId} onCopy={copyText} />
            <p className="text-xs text-gray-500 dark:text-slate-400 mt-2">
              Apply it with <em>Copy kubectl</em>, or save it as{" "}
              <code className="font-mono">renovatejob.yaml</code> for your GitOps repo. The filter
              limits autodiscovery to your organisation — without one, every repository the token
              can see is discovered.
            </p>
          </SetupStep>

          <SetupStep {...stepProps("accepted")} title="Job accepted by the operator" doneSummary="accepted">
            <p className="text-sm text-gray-600 dark:text-slate-400">
              {acceptedStep.state === "blocked"
                ? "The operator's policy refused the job. Fix the value named below and the operator re-checks automatically."
                : "Checked automatically once the RenovateJob exists."}
            </p>
            <SetupBlockedDetail detail={acceptedStep.detail} />
          </SetupStep>

          <SetupStep
            {...stepProps("discovery")}
            title="Discover your repositories"
            doneSummary={discoveryStep.detail || "done"}
          >
            <p className="text-sm text-gray-600 dark:text-slate-400">
              {discoveryRunning
                ? "Discovery is running — this step ticks off as soon as the operator reports the first repository."
                : "Discovery runs on the next cron tick, or start it now:"}
            </p>
            {canRunDiscovery && (
              <button
                type="button"
                onClick={() => onRunDiscovery(firstJob)}
                disabled={discoveryRunning}
                className="mt-3 bg-primary hover:bg-primary-hover disabled:opacity-60 disabled:cursor-not-allowed text-white px-3 py-1.5 rounded-lg font-semibold text-[0.813rem] shadow-sm hover:shadow-md transition-all inline-flex items-center gap-1.5"
                aria-label={discoveryRunning ? "Discovery running" : `Run discovery for ${firstJob.name}`}
              >
                {discoveryRunning && <Spinner />}
                {discoveryRunning ? "Discovery running..." : "Run discovery now"}
              </button>
            )}
          </SetupStep>
        </ol>

        {openHints.length > 0 && (
          <aside className="lg:border-l lg:border-gray-200 lg:dark:border-slate-700 lg:pl-6">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-slate-400 mb-3">
              Recommended next steps
            </h3>
            <ul className="space-y-4">
              {openHints.map((hint) => (
                <li key={hint.id} className="text-sm">
                  <a
                    href={SETUP_HINTS[hint.id].href}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="font-semibold text-primary hover:underline"
                  >
                    {SETUP_HINTS[hint.id].title}
                  </a>
                  <p className="text-xs text-gray-500 dark:text-slate-400 mt-0.5">
                    {SETUP_HINTS[hint.id].body}
                  </p>
                </li>
              ))}
            </ul>
          </aside>
        )}
      </div>
    </div>
  );
}
window.SetupGuide = SetupGuide;
