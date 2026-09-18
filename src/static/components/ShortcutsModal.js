function ShortcutsModal({ onClose, shortcuts }) {
  const { useEffect } = React;

  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="bg-white dark:bg-slate-800 rounded-xl shadow-2xl border border-gray-200 dark:border-slate-600 p-5 max-w-xs w-full mx-4"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label="Keyboard shortcuts"
      >
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Keyboard shortcuts</h2>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600 dark:hover:text-slate-300 transition-colors"
            aria-label="Close shortcuts"
          >
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
            </svg>
          </button>
        </div>
        <div className="space-y-2.5">
          {shortcuts.map(([keys, desc]) => (
            <div key={desc} className="flex items-center justify-between gap-6">
              <span className="text-xs text-gray-600 dark:text-slate-400">{desc}</span>
              <div className="flex items-center gap-1 shrink-0">
                {keys.map((k, i) => (
                  <React.Fragment key={k}>
                    {i > 0 && <span className="text-[0.6rem] text-gray-400 dark:text-slate-500 mx-0.5">or</span>}
                    <kbd className="px-1.5 py-0.5 text-xs font-mono font-semibold rounded border border-gray-300 dark:border-slate-600 bg-gray-100 dark:bg-slate-700 text-gray-700 dark:text-slate-300">{k}</kbd>
                  </React.Fragment>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
window.ShortcutsModal = ShortcutsModal;
