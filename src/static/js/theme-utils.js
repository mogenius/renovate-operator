(function () {
  'use strict';

  window.ThemeUtils = {
    getTheme: () => {
      try {
        return localStorage.getItem('theme') || 'system';
      } catch {
        return 'system';
      }
    },
    setTheme: (theme) => {
      try {
        localStorage.setItem('theme', theme);
      } catch (e) {
        console.warn('Failed to save theme:', e);
      }
    },
    isDark: (theme) => {
      return theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
    },
    apply: (theme) => {
      document.documentElement.classList.toggle('dark', window.ThemeUtils.isDark(theme));
    },
    nextTheme: (theme) => {
      return theme === 'system' ? 'light' : theme === 'light' ? 'dark' : 'system';
    }
  };

  window.ThemeUtils.apply(window.ThemeUtils.getTheme());

  // Keep the UI in sync with OS theme changes while 'system' is selected.
  try {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)');
    const onOSThemeChange = () => {
      if (window.ThemeUtils.getTheme() === 'system') {
        window.ThemeUtils.apply('system');
        window.dispatchEvent(new Event('themechange'));
      }
    };
    if (prefersDark.addEventListener) {
      prefersDark.addEventListener('change', onOSThemeChange);
    } else if (prefersDark.addListener) {
      prefersDark.addListener(onOSThemeChange);
    }
  } catch (e) {
    console.warn('Failed to bind OS theme listener:', e);
  }
})();
