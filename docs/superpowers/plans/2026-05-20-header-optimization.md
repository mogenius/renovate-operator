# Header optimization implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure `<SiteHeader>` into a brand strip (logo, tagline, version, auth, theme toggle) and a stats strip (the 8 StatBadge children), so mobile gets two clean rows in the brand strip and `sm:` and up collapses everything inline.

**Architecture:** Single component (`src/static/components/SiteHeader.js`). The `<header>` shell stays. Inside, two stacked `<div>` strips replace the current single flex row. Mobile uses `flex-wrap` + `order-*` + `basis-full` on the middle (tagline) group so it falls to row 2 below the logo+controls row. From `sm:` upward, `sm:flex-1` and reset order put everything inline.

**Tech Stack:** Babel-standalone JSX served from disk, Tailwind utility classes, no build step. Mounted live into the Docker preview at http://localhost:8080 via volume mount (no rebuild required after edits).

---

## File structure

- **Modify** `src/static/components/SiteHeader.js` — full component rewrite. Single file, single responsibility, no new files.

The component's exported signature stays identical: `SiteHeader({ version, authInfo, children })`. Callers in `src/static/index.html` are untouched.

---

## Task 1: Replace SiteHeader with the two-band structure

**Files:**
- Modify: `src/static/components/SiteHeader.js` (full rewrite, ~70 lines)

- [ ] **Step 1: Confirm working state**

Run:
```bash
cd /Users/dtaege/Projects/meta/tmp/renovate-operator
git status --short
git branch --show-current
```

Expected:
- Branch: `fix/dashboard-horizontal-scroll`
- Only untracked: `tmp/`
- No modified files (the spec commit `1dbec53` is the latest)

If anything else is modified, stop and reconcile before proceeding.

- [ ] **Step 2: Snapshot the current state for the comparison page**

Run:
```bash
cp src/static/components/SiteHeader.js tmp/preview/snapshots/SiteHeader.before.js
```

This is for the comparison page generated in Task 3. The file is gitignored under `tmp/`.

- [ ] **Step 3: Rewrite `src/static/components/SiteHeader.js`**

Write the file with the following exact contents:

```jsx
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
            <div className="flex flex-wrap items-center gap-2">
              {children}
            </div>
          </div>
        )}
      </div>
    </header>
  );
}
```

Notes embedded in the code:
- `order-2 sm:order-3` on the right cluster: lower order number on mobile so the cluster sits at the end of row 1 (after the logo, before the basis-full tagline group wraps it down).
- `order-3 sm:order-2` on the tagline group: higher order number on mobile (= last item, gets wrapped to row 2 by `basis-full`).
- `ml-auto sm:ml-0` on the right cluster: pushes it to the right edge on mobile, neutral from `sm:` upward.
- Logo `h-10` on mobile is the spec-mandated smaller height (was `h-16`).

- [ ] **Step 4: Reload the running preview and check for console errors**

The Docker preview container `renovate-preview` mounts `src/static/` read-only, so saving the file is enough. Browser auto-reload via cache-bust is needed.

In the Chrome DevTools MCP page, navigate to `http://localhost:8080/` and confirm:
- React mounts (the logo, tagline, and "gitlab" job card render).
- No JS console errors related to JSX parsing or `ThemeToggle` resolution.

If there's a console error, fix the syntax before proceeding.

- [ ] **Step 5: Commit the structural change**

Run:
```bash
git add src/static/components/SiteHeader.js
git commit -m "$(cat <<'EOF'
refactor(ui): split SiteHeader into brand and stats strips

Two stacked bands replace the previous single flex row:

- Brand strip: logo + tagline + version + auth + theme toggle. On mobile,
  the logo and right cluster (auth, theme) share row 1; the tagline + version
  drop to row 2 via flex-wrap + basis-full. From sm: upward, everything
  collapses inline with the tagline group taking the middle (flex-1).
- Stats strip: the 8 StatBadge children, still wrapping. Separated from the
  brand strip by a subtle border-t.

The mobile-only duplicate ThemeToggle is gone; one toggle now serves all
widths. Component signature (version, authInfo, children) is unchanged.
EOF
)"
```

Expected output: `[fix/dashboard-horizontal-scroll <sha>] refactor(ui): split SiteHeader into brand and stats strips` with `1 file changed`.

---

## Task 2: Visual verification across breakpoints

Use the existing Chrome DevTools MCP page and the running Docker preview.

**Files:**
- Capture: `tmp/preview/snapshots/header-340.png`, `header-640.png`, `header-768.png`, `header-1024.png`, `header-1280.png`, `header-1920.png`

For each width, the acceptance criteria are:

1. `document.documentElement.scrollWidth === document.documentElement.clientWidth` (no horizontal page scroll).
2. Brand strip and stats strip are visually distinct (border-t between them visible in dark theme).
3. Theme toggle sits in the brand strip's right cluster, never floating mid-area.
4. Tagline either fits on one line or wraps only at the space before `Way`.
5. Stats strip still wraps as today, no internal horizontal scroll.

- [ ] **Step 1: Verify and capture 340 px (mobile, < sm)**

Emulate `340x900x1,mobile,touch`. Navigate to `http://localhost:8080/`. Wait 1.5 seconds. Run:

```js
() => ({
  viewport: window.innerWidth,
  docOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
  logoH: document.querySelector('header img').getBoundingClientRect().height,
  themeTopY: document.querySelector('header button[aria-label*="theme" i], header button[title*="theme" i]')?.getBoundingClientRect().top,
  logoTopY: document.querySelector('header img').getBoundingClientRect().top,
  taglineLines: (() => {
    const p = [...document.querySelectorAll('p')].find(x => x.textContent.includes('Kubernetes'));
    return p ? Math.round(p.getBoundingClientRect().height / parseFloat(getComputedStyle(p).lineHeight)) : null;
  })(),
})
```

Expected:
- `docOverflow`: 0
- `logoH`: 40 (h-10)
- `themeTopY` and `logoTopY` within ~10 px of each other (theme toggle vertically aligned with logo)
- `taglineLines`: 1 or 2

Take screenshot to `tmp/preview/snapshots/header-340.png`.

- [ ] **Step 2: Verify and capture 640 px (sm boundary)**

Emulate `640x900x1`. Reload. Run the same evaluator. Expected:
- `docOverflow`: 0
- `logoH`: 64 (h-16) — switched to sm size
- Tagline on one line (`taglineLines`: 1)

Screenshot to `tmp/preview/snapshots/header-640.png`.

- [ ] **Step 3: Verify and capture 768 px (md boundary)**

Emulate `768x900x1`. Reload. Run the evaluator. Expected:
- `docOverflow`: 0
- Tagline on one line

Screenshot to `tmp/preview/snapshots/header-768.png`.

- [ ] **Step 4: Verify and capture 1024 px (lg, table appears)**

Emulate `1024x900x1`. Reload. Click the gitlab card to expand. Run:

```js
() => {
  const t = document.querySelector('table');
  const w = t ? t.parentElement : null;
  return {
    viewport: window.innerWidth,
    docOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    wrapperOverflow: w ? w.scrollWidth - w.clientWidth : null,
  };
}
```

Expected:
- `docOverflow`: 0
- `wrapperOverflow`: 0 (the scroll fix still holds)

Screenshot to `tmp/preview/snapshots/header-1024.png`.

- [ ] **Step 5: Verify and capture 1280 px (xl, full padding)**

Emulate `1280x900x1`. Reload. Expand the card. Same evaluator. Expected: both overflow values are 0.

Screenshot to `tmp/preview/snapshots/header-1280.png`.

- [ ] **Step 6: Verify and capture 1920 px (wide desktop)**

Emulate `1920x900x1`. Reload. Expand. Same evaluator. Expected: both 0.

Screenshot to `tmp/preview/snapshots/header-1920.png`.

- [ ] **Step 7: Stop and inspect**

If any width fails an acceptance criterion, stop here and report the failure. Otherwise, proceed.

---

## Task 3: Build before/after comparison page

**Files:**
- Create: `tmp/preview/snapshots/header-before-*.png` (one per width)
- Create: `tmp/preview/snapshots/comparison-header.html`

- [ ] **Step 1: Swap to the pre-refactor SiteHeader**

The current file on disk reflects Task 1's rewrite. To capture "before" shots, restore the previous version (the state with the scroll fix applied but without the brand/stats split):

```bash
git checkout HEAD~1 -- src/static/components/SiteHeader.js
```

`HEAD~1` is the spec-doc commit (`1dbec53`), which still has the pre-refactor SiteHeader. Verify with:
```bash
git diff src/static/components/SiteHeader.js | head -5
```
Expected: shows the rewrite as a reversal (lots of `-` lines from the new structure).

- [ ] **Step 2: Capture "before" screenshots at the same six widths**

For each width (340 mobile-touch, 640, 768, 1024, 1280, 1920):
- Emulate the viewport.
- Reload `http://localhost:8080/`.
- Expand the gitlab card at widths ≥ 1024.
- Take screenshot to `tmp/preview/snapshots/header-before-<width>.png`.

- [ ] **Step 3: Restore the refactored SiteHeader**

```bash
git checkout fix/dashboard-horizontal-scroll -- src/static/components/SiteHeader.js
```

Verify:
```bash
git diff src/static/components/SiteHeader.js
```
Expected: empty diff (file matches HEAD).

- [ ] **Step 4: Write `tmp/preview/snapshots/comparison-header.html`**

Use the same dark-theme HTML scaffold as `comparison.html`, six rows, left = `header-before-<w>.png`, right = `header-<w>.png`. Reuse the styles by linking to or copying from `comparison.html`.

Minimal contents — adapt the existing `comparison.html` styles, change titles to "Header redesign — before / after", and update image paths. Each row's caption: viewport width, with no overflow numbers (this isn't about overflow, it's about layout).

- [ ] **Step 5: Open the comparison page in the browser and verify each pair loads**

Navigate to `file:///Users/dtaege/Projects/meta/tmp/renovate-operator/tmp/preview/snapshots/comparison-header.html`. Confirm all 12 images render.

---

## Task 4: Hand off to user, do NOT update the PR yet

Per the user's instruction, the PR (`mogenius/renovate-operator#347`) stays unchanged until they've reviewed.

- [ ] **Step 1: Confirm the commit landed locally**

```bash
git log --oneline -3
```
Expected output, top to bottom:
- `<sha> refactor(ui): split SiteHeader into brand and stats strips`
- `1dbec53 docs: add design spec for header optimization`
- `84f23ae fix(ui): remove horizontal scrolling on dashboard`

- [ ] **Step 2: Verify nothing else is staged or modified**

```bash
git status --short
```
Expected: only `?? tmp/` (untracked snapshots and the preview folder).

- [ ] **Step 3: Report to the user**

Report the comparison page path and the commit sha. Wait for explicit instruction before running `git push` or `gh pr edit`. Do not push.

---

## Self-review

Spec coverage check — every section of `2026-05-20-header-optimization-design.md` maps to a task:

- "Problem" / "Goal" — context only, no task needed.
- "Approach" (two stacked bands, faint border-b) — Task 1 Step 3 (new JSX with two `<div>` strips + `border-t` between them).
- "Brand strip behaviour — Below `sm:` (< 640 px)" — Task 1 Step 3 (logo `h-10`, basis-full on tagline group, `whitespace-nowrap` on Kubernetes-Native span).
- "Brand strip behaviour — `sm:` and up" — Task 1 Step 3 (`sm:flex-1` and order resets collapse everything inline; mobile-only ThemeToggle removed).
- "Auth area" — Task 1 Step 3 (auth block conditionally renders inside the right cluster; flex-wrap on the outer brand row lets it drop to its own line if both auth and theme would overflow row 1).
- "Stats strip" — Task 1 Step 3 (children wrapped in their own bordered strip).
- "Files touched" — only `SiteHeader.js`. Confirmed in plan.
- "Testing" — Task 2 covers visual verification at the six widths.
- "Rollout" — Task 4 stops before push.

Placeholder scan — no TBD / TODO / "similar to" / "appropriate". Each step has either a command, JS evaluator, or full JSX block.

Type consistency — single component, no cross-task type references.

---

## Execution handoff

**Plan complete and saved to `docs/superpowers/plans/2026-05-20-header-optimization.md`.**

Per the user's direction ("do it, do not update the PR yet"), executing inline via `superpowers:executing-plans`. Tasks 1–4 are short enough for batched execution with one checkpoint after Task 2 (visual verification) so any breakpoint regression surfaces before the comparison page is built.
