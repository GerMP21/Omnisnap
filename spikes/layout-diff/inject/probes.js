// Omnisnap mechanism probes: causal interventions run inside the page.
// Evaluated ONCE after capture; installs window.__omnisnapProbe with
//   apply(id)  neutralize one mechanism, wait for layout, resolve
//   revert()   undo everything applied
// The driver re-measures (extract.js) after each apply. If a cross-engine
// difference disappears under a probe, that probe names its mechanism.
// Script-only, so it works over CDP, BiDi and WebDriver Classic alike.
(() => {
  const raf = window.__omnisnapRAF || window.requestAnimationFrame.bind(window);
  const settle = () => new Promise((r) => raf(() => raf(r)));
  const CSS = {
    // Same font file and fixed advances in every engine: removes text
    // advance, shaping and font-metric differences (catalog K2, K4).
    font: '*:not(svg):not(svg *) { font-family: "Noto Sans Mono", "DejaVu Sans Mono", monospace !important; font-variant-ligatures: none !important; font-feature-settings: normal !important; letter-spacing: 0 !important; word-spacing: 0 !important; }',
    // Strip engine-specific form-control defaults (catalog K8).
    controls: 'input, select, textarea, button { appearance: none !important; -webkit-appearance: none !important; font: inherit !important; padding: 2px 4px !important; border: 1px solid #888 !important; border-radius: 0 !important; box-sizing: border-box !important; }',
  };
  let undo = [];
  // An element's inline style, read through the platform's own accessor: a
  // custom element can redefine style (youtube.com's <yt-attributed-string>
  // returns a Symbol), and a <form> control named "style" would shadow it
  // (Task 87). null for an element with no inline style (a namespace the
  // engine does not style): the probe skips it and counts it.
  const styleOf = (el) => {
    for (const C of [window.HTMLElement, window.SVGElement, window.MathMLElement]) {
      if (C && el instanceof C) return Object.getOwnPropertyDescriptor(C.prototype, 'style').get.call(el);
    }
    return null;
  };
  const D = Document.prototype;
  const docEl = () => Object.getOwnPropertyDescriptor(D, 'documentElement').get.call(document);
  // Whole-pixel line heights everywhere (catalog K1, K2). Computed values are
  // read for all elements first, then pinned, so inheritance can't interfere.
  const wholePixelLineHeights = () => {
    const plan = [];
    let unavailable = 0;
    for (const el of D.querySelectorAll.call(document, 'body, body *')) {
      const cs = getComputedStyle(el);
      if (cs.display === 'none') continue;
      const fs = parseFloat(cs.fontSize);
      const lh = cs.lineHeight === 'normal' ? 1.2 * fs : parseFloat(cs.lineHeight);
      if (!Number.isFinite(lh)) continue;
      const whole = Math.round(lh);
      if (cs.lineHeight !== 'normal' && Math.abs(lh - whole) < 0.01) continue;
      const st = styleOf(el);
      if (!st) { unavailable++; continue; }
      plan.push([st, whole]);
    }
    for (const [st, whole] of plan) {
      undo.push({ st, value: st.getPropertyValue('line-height'), priority: st.getPropertyPriority('line-height') });
      st.setProperty('line-height', `${whole}px`, 'important');
    }
    return { pinned: plan.length, unavailable };
  };
  const addStyle = (css) => {
    const s = D.createElement.call(document, 'style');
    s.setAttribute('data-omnisnap', 'probe');
    s.textContent = css;
    docEl().append(s);
    undo.push({ style: s });
  };
  window.__omnisnapProbe = {
    async apply(id) {
      this.revert();
      const parts = id === 'all' ? ['font', 'lh', 'controls'] : [id];
      let changed = 0;
      let unavailable = 0;
      for (const p of parts) {
        if (p === 'lh') {
          const r = wholePixelLineHeights();
          changed += r.pinned;
          unavailable += r.unavailable;
        } else addStyle(CSS[p]);
      }
      await settle();
      return JSON.stringify({ id, lineHeightsPinned: changed, lineHeightsUnavailable: unavailable });
    },
    revert() {
      for (const u of undo.reverse()) {
        if (u.style) u.style.remove();
        else if (u.value) u.st.setProperty('line-height', u.value, u.priority);
        else u.st.removeProperty('line-height');
      }
      undo = [];
      return 'reverted';
    },
  };
  return JSON.stringify({ installed: true });
})()
