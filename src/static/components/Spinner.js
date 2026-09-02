// The one spinner in the UI. Every button that stays disabled while an
// operation runs — discovery, trigger, trigger-all, cancel — renders this in
// place of its idle icon, so "busy" looks the same everywhere.
//
// currentColor and a caller-supplied size keep it usable inside a button
// (w-3.5) and on a full-page loading state (w-5) alike.
//
// Decorative on purpose: every call site already states the busy state in
// visible text or the button's aria-label, so announcing the spinner too
// would read it out twice.
function Spinner({ className = "w-3.5 h-3.5" }) {
  return (
    <svg
      className={`animate-spin flex-shrink-0 ${className}`}
      fill="none"
      viewBox="0 0 24 24"
      aria-hidden="true"
    >
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8z" />
    </svg>
  );
}
window.Spinner = Spinner;
