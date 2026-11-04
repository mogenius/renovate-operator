(function () {
  'use strict';

  // Shared Floating UI positioning for tooltip-style bubbles. Anchors the
  // `floating` element below `reference` (flipping and shifting to stay in the
  // viewport) and keeps it there while either element moves or resizes. Returns
  // the autoUpdate cleanup function.
  window.FloatingUtils = {
    anchorBelow: (reference, floating) => {
      const { computePosition, autoUpdate, offset, flip, shift } = window.FloatingUI;
      const position = () => {
        if (!reference.isConnected || !floating.isConnected) return;
        computePosition(reference, floating, {
          placement: 'bottom',
          strategy: 'fixed',
          middleware: [offset(8), flip(), shift({ padding: 8 })],
        }).then(({ x, y }) => {
          Object.assign(floating.style, { left: `${x}px`, top: `${y}px` });
        });
      };
      position();
      return autoUpdate(reference, floating, position);
    },
  };
})();
