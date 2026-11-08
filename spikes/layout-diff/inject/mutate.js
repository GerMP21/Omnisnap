// Mutation testing on real pages: inject ONE realistic cross-engine bug into
// a settled page, in one engine only. Evaluated as one expression; installs
// window.__omnisnapMutate(ops, seed) -> Promise<JSON string>.
//
// Each operator imitates a bug class developers hit between engines (the
// planted pages' patterns, applied to real layouts): a missing feature
// fallback, a different default, a different sizing rule. The first
// operator in `ops` that has a candidate element is applied to a seeded
// random candidate. The target gets data-omnisnap-mut, its parent
// data-omnisnap-mutp; extract.js records both, so the diff knows the answer.
(() => {
  const rng = (seed) => () => { seed |= 0; seed = (seed + 0x6D2B79F5) | 0; let t = Math.imul(seed ^ (seed >>> 15), 1 | seed); t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t; return ((t ^ (t >>> 14)) >>> 0) / 4294967296; };
  const px = (v) => parseFloat(v) || 0;
  const docW = Math.max(document.documentElement.scrollWidth, innerWidth);
  const visible = (el, r, cs) => r.width >= 24 && r.height >= 12 && cs.visibility === 'visible' && cs.opacity !== '0' && cs.display !== 'none';
  const ownText = (el) => [...el.childNodes].some((n) => n.nodeType === 3 && n.nodeValue.trim().length > 12);
  const isBlock = (cs) => !/^inline$|^contents$/.test(cs.display);
  const kids = (el) => [...el.children].filter((k) => { const r = k.getBoundingClientRect(); return r.width > 0 && r.height > 0; });
  const flexRow = (cs) => /flex/.test(cs.display) && !/column/.test(cs.flexDirection);
  const pick = (r, xs) => xs[Math.floor(r() * xs.length)];

  // op: [applies(el, rect, style) -> boolean, css(el, rect, style, rng) -> string]
  const OPS = {
    'width-shrink': [(el, r, cs) => isBlock(cs) && r.width >= 120 && r.width < docW * 0.9, (el, r, cs, g) => `width: ${Math.round(r.width * pick(g, [0.6, 0.75, 0.9]))}px !important; max-width: none !important; flex: none !important;`],
    padding: [(el, r, cs) => isBlock(cs) && (el.children.length > 0 || ownText(el)), (el, r, cs, g) => `padding-top: ${px(cs.paddingTop) + pick(g, [4, 8, 16, 32])}px !important;`],
    'padding-left': [(el, r, cs) => isBlock(cs) && ownText(el), (el, r, cs, g) => `padding-left: ${px(cs.paddingLeft) + pick(g, [6, 12, 24])}px !important;`],
    'margin-top': [(el, r, cs) => isBlock(cs) && el.previousElementSibling, (el, r, cs, g) => `margin-top: ${px(cs.marginTop) + pick(g, [6, 12, 24])}px !important;`],
    nudge: [(el, r, cs) => cs.position === 'static' && isBlock(cs), (el, r, cs, g) => `position: relative !important; ${pick(g, ['top', 'left'])}: ${pick(g, [3, 5, 8, 14])}px !important;`],
    hide: [(el, r) => r.height <= 600, () => 'display: none !important;'],
    invisible: [(el, r) => r.height <= 600, () => 'visibility: hidden !important;'],
    gap: [(el, r, cs) => /flex|grid/.test(cs.display) && kids(el).length >= 2, (el, r, cs, g) => `gap: ${pick(g, [8, 16, 24])}px !important;`],
    'wrap-narrow': [(el, r, cs) => flexRow(cs) && kids(el).length >= 3 && r.width >= 200, (el, r) => `flex-wrap: wrap !important; max-width: ${Math.round(r.width * 0.6)}px !important;`],
    'font-up': [(el, r, cs) => ownText(el) && px(cs.fontSize) >= 12, (el, r, cs, g) => `font-size: ${Math.round(px(cs.fontSize) * pick(g, [1.15, 1.3]))}px !important;`],
    nowrap: [(el, r, cs) => ownText(el) && r.height >= px(cs.fontSize) * 2.5, () => 'white-space: nowrap !important;'],
    'align-self': [(el) => { const p = el.parentElement; if (!p) return false; const pcs = getComputedStyle(p); return flexRow(pcs) && /normal|stretch/.test(pcs.alignItems) && kids(p).length >= 2; }, () => 'align-self: flex-start !important;'],
    overlap: [(el, r, cs) => { const p = el.previousElementSibling; if (!p || !isBlock(cs)) return false; const pr = p.getBoundingClientRect(); return Math.abs(pr.top - r.top) < 4 && pr.right <= r.left + 1; }, (el, r, cs, g) => `margin-left: -${pick(g, [16, 32])}px !important;`],
    clip: [(el, r, cs) => r.height >= 60 && (ownText(el) || el.children.length > 0) && cs.overflow === 'visible', (el, r) => `max-height: ${Math.round(r.height * 0.5)}px !important; overflow: hidden !important;`],
    order: [(el) => { const p = el.parentElement; return p && /flex|grid/.test(getComputedStyle(p).display) && kids(p).length >= 3 && el !== kids(p)[0]; }, () => 'order: -1 !important;'],
    'letter-spacing': [(el, r, cs) => ownText(el), (el, r, cs, g) => `letter-spacing: ${pick(g, [0.04, 0.08])}em !important;`],
    'line-height': [(el, r, cs) => ownText(el) && r.height >= px(cs.fontSize) * 2.5, (el, r, cs, g) => `line-height: ${pick(g, [1.25, 1.6])} !important;`],
    border: [(el, r, cs) => isBlock(cs) && r.width >= 60, (el, r, cs, g) => `border: ${pick(g, [2, 4])}px solid transparent !important;`],
    'ellipsis-clip': [(el, r, cs) => cs.textOverflow === 'ellipsis', () => 'text-overflow: clip !important;'],
  };

  window.__omnisnapMutate = (ops, seed) => {
    const g = rng(seed);
    const all = [...document.body.querySelectorAll('*')].filter((el) => !/^(SCRIPT|STYLE|NOSCRIPT|TEMPLATE|svg|path|BR|HTML|BODY)$/.test(el.tagName) && !el.closest('svg'));
    for (const op of ops) {
      const [applies, css] = OPS[op] || [];
      if (!applies) continue;
      const cands = all.filter((el) => {
        const r = el.getBoundingClientRect();
        const cs = getComputedStyle(el);
        if (!visible(el, r, cs) || cs.position === 'fixed') return false;
        if (r.width * r.height > docW * 1200 * 0.5) return false; // not a page-sized wrapper
        try { return Boolean(applies(el, r, cs)); } catch (_) { return false; }
      });
      if (!cands.length) continue;
      const el = cands[Math.floor(g() * cands.length)];
      const r = el.getBoundingClientRect();
      const rule = css(el, r, getComputedStyle(el), g);
      // A real CSS bug hits a RULE, not one element: when the target has a
      // class shared by a few elements (<= 30), every one of them gets it.
      const cls = [...el.classList].find((c) => /^[A-Za-z_][\w-]*$/.test(c));
      let group = [el];
      if (cls) {
        const same = [...document.querySelectorAll(`${el.tagName.toLowerCase()}.${CSS.escape(cls)}`)];
        if (same.length > 1 && same.length <= 30) group = same;
      }
      for (const x of group) {
        x.setAttribute('data-omnisnap-mut', op);
        if (x.parentElement) x.parentElement.setAttribute('data-omnisnap-mutp', op);
      }
      const style = document.createElement('style');
      style.id = '__omnisnap_mut';
      style.textContent = `[data-omnisnap-mut] { ${rule} }`;
      document.head.appendChild(style);
      const raf = window.__omnisnapRAF || window.requestAnimationFrame.bind(window);
      const tag = el.tagName.toLowerCase() + (el.id ? `#${el.id}` : '');
      return new Promise((resolve) => raf(() => raf(() => resolve(JSON.stringify({ ok: true, op, rule, tag, scope: group.length, candidates: cands.length, box: [r.left + scrollX, r.top + scrollY, r.width, r.height].map((v) => Math.round(v)) })))));
    }
    return Promise.resolve(JSON.stringify({ ok: false, tried: ops }));
  };
  // Undo the mutation (several mutants share one page load).
  window.__omnisnapMutateRevert = () => {
    document.getElementById('__omnisnap_mut')?.remove();
    for (const x of document.querySelectorAll('[data-omnisnap-mut]')) x.removeAttribute('data-omnisnap-mut');
    for (const x of document.querySelectorAll('[data-omnisnap-mutp]')) x.removeAttribute('data-omnisnap-mutp');
    const raf = window.__omnisnapRAF || window.requestAnimationFrame.bind(window);
    return new Promise((resolve) => raf(() => raf(() => resolve('{"reverted":true}'))));
  };
  return JSON.stringify({ installed: Object.keys(OPS) });
})()
