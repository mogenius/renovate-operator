export function StatBadge({
  label,
  value,
  valueClass = "text-gray-900 dark:text-slate-100",
  borderClass = "border-gray-200 dark:border-slate-600",
  onClick,
  active = false,
  activeBorderClass = "ring-2 ring-blue-500 border-blue-500 dark:ring-blue-400 dark:border-blue-400",
}) {
  const clickable = typeof onClick === "function";
  const handleKeyDown = clickable
    ? (e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onClick(e);
        }
      }
    : undefined;
  const interactiveProps = clickable
    ? {
        role: "button",
        tabIndex: 0,
        onClick,
        onKeyDown: handleKeyDown,
        "aria-pressed": active,
      }
    : {};
  const effectiveBorder = active ? activeBorderClass : borderClass;
  const cursorClass = clickable ? "cursor-pointer select-none" : "";
  return (
    <div
      {...interactiveProps}
      className={`flex items-center justify-between gap-1.5 min-w-[7rem] bg-white dark:bg-slate-700 rounded-lg border ${effectiveBorder} px-3 py-1.5 shadow-sm hover:shadow-md transition-all ${cursorClass}`}
    >
      <span className="text-[0.625rem] text-gray-500 dark:text-slate-400 uppercase tracking-wider whitespace-nowrap">{label}</span>
      <span className={`text-sm font-bold ${valueClass} tabular-nums`}>{value}</span>
    </div>
  );
}
