export function ThemeToggle() {
  const [theme, setTheme] = React.useState(() => window.ThemeUtils.getTheme());

  const cycleTheme = () => {
    const newTheme = window.ThemeUtils.nextTheme(theme);
    setTheme(newTheme);
    window.ThemeUtils.setTheme(newTheme);
    window.ThemeUtils.apply(newTheme);
  };

  React.useEffect(() => {
    window.ThemeUtils.apply(theme);
  }, [theme]);

  React.useEffect(() => {
    const onOSChange = () => {
      if (window.ThemeUtils.getTheme() === 'system') {
        setTheme('system');
      }
    };
    window.addEventListener('themechange', onOSChange);
    return () => window.removeEventListener('themechange', onOSChange);
  }, []);

  const labels = {
    system: 'Theme: system (click for light)',
    light: 'Theme: light (click for dark)',
    dark: 'Theme: dark (click for system)'
  };
  const label = labels[theme] || labels.system;

  return (
    <button
      onClick={cycleTheme}
      className="p-2 rounded-lg border border-gray-300 dark:border-slate-600 hover:bg-gray-100 dark:hover:bg-slate-700 transition-all text-gray-700 dark:text-slate-200"
      aria-label={label}
    >
      {theme === 'light' ? (
        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
        </svg>
      ) : theme === 'dark' ? (
        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
        </svg>
      ) : (
        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
      )}
    </button>
  );
}
