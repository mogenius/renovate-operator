// A tooltip that floats below its trigger on hover and keyboard focus.
// Positioning is delegated to Floating UI (vendored into js/floating-ui.esm.js)
// and the bubble is rendered into document.body through a portal, so no layout
// container can clip it. The trigger sits in a wrapper <span>: that keeps the
// trigger's own layout intact while giving the hover/focus listeners an element
// that still fires even when the trigger is a disabled button. The wrapper is
// `min-w-0` so a `truncate` trigger inside a flex row still shrinks: a flex item
// otherwise refuses to go below its content width and the ellipsis never shows.
// `content` takes precedence; otherwise `label` reads before any `keys`, which
// render as keycaps. With neither, the trigger renders without a tooltip.
//
// Opening is delayed a moment, like the native title tooltip, so brushing past
// a control does not flash its bubble; leaving or blurring closes immediately.
// Shared with the hand-rolled "Last Run Duration" tooltip in index.html so both
// open after the same pause.
const TOOLTIP_DELAY_MS = 500;
window.TOOLTIP_DELAY_MS = TOOLTIP_DELAY_MS;

function Tooltip({ content, label, keys = [], variant = "dark", className = "", children }) {
  const { useState, useRef, useLayoutEffect, useEffect, useCallback, isValidElement, cloneElement } = React;
  const [open, setOpen] = useState(false);
  const triggerRef = useRef(null);
  const tooltipRef = useRef(null);
  const delayRef = useRef(null);
  const idRef = useRef(`tooltip-${Math.random().toString(36).slice(2, 9)}`);

  const openTooltip = useCallback(() => {
    clearTimeout(delayRef.current);
    delayRef.current = setTimeout(() => {
      delayRef.current = null;
      setOpen(true);
    }, TOOLTIP_DELAY_MS);
  }, []);
  const closeTooltip = useCallback(() => {
    clearTimeout(delayRef.current);
    delayRef.current = null;
    setOpen(false);
  }, []);

  // Drop any pending open if the trigger unmounts while the delay is running.
  useEffect(() => () => clearTimeout(delayRef.current), []);

  const bubbleClass =
    variant === "light"
      ? "pointer-events-none fixed top-0 left-0 z-50 w-64 rounded-lg bg-white dark:bg-slate-800 border border-gray-200 dark:border-slate-600 shadow-lg p-3 text-left"
      : "pointer-events-none fixed top-0 left-0 z-50 max-w-xs rounded-lg bg-gray-900 dark:bg-slate-700 px-2.5 py-1.5 text-xs text-white shadow-xl";

  const body =
    content !== undefined
      ? content
      : label || keys.length > 0
        ? (
          <span className="flex items-center gap-1.5">
            {label && <span>{label}</span>}
            {keys.map((key) => (
              <Keycap key={key} size="sm" variant="onDark">{key}</Keycap>
            ))}
          </span>
        )
        : null;

  useLayoutEffect(() => {
    if (!open) return;
    const trigger = triggerRef.current;
    const tooltip = tooltipRef.current;
    if (!trigger || !tooltip) return;

    return window.FloatingUtils.anchorBelow(trigger, tooltip);
  }, [open]);

  if (!body) {
    return isValidElement(children) ? children : <>{children}</>;
  }

  const child = isValidElement(children) ? children : <span>{children}</span>;
  const trigger = cloneElement(child, { "aria-describedby": open ? idRef.current : undefined });

  return (
    <>
      <span
        ref={triggerRef}
        className={`inline-flex min-w-0 ${className}`}
        onMouseEnter={openTooltip}
        onMouseLeave={closeTooltip}
        onFocus={openTooltip}
        onBlur={closeTooltip}
      >
        {trigger}
      </span>
      {open && window.createPortal(
        <div
          ref={tooltipRef}
          id={idRef.current}
          role="tooltip"
          className={bubbleClass}
        >
          {body}
        </div>,
        document.body
      )}
    </>
  );
}
window.Tooltip = Tooltip;
