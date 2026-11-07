// Omnisnap geometry extractor. Evaluated in the page as ONE expression after
// stabilize.js. Engine-agnostic; the Go core will embed this file unchanged.
// Returns a JSON string: { meta, root } where every node is
//   t   tag            id/tid/role  stable anchors     c  first classes
//   n   accessible-ish name           u  href/src (host+path)
//   tx  own text (truncated)  th  hash of own text   ln  line boxes of own text
//   tw  horizontal extent of own text   lt  tops of own-text line boxes (doc px)
//   lh/fs/ff/fw/ws  computed line-height, font-size, first font family,
//       font-weight, white-space (text owners only)
//   ia  interactive (link, button, form control, interactive role)
//   tr  text visibly truncated (ellipsis or line clamp cutting content)
//   b   [x, y, w, h] border box in document coordinates (0.1 px)
//   d   computed display   p  position (if not static)   hid  invisible
//   o   [overflowX, overflowY] px of content beyond the box   of  overflow modes
//   k   rendered children (open shadow roots included)
(() => {
  const MAX_NODES = 20000;
  const SKIP = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'TEMPLATE', 'LINK', 'META', 'HEAD', 'TITLE', 'BASE']);
  const sx = window.scrollX;
  const sy = window.scrollY;
  const r1 = (v) => Math.round(v * 10) / 10;
  const norm = (s) => (s || '').replace(/\s+/g, ' ').trim();
  let count = 0;
  let truncated = false;

  const hash = (s) => { // FNV-1a, 32 bit
    let h = 0x811c9dc5;
    for (let i = 0; i < s.length; i++) { h ^= s.charCodeAt(i); h = Math.imul(h, 0x01000193); }
    return (h >>> 0).toString(36);
  };

  const ownText = (el) => {
    let t = '';
    for (const n of el.childNodes) if (n.nodeType === 3) t += n.nodeValue;
    return norm(t);
  };

  // Line boxes occupied by the element's own text nodes, and the horizontal
  // extent of that text. The extent tells "my text got wider" (a cause) apart
  // from "my box was resized from outside" (an effect).
  const range = document.createRange();
  const textGeometry = (el) => {
    const tops = [];
    let left = Infinity;
    let right = -Infinity;
    for (const n of el.childNodes) {
      if (n.nodeType !== 3 || !n.nodeValue.trim()) continue;
      range.selectNodeContents(n);
      for (const r of range.getClientRects()) {
        if (r.width < 0.5 || r.height < 0.5) continue;
        if (!tops.some((t) => Math.abs(t - r.top) < 3)) tops.push(r.top);
        left = Math.min(left, r.left);
        right = Math.max(right, r.right);
      }
    }
    return { lines: tops.length, width: right > left ? right - left : 0, tops };
  };

  const INTERACTIVE = new Set(['A', 'BUTTON', 'INPUT', 'SELECT', 'TEXTAREA', 'SUMMARY', 'LABEL']);
  const INTERACTIVE_ROLES = /^(button|link|tab|menuitem|checkbox|radio|switch|option|combobox|textbox)$/;
  const isInteractive = (el) => (INTERACTIVE.has(el.tagName) && (el.tagName !== 'A' || el.hasAttribute('href')))
    || INTERACTIVE_ROLES.test(el.getAttribute('role') || '') || el.tabIndex >= 0 && el.hasAttribute('tabindex');
  const r2 = (v) => Math.round(v * 100) / 100;

  const urlKey = (u) => {
    try { const x = new URL(u, location.href); return x.host + x.pathname; } catch (_) { return u.slice(0, 120); }
  };

  const walk = (el) => {
    if (SKIP.has(el.tagName)) return null;
    if (count >= MAX_NODES) { truncated = true; return null; }
    const cs = getComputedStyle(el);
    if (cs.display === 'none') return null;
    count++;

    const rect = el.getBoundingClientRect();
    const node = { t: el.tagName.toLowerCase() };
    if (el.id) node.id = el.id;
    const tid = el.getAttribute('data-testid') || el.getAttribute('data-test') || el.getAttribute('data-cy');
    if (tid) node.tid = tid;
    const role = el.getAttribute('role');
    if (role) node.role = role;
    const cls = norm(el.getAttribute('class'));
    if (cls) node.c = cls.split(' ').slice(0, 6).join(' ');
    const name = el.getAttribute('aria-label') || el.getAttribute('alt') || el.getAttribute('title') || el.getAttribute('name');
    if (name) node.n = norm(name).slice(0, 60);
    const ref = el.getAttribute('href') || el.getAttribute('src');
    if (ref && !ref.startsWith('data:') && !ref.startsWith('javascript:')) node.u = urlKey(ref);
    const own = ownText(el);
    if (own) {
      const tg = textGeometry(el);
      node.tx = own.slice(0, 60);
      node.th = hash(own);
      node.ln = tg.lines;
      node.tw = r1(tg.width);
      // Line boxes and typography: line pitch is the median gap between line
      // tops; comparing it with the computed line-height names the mechanism
      // (e.g. 22.75px laid out as 22px).
      node.lt = tg.tops.map((t) => r1(t + sy)).sort((p, q) => p - q).slice(0, 60);
      node.lh = cs.lineHeight === 'normal' ? 'normal' : r2(parseFloat(cs.lineHeight));
      node.fs = r2(parseFloat(cs.fontSize));
      node.ff = (cs.fontFamily.split(',')[0] || '').replace(/["']/g, '').trim().toLowerCase();
      node.fw = cs.fontWeight;
      if (cs.whiteSpace !== 'normal') node.ws = cs.whiteSpace;
    }
    if (isInteractive(el)) node.ia = 1;
    // Visibly truncated text: ellipsis or line clamp actually cutting content.
    const clamp = cs.webkitLineClamp && cs.webkitLineClamp !== 'none';
    if ((cs.textOverflow === 'ellipsis' && el.scrollWidth > el.clientWidth + 1) || (clamp && el.scrollHeight > el.clientHeight + 1)) node.tr = 1;

    node.b = [r1(rect.left + sx), r1(rect.top + sy), r1(rect.width), r1(rect.height)];
    node.d = cs.display;
    if (cs.position !== 'static') node.p = cs.position;
    if (cs.visibility !== 'visible' || cs.opacity === '0') node.hid = 1;

    if (el.clientWidth > 0 || el.clientHeight > 0) {
      const ox = el.scrollWidth - el.clientWidth;
      const oy = el.scrollHeight - el.clientHeight;
      if (ox > 1 || oy > 1) { node.o = [Math.max(0, ox), Math.max(0, oy)]; node.of = cs.overflowX + '/' + cs.overflowY; }
    }

    // An <svg> is one box: its shapes are graphics, not layout, and engines
    // disagree on their geometry APIs (e.g. <use> bounds), not their pixels.
    if (node.t === 'svg') return node;

    const kids = [];
    const children = el.shadowRoot ? [...el.shadowRoot.children, ...el.children] : el.children;
    for (const c of children) {
      const k = walk(c);
      if (k) kids.push(k);
    }
    if (kids.length) node.k = kids;
    return node;
  };

  const root = walk(document.documentElement);
  const de = document.documentElement;
  const body = document.body;
  const fonts = { loaded: [], failed: [] };
  if (document.fonts) {
    for (const f of document.fonts) {
      const fam = f.family.replace(/["']/g, '');
      if (f.status === 'loaded' && !fonts.loaded.includes(fam)) fonts.loaded.push(fam);
      if (f.status === 'error' && !fonts.failed.includes(fam)) fonts.failed.push(fam);
    }
  }
  return JSON.stringify({
    meta: {
      url: location.href,
      title: document.title,
      ua: navigator.userAgent,
      vw: innerWidth,
      vh: innerHeight,
      cw: de.clientWidth,
      dpr: devicePixelRatio,
      docW: Math.max(de.scrollWidth, body ? body.scrollWidth : 0),
      docH: Math.max(de.scrollHeight, body ? body.scrollHeight : 0),
      nodes: count,
      truncated,
      fonts,
    },
    root,
  });
})()
