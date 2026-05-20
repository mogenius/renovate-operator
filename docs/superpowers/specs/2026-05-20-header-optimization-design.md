# Header optimization — design

Status: approved
Date: 2026-05-20
Author: derdiggn (with brainstorming assist)
Scope: `src/static/components/SiteHeader.js`

## Problem

The current SiteHeader stacks logo + tagline on the left, stat badges in the middle,
and theme toggle + auth on the right, all in a single flex row at `md:` and up. On
mobile it stacks vertically: logo on top, then a tall column of tagline → version →
stat badges with the theme toggle floating mid-column.

That layout causes three concrete issues on small screens:

- The tagline + version row competes with the theme toggle for vertical alignment,
  and the toggle ends up floating opposite the tagline rather than next to the logo.
- The version badge crowds the tagline horizontally and wraps awkwardly at
  constrained desktop widths (around 1280–1380 px).
- The eight stat badges, the brand, and the utility controls all live in the same
  flex row, so visual hierarchy collapses — everything looks like a peer.

## Goal

Restructure the header into two clearly separated bands so brand and utility live
in one, and the dashboard summary lives in the other. Both bands stack on every
viewport; only their internal layout reflows.

## Approach

The `<SiteHeader>` renders two stacked sub-bands inside the same `<header>` shell:

```
<header>
  ┌── brand strip ────────────────────────────────────┐
  │ logo  ...  [auth · theme]                         │  row 1
  │ tagline · Kubernetes-Native · Way  ·  v4.8.0      │  row 2 (mobile)
  ├───────────────────────────────────────────────────┤
  │ stats strip                                       │
  │ [ALL][FAILED][SCHEDULED][RUNNING][COMPLETED]…     │
  └───────────────────────────────────────────────────┘
```

A faint `border-b` separates the strips so they read as distinct without adding
visual weight.

## Brand strip behaviour

### Below `sm:` (< 640 px) — two rows

- Row 1: logo (`h-10`, smaller than the current `h-16`) on the left; theme toggle
  aligned to the right of the same row, vertically centred with the logo.
- Row 2: accent bar + tagline left-aligned, version right-aligned in the same
  row, both in smaller text. The `Kubernetes-Native` token stays unbreakable
  (`whitespace-nowrap` on the inner span) so the wrap point is the space before
  `Way`. If the tagline still has to wrap to two visual lines at extremely narrow
  widths, the version sticks to the right edge of the row's first line.

### `sm:` and up (≥ 640 px) — one row

- Logo + accent bar + tagline + version + spacer + auth + theme toggle, all inline.
- The existing `md:hidden` duplicate theme toggle is removed: one toggle serves
  all widths and lives in the brand strip's right cluster.

### Auth area

When `authInfo.enabled && authInfo.authenticated` is true, the name + Logout
button render in the right cluster next to the theme toggle. Below `sm:`, that
cluster sits in row 1 with the logo on the left and the right side reserved for
auth + theme; if both are present and the row gets too tight, `flex-wrap` lets
the auth chunk drop to its own row below the logo/theme row. When auth is
disabled or unauthenticated, only the theme toggle shows in the right cluster.

## Stats strip

The stat badges container becomes a sibling of the brand strip inside the same
`<header>`. Structure stays as today: `flex flex-wrap items-center gap-2` with
the 8 `StatBadge` children passed through the existing `children` prop.

- The current forced line break at `lg:` (`<div className="hidden lg:block basis-full h-0" />`)
  is preserved.
- The strip gets `pt-3` so it reads as its own band; horizontal padding matches
  the brand strip (`px-3 sm:px-6 lg:px-8`).
- The dashboard summary inside the strip is unchanged — same badges, same
  filter behaviour, same callbacks.

## Files touched

- `src/static/components/SiteHeader.js` — restructured into two stacked
  containers inside the existing `<header>` element. The component's exported
  signature (`{ version, authInfo, children }`) and the children API stay
  identical.

No other files change. `src/static/index.html` keeps passing the same 8 StatBadge
children, the spacer `<div>`, and the `version` / `authInfo` props.

## Out of scope

- The stat badges themselves (sizing, copy, icons, count).
- Filter behaviour on click.
- The dashboard table beneath the header.
- Any change to the auth/login flow.

## Testing

Visual verification in the local Docker preview at the same widths used for the
scroll-fix sweep: 340, 640, 768, 1024, 1280, 1920 px. At each width, confirm:

- The two bands render with a visible border between them.
- No horizontal document scroll.
- Theme toggle sits in the brand strip's right cluster (not floating).
- Tagline wraps only at the space before `Way` at the narrowest widths.
- Stat badges still wrap as today.

A before/after comparison page (`tmp/preview/snapshots/comparison-header.html`)
is generated as part of the work to make the diff easy to review.

## Rollout

Single commit on the same feature branch (`fix/dashboard-horizontal-scroll`) so
the open PR carries both the scroll fix and this header refactor. PR description
gets a short addendum covering the brand/stats split.
