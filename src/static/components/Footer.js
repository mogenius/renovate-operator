// Shared footer for every page: a single slim bar on the header's surface.
// Brand on the left, documentation quick links with icons in the middle,
// icon-only project links on the right. The docs links are static and
// absolute (the documentation is not shipped with the operator) and cover
// the topics the setup guide's dynamic hints promote — auth, policy, log
// storage — so they stay discoverable after the guide is gone without
// leaking any of this install's configuration state.
const FOOTER_REPO = "https://github.com/mogenius/renovate-operator";
const FOOTER_DOCS = `${FOOTER_REPO}/blob/main/docs`;

// Heroicons outline paths, matching the icon style used across the app.
const FOOTER_DOC_LINKS = [
  {
    label: "Getting Started",
    href: `${FOOTER_DOCS}/getting-started.md`,
    icon: "M13 10V3L4 14h7v7l9-11h-7z",
  },
  {
    label: "Platforms",
    href: `${FOOTER_REPO}/tree/main/docs/platforms`,
    icon: "M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253",
  },
  {
    label: "Authentication",
    href: `${FOOTER_DOCS}/configuration/auth.md`,
    icon: "M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z",
  },
  {
    label: "Security & Policy",
    href: `${FOOTER_DOCS}/security/security.md`,
    icon: "M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z",
  },
  {
    label: "Webhooks",
    href: `${FOOTER_REPO}/tree/main/docs/webhooks`,
    icon: "M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1",
  },
  {
    label: "Operations",
    href: `${FOOTER_REPO}/tree/main/docs/operations`,
    icon: "M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z",
  },
];

const FOOTER_PROJECT_LINKS = [
  {
    label: "GitHub",
    href: FOOTER_REPO,
    fill: true,
    icon: "M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.203 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z",
  },
  {
    label: "Issues",
    href: `${FOOTER_REPO}/issues`,
    icon: "M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z",
  },
  {
    label: "Releases",
    href: `${FOOTER_REPO}/releases`,
    icon: "M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-5 5a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 10V5a2 2 0 012-2z",
  },
];

function Footer() {
  const base = window.__BASE_PATH__ || "";

  return (
    <footer className="bg-white dark:bg-slate-800 border-t border-gray-200 dark:border-slate-700 transition-colors duration-200">
      <div className="max-w-7xl mx-auto px-3 sm:px-6 lg:px-8 py-3">
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
          <a
            href={`${base}/`}
            className="flex items-center gap-2 shrink-0"
            aria-label="Renovate Operator dashboard"
          >
            <div className="w-1 h-5 bg-gradient-to-b from-primary to-primary-hover rounded-full" aria-hidden="true"></div>
            <span className="text-sm font-bold text-gray-900 dark:text-slate-100 whitespace-nowrap">
              Renovate Operator
            </span>
          </a>

          <nav
            aria-label="Documentation"
            className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-gray-500 dark:text-slate-400"
          >
            {FOOTER_DOC_LINKS.map((link) => (
              <a
                key={link.label}
                href={link.href}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 hover:text-primary transition-colors whitespace-nowrap"
              >
                <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d={link.icon} />
                </svg>
                {link.label}
              </a>
            ))}
          </nav>

          <nav aria-label="Project" className="ml-auto flex items-center gap-1">
            {FOOTER_PROJECT_LINKS.map((link) => (
              <a
                key={link.label}
                href={link.href}
                target="_blank"
                rel="noopener noreferrer"
                title={link.label}
                aria-label={link.label}
                className="p-1.5 rounded-lg text-gray-400 dark:text-slate-500 hover:text-primary hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
              >
                {link.fill ? (
                  <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                    <path fillRule="evenodd" clipRule="evenodd" d={link.icon} />
                  </svg>
                ) : (
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d={link.icon} />
                  </svg>
                )}
              </a>
            ))}
          </nav>
        </div>
      </div>
    </footer>
  );
}
window.Footer = Footer;
