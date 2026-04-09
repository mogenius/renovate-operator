export function StatBadge({
  label,
  value,
  valueClass = "text-gray-900 dark:text-slate-100",
  borderClass = "border-gray-200 dark:border-slate-600",
  onClick,
  selected = false,
  title,
}) {
  const isInteractive = typeof onClick === "function";
  const className = `flex items-center justify-between gap-1.5 min-w-[7rem] rounded-lg border px-3 py-1.5 shadow-sm transition-all ${borderClass} ${
    isInteractive
      ? "bg-white dark:bg-slate-700 hover:shadow-md hover:-translate-y-0.5 cursor-pointer focus:outline-none focus:ring-2 focus:ring-primary/40"
      : "bg-white dark:bg-slate-700 hover:shadow-md"
  } ${
    selected
      ? "border-primary bg-primary/5 dark:bg-primary/10 ring-2 ring-primary/30"
      : ""
  }`;

  if (isInteractive) {
    return (
      <button
        type="button"
        onClick={onClick}
        aria-pressed={selected}
        title={title || `Filter jobs by ${label}`}
        className={className}
      >
        <span className="text-[0.625rem] text-gray-500 dark:text-slate-400 uppercase tracking-wider whitespace-nowrap">{label}</span>
        <span className={`text-sm font-bold ${valueClass} tabular-nums`}>{value}</span>
      </button>
    );
  }

  return (
    <div className={className} title={title}>
      <span className="text-[0.625rem] text-gray-500 dark:text-slate-400 uppercase tracking-wider whitespace-nowrap">{label}</span>
      <span className={`text-sm font-bold ${valueClass} tabular-nums`}>{value}</span>
    </div>
  );
}
