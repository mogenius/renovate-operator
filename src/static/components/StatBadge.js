export function StatBadge({ label, value, valueClass = "text-gray-900 dark:text-slate-100", borderClass = "border-gray-200 dark:border-slate-600" }) {
  return (
    <div className={`flex items-center justify-between gap-1.5 min-w-[7rem] bg-white dark:bg-slate-700 rounded-lg border ${borderClass} px-3 py-1.5 shadow-sm hover:shadow-md transition-all`}>
      <span className="text-[0.625rem] text-gray-500 dark:text-slate-400 uppercase tracking-wider whitespace-nowrap">{label}</span>
      <span className={`text-sm font-bold ${valueClass} tabular-nums`}>{value}</span>
    </div>
  );
}
