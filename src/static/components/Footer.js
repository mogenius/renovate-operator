export function Footer() {
  return (
    <footer className="border-t border-gray-200 dark:border-slate-700 py-3 px-3 sm:px-6 lg:px-8 transition-colors duration-200">
      <div className="max-w-7xl mx-auto text-center text-xs text-gray-400 dark:text-slate-500">
        Want to contribute? Check us out on{" "}
        <a
          href="https://github.com/mogenius/renovate-operator"
          target="_blank"
          rel="noopener noreferrer"
          className="hover:text-primary transition-colors underline underline-offset-2"
        >
          GitHub
        </a>
      </div>
    </footer>
  );
}
