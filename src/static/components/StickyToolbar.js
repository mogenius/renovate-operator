// A bar pinned to the top of the viewport, holding the controls a page must keep
// reachable while its content scrolls — the dashboard's filters and search, the
// logs page's level filter and search. The brand strip above it scrolls away like
// ordinary content.
//
// The bar is sticky at every scroll position; `data-stuck` and the shadow only say
// whether the page has left the brand strip behind yet, so the bar lifts itself off
// the content instead of carrying a shadow while it still sits under the header.
//
// z-30 puts it above the page content and below the full-page overlays that dismiss
// a popover (z-40) — cards < toolbar < overlay < menu. A popover opened from inside
// the bar is positioned against the bar itself and rides along with it.
function StickyToolbar({ testId, children }) {
  const [isStuck, setIsStuck] = React.useState(false);

  React.useEffect(() => {
    const updateStuckState = () => setIsStuck(window.scrollY > 0);
    updateStuckState();
    window.addEventListener("scroll", updateStuckState, { passive: true });
    return () => window.removeEventListener("scroll", updateStuckState);
  }, []);

  return (
    <div
      data-testid={testId}
      data-stuck={isStuck ? "true" : "false"}
      className={`sticky top-0 z-30 bg-white dark:bg-slate-800 border-b border-gray-200 dark:border-slate-700 transition-shadow duration-200 ${
        isStuck ? "shadow-md" : ""
      }`}
    >
      <div className="max-w-7xl mx-auto w-full px-3 sm:px-6 lg:px-8 py-2 sm:py-3 flex flex-col gap-2">
        {children}
      </div>
    </div>
  );
}
window.StickyToolbar = StickyToolbar;
