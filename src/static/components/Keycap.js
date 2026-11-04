// The keyboard-shortcut "keycap": a small, square-ish key rendered in the help
// overview and inside tooltips. One component keeps both render sites (and
// their light/dark and on-dark variants) identical.
function Keycap({ children, size = "md", variant = "default", className = "" }) {
  const sizes = {
    sm: "min-w-[1.125rem] h-[1.125rem] px-1 text-[0.6875rem]",
    md: "min-w-[1.5rem] h-6 px-1.5 text-xs",
  };
  const variants = {
    default:
      "border-gray-300 dark:border-slate-600 border-b-gray-300 dark:border-b-slate-600 bg-white dark:bg-slate-800 text-gray-700 dark:text-slate-200",
    onDark:
      "border-white/30 border-b-white/30 bg-white/15 text-white/80",
  };

  return (
    <kbd
      className={`inline-flex items-center justify-center rounded-md border border-b-2 font-mono font-semibold leading-none ${variants[variant]} ${sizes[size]} ${className}`}
    >
      {children}
    </kbd>
  );
}
window.Keycap = Keycap;
