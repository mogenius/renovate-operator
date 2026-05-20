export function SiteHeader({ version, authInfo, children }) {
  const showAuth = authInfo && authInfo.enabled && authInfo.authenticated;

  const authBlock = showAuth && (
    <div className="flex items-center gap-2">
      <span
        className="text-xs sm:text-sm text-gray-600 dark:text-slate-300 truncate max-w-[120px] sm:max-w-[150px]"
        title={authInfo.email}
      >
        {authInfo.name || authInfo.email}
      </span>
      <a
        href="/auth/logout"
        className="px-2 py-1.5 sm:px-3 sm:py-2 rounded-lg border border-gray-300 dark:border-slate-600 hover:bg-gray-100 dark:hover:bg-slate-700 transition-all text-gray-700 dark:text-slate-200 text-xs sm:text-sm font-medium"
      >
        Logout
      </a>
    </div>
  );

  return (
    <header className="bg-white dark:bg-slate-800 border-b border-gray-200 dark:border-slate-700 mb-4 sm:mb-6 transition-colors duration-200">
      <div className="max-w-7xl mx-auto">
        {/* Brand strip */}
        <div className="px-3 sm:px-6 lg:px-8 py-4 sm:py-5">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <a href="/" className="shrink-0">
              <img
                src="/assets/logo.png"
                alt="Renovate Operator Logo"
                className="h-10 sm:h-16 lg:h-20 w-auto object-contain dark:brightness-0 dark:invert"
              />
            </a>

            {/* Tagline group — on mobile this drops to its own row (basis-full),
                from sm: up it sits inline between logo and right cluster (flex-1) */}
            <div className="order-3 sm:order-2 basis-full sm:basis-auto sm:flex-1 flex flex-wrap items-center justify-between gap-x-2 gap-y-1 min-w-0">
              <div className="flex items-center gap-2 min-w-0">
                <div className="w-1 h-5 sm:h-6 bg-gradient-to-b from-primary to-primary-hover rounded-full flex-shrink-0"></div>
                <p className="text-gray-700 dark:text-slate-300 font-medium text-sm sm:text-base tracking-wide">
                  Renovate: The{" "}
                  <span className="text-primary font-semibold whitespace-nowrap">
                    Kubernetes-Native
                  </span>{" "}
                  Way
                </p>
              </div>
              {version && (
                <span className="text-xs text-gray-400 dark:text-slate-500 font-mono">
                  v{version}
                </span>
              )}
            </div>

            {/* Right cluster — on mobile sits at row 1 right via ml-auto + order-2;
                from sm: it slides to the end of the inline row */}
            <div className="order-2 sm:order-3 ml-auto sm:ml-0 flex items-center gap-2 shrink-0">
              {authBlock}
              <ThemeToggle />
            </div>
          </div>
        </div>

        {/* Stats strip */}
        {children && (
          <div className="px-3 sm:px-6 lg:px-8 py-3 border-t border-gray-200 dark:border-slate-700">
            <div className="grid grid-cols-2 sm:flex sm:flex-wrap items-center gap-2">
              {children}
            </div>
          </div>
        )}
      </div>
    </header>
  );
}
