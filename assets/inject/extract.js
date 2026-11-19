// Omnisnap geometry extractor. Evaluated in the page as ONE expression after
// stabilize.js. Engine-agnostic; the Go core will embed this file unchanged.
// Returns a JSON string: { meta, root } where every node is
//   t   tag            id/tid/role  stable anchors     c  first classes
//   n   accessible-ish name           u  href/src (host+path)
//   tx  own text (truncated)  th  hash of own text   ln  line boxes of own text
//   tw  horizontal extent of own text   lt  tops of own-text line boxes (doc px)
//   ti  inset of own text from the box's left edge (px)
//   mut / mutp  mutation-testing markers (inject/mutate.js): target / its parent
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

  // The page's DOM is read through the platform's own accessors, never
  // through properties the page can shadow: a <form> exposes its controls as
  // properties that win over built-ins (form.id is <input name="id"> when
  // there is one: ui.com, whose input carried a framework back-pointer, so
  // the result was a cycle), document does the same with named elements
  // (<form name="title">), and a custom element can redefine any property
  // (Tasks 87, 88).
  const E = Element.prototype;
  const getter = (proto, name) => Object.getOwnPropertyDescriptor(proto, name).get;
  const tagOf = getter(E, 'tagName');
  const idOf = getter(E, 'id');
  const childrenOf = getter(E, 'children');
  const shadowOf = getter(E, 'shadowRoot');
  const childNodesOf = getter(Node.prototype, 'childNodes');
  const scrollW = getter(E, 'scrollWidth');
  const scrollH = getter(E, 'scrollHeight');
  const clientW = getter(E, 'clientWidth');
  const clientH = getter(E, 'clientHeight');
  const attr = (el, n) => E.getAttribute.call(el, n);
  const hasAttr = (el, n) => E.hasAttribute.call(el, n);
  const rectOf = (el) => E.getBoundingClientRect.call(el);
  // tabIndex lives on HTMLElement and SVGElement (and MathMLElement).
  const tabIndexOf = (el) => {
    for (const C of [window.HTMLElement, window.SVGElement, window.MathMLElement]) {
      if (C && el instanceof C) return getter(C.prototype, 'tabIndex').call(el);
    }
    return -1;
  };
  const D = Document.prototype;
  const docEl = getter(D, 'documentElement').call(document);
  const docBody = getter(D, 'body').call(document);
  const docTitle = getter(D, 'title').call(document);
  const docFonts = getter(D, 'fonts') && getter(D, 'fonts').call(document);

  const hash = (s) => { // FNV-1a, 32 bit
    let h = 0x811c9dc5;
    for (let i = 0; i < s.length; i++) { h ^= s.charCodeAt(i); h = Math.imul(h, 0x01000193); }
    return (h >>> 0).toString(36);
  };

  const ownText = (el) => {
    let t = '';
    for (const n of childNodesOf.call(el)) if (n.nodeType === 3) t += n.nodeValue;
    return norm(t);
  };

  // Line boxes occupied by the element's own text nodes, and the horizontal
  // extent of that text. The extent tells "my text got wider" (a cause) apart
  // from "my box was resized from outside" (an effect).
  const range = Document.prototype.createRange.call(document);
  const textGeometry = (el) => {
    const tops = [];
    let left = Infinity;
    let right = -Infinity;
    for (const n of childNodesOf.call(el)) {
      if (n.nodeType !== 3 || !n.nodeValue.trim()) continue;
      range.selectNodeContents(n);
      for (const r of range.getClientRects()) {
        if (r.width < 0.5 || r.height < 0.5) continue;
        if (!tops.some((t) => Math.abs(t - r.top) < 3)) tops.push(r.top);
        left = Math.min(left, r.left);
        right = Math.max(right, r.right);
      }
    }
    return { lines: tops.length, width: right > left ? right - left : 0, tops, left };
  };

  const INTERACTIVE = new Set(['A', 'BUTTON', 'INPUT', 'SELECT', 'TEXTAREA', 'SUMMARY', 'LABEL']);
  const INTERACTIVE_ROLES = /^(button|link|tab|menuitem|checkbox|radio|switch|option|combobox|textbox)$/;
  const isInteractive = (el, tag) => (INTERACTIVE.has(tag) && (tag !== 'A' || hasAttr(el, 'href')))
    || INTERACTIVE_ROLES.test(attr(el, 'role') || '') || tabIndexOf(el) >= 0 && hasAttr(el, 'tabindex');
  const r2 = (v) => Math.round(v * 100) / 100;

  const urlKey = (u) => {
    try { const x = new URL(u, location.href); return x.host + x.pathname; } catch (_) { return u.slice(0, 120); }
  };

  const walk = (el) => {
    const tag = tagOf.call(el);
    if (SKIP.has(tag)) return null;
    if (count >= MAX_NODES) { truncated = true; return null; }
    const cs = getComputedStyle(el);
    if (cs.display === 'none') return null;
    count++;

    const rect = rectOf(el);
    const node = { t: tag.toLowerCase() };
    const id = idOf.call(el);
    if (id) node.id = id;
    const tid = attr(el, 'data-testid') || attr(el, 'data-test') || attr(el, 'data-cy');
    if (tid) node.tid = tid;
    const role = attr(el, 'role');
    if (role) node.role = role;
    const cls = norm(attr(el, 'class'));
    if (cls) node.c = cls.split(' ').slice(0, 6).join(' ');
    const name = attr(el, 'aria-label') || attr(el, 'alt') || attr(el, 'title') || attr(el, 'name');
    if (name) node.n = norm(name).slice(0, 60);
    const ref = attr(el, 'href') || attr(el, 'src');
    if (ref && !ref.startsWith('data:') && !ref.startsWith('javascript:')) node.u = urlKey(ref);
    const own = ownText(el);
    if (own) {
      const tg = textGeometry(el);
      node.tx = own.slice(0, 60);
      node.th = hash(own);
      node.ln = tg.lines;
      node.tw = r1(tg.width);
      if (Number.isFinite(tg.left)) node.ti = r1(tg.left - rect.left);
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
    if (isInteractive(el, tag)) node.ia = 1;
    if (hasAttr(el, 'data-omnisnap-mut')) node.mut = 1;
    if (hasAttr(el, 'data-omnisnap-mutp')) node.mutp = 1;
    // Visibly truncated text: ellipsis or line clamp actually cutting content.
    const clamp = cs.webkitLineClamp && cs.webkitLineClamp !== 'none';
    const sw = scrollW.call(el);
    const sh = scrollH.call(el);
    const cw = clientW.call(el);
    const ch = clientH.call(el);
    if ((cs.textOverflow === 'ellipsis' && sw > cw + 1) || (clamp && sh > ch + 1)) node.tr = 1;

    node.b = [r1(rect.left + sx), r1(rect.top + sy), r1(rect.width), r1(rect.height)];
    node.d = cs.display;
    if (cs.position !== 'static') node.p = cs.position;
    if (cs.visibility !== 'visible' || cs.opacity === '0') node.hid = 1;

    if (cw > 0 || ch > 0) {
      const ox = sw - cw;
      const oy = sh - ch;
      if (ox > 1 || oy > 1) { node.o = [Math.max(0, ox), Math.max(0, oy)]; node.of = cs.overflowX + '/' + cs.overflowY; }
    }

    // An <svg> is one box: its shapes are graphics, not layout, and engines
    // disagree on their geometry APIs (e.g. <use> bounds), not their pixels.
    if (node.t === 'svg') return node;

    const kids = [];
    const shadow = shadowOf.call(el);
    const children = shadow ? [...shadow.children, ...childrenOf.call(el)] : childrenOf.call(el);
    for (const c of children) {
      const k = walk(c);
      if (k) kids.push(k);
    }
    if (kids.length) node.k = kids;
    return node;
  };

  const root = walk(docEl);
  const de = docEl;
  const body = docBody;
  const fonts = { loaded: [], failed: [] };
  if (docFonts) {
    for (const f of docFonts) {
      const fam = f.family.replace(/["']/g, '');
      if (f.status === 'loaded' && !fonts.loaded.includes(fam)) fonts.loaded.push(fam);
      if (f.status === 'error' && !fonts.failed.includes(fam)) fonts.failed.push(fam);
    }
  }
  return JSON.stringify({
    meta: {
      url: location.href,
      title: docTitle,
      ua: navigator.userAgent,
      vw: innerWidth,
      vh: innerHeight,
      cw: clientW.call(de),
      dpr: devicePixelRatio,
      docW: Math.max(scrollW.call(de), body ? scrollW.call(body) : 0),
      docH: Math.max(scrollH.call(de), body ? scrollH.call(body) : 0),
      nodes: count,
      truncated,
      fonts,
    },
    root,
  });
})()
