export function SiteHeader({ version, authInfo, children }) {
  return (
    <header className="bg-white dark:bg-slate-800 border-b border-gray-200 dark:border-slate-700 mb-4 sm:mb-6 transition-colors duration-200">
      <div className="max-w-7xl mx-auto px-3 sm:px-6 lg:px-8 py-4 sm:py-6">
        <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4 sm:gap-6">
          <div className="flex items-center justify-between md:justify-start gap-3">
            <div className="flex flex-col items-start gap-2 sm:gap-3">
              <img
                src="/assets/logo.png"
                alt="Renovate Operator Logo"
                className="w-32 h-16 sm:w-40 sm:h-20 object-contain dark:brightness-0 dark:invert"
              />
              <div className="flex items-center gap-2">
                <div className="w-1 h-5 sm:h-6 bg-gradient-to-b from-primary to-primary-hover rounded-full"></div>
                <p className="text-gray-700 dark:text-slate-300 font-medium text-sm sm:text-base tracking-wide whitespace-nowrap">
                  Renovate: The{" "}
                  <span className="text-primary font-semibold">
                    Kubernetes-Native
                  </span>{" "}
                  Way
                </p>
                {version && (
                  <span className="text-xs text-gray-400 dark:text-slate-500 font-mono ml-1">
                    v{version}
                  </span>
                )}
              </div>
            </div>
            <div className="md:hidden">
              <ThemeToggle />
            </div>
          </div>

          {children && (
            <div className="flex flex-wrap items-center gap-2">
              {children}
            </div>
          )}

          <div className="hidden md:flex flex-col items-end gap-2 shrink-0">
            <ThemeToggle />
            {authInfo && authInfo.enabled && authInfo.authenticated && (
              <div className="flex items-center gap-2">
                <span className="text-xs sm:text-sm text-gray-600 dark:text-slate-300 truncate max-w-[150px]" title={authInfo.email}>
                  {authInfo.name || authInfo.email}
                </span>
                <a
                  href="/auth/logout"
                  className="px-2 py-1.5 sm:px-3 sm:py-2 rounded-lg border border-gray-300 dark:border-slate-600 hover:bg-gray-100 dark:hover:bg-slate-700 transition-all text-gray-700 dark:text-slate-200 text-xs sm:text-sm font-medium"
                >
                  Logout
                </a>
              </div>
            )}
          </div>
        </div>
      </div>
    </header>
  );
}
