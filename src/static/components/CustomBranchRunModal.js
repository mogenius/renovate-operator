// Mirrors validateBaseBranch in src/ui/executionOptions.go; the server is authoritative,
// this only spares the user a round trip for an obviously invalid name.
function customBranchNameProblem(branch) {
  if (branch.length > 250) return "At most 250 characters.";
  if (!/^[A-Za-z0-9][A-Za-z0-9._/-]*$/.test(branch)) {
    return "Start with a letter or digit; use only letters, digits, '.', '_', '-' and '/'.";
  }
  for (const forbidden of ["..", "//", "/.", ".lock/"]) {
    if (branch.includes(forbidden)) return `Must not contain "${forbidden}".`;
  }
  for (const forbiddenSuffix of ["/", ".", ".lock"]) {
    if (branch.endsWith(forbiddenSuffix)) return `Must not end with "${forbiddenSuffix}".`;
  }
  return null;
}

function CustomBranchRunModal({ projectName, onSubmit, onClose }) {
  const { useEffect, useState } = React;
  const [branch, setBranch] = useState("");
  const [dryRun, setDryRun] = useState(false);
  const [debug, setDebug] = useState(false);

  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onClose]);

  const trimmedBranch = branch.trim();
  const branchProblem = trimmedBranch ? customBranchNameProblem(trimmedBranch) : null;
  const canSubmit = trimmedBranch !== "" && !branchProblem;

  const submit = (e) => {
    e.preventDefault();
    if (!canSubmit) return;
    const executionOptions = { baseBranch: trimmedBranch };
    if (dryRun) executionOptions.dryRun = true;
    if (debug) executionOptions.debug = true;
    onSubmit(executionOptions);
  };

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 backdrop-blur-sm"
      onClick={onClose}
    >
      <form
        className="bg-white dark:bg-slate-800 rounded-xl shadow-2xl border border-gray-200 dark:border-slate-600 p-5 max-w-md w-full mx-4"
        onClick={(e) => e.stopPropagation()}
        onSubmit={submit}
        role="dialog"
        aria-modal="true"
        aria-label="Run with custom branch"
      >
        <div className="flex items-center justify-between mb-1">
          <h2 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Run with custom branch</h2>
          <button
            type="button"
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600 dark:hover:text-slate-300 transition-colors"
            aria-label="Close custom branch run"
          >
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
            </svg>
          </button>
        </div>
        <p className="text-xs text-gray-600 dark:text-slate-400 mb-4">
          One run of <span className="font-mono break-all">{projectName}</span> against this branch, using the Renovate config on it. Nothing is saved for later runs.
        </p>

        <label className="block text-xs font-medium text-gray-700 dark:text-slate-300 mb-1" htmlFor="custom-branch-name">
          Branch
        </label>
        <input
          id="custom-branch-name"
          type="text"
          autoFocus
          autoComplete="off"
          spellCheck={false}
          value={branch}
          onChange={(e) => setBranch(e.target.value)}
          placeholder="e.g. test/renovate-config"
          aria-invalid={!!branchProblem}
          aria-describedby="custom-branch-name-problem"
          className="w-full px-2.5 py-1.5 text-sm font-mono rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-900 text-gray-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-primary"
        />
        <p id="custom-branch-name-problem" className="min-h-[1rem] mt-1 text-xs text-error">
          {branchProblem || ""}
        </p>

        <label className="flex items-start gap-2 mt-2 text-xs text-gray-700 dark:text-slate-300">
          <input type="checkbox" checked={dryRun} onChange={(e) => setDryRun(e.target.checked)} className="mt-0.5" />
          <span>Dry run: only log what would change, open no pull requests</span>
        </label>
        <label className="flex items-start gap-2 mt-2 text-xs text-gray-700 dark:text-slate-300">
          <input type="checkbox" checked={debug} onChange={(e) => setDebug(e.target.checked)} className="mt-0.5" />
          <span>Debug log level</span>
        </label>
        {!dryRun && (
          <p className="mt-3 text-xs text-amber-700 dark:text-amber-400">
            Without dry run, Renovate opens and updates real pull requests that target this branch.
          </p>
        )}

        <div className="flex justify-end gap-2 mt-4">
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 text-[0.813rem] rounded-lg text-gray-700 dark:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={!canSubmit}
            className="bg-primary hover:bg-primary-hover disabled:opacity-60 disabled:cursor-not-allowed text-white px-3 py-1.5 rounded-lg font-semibold text-[0.813rem] shadow-sm transition-all"
          >
            Run
          </button>
        </div>
      </form>
    </div>
  );
}
window.CustomBranchRunModal = CustomBranchRunModal;
