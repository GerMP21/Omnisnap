// Omnisnap report (M0): summary table and two synced screenshot panes per
// result. Plain ES2020, no framework. Every string from the page or the run
// is inserted with textContent: selectors and page text are untrusted.
'use strict';
(() => {
  const DATA = JSON.parse(document.getElementById('data').textContent);

  // h builds an element; children that are strings become text nodes.
  const h = (tag, attrs, ...kids) => {
    const el = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs || {})) {
      if (v == null) continue;
      if (k === 'class') el.className = v; else el.setAttribute(k, String(v));
    }
    for (const k of kids.flat()) {
      if (k == null) continue;
      el.append(typeof k === 'string' || typeof k === 'number' ? document.createTextNode(String(k)) : k);
    }
    return el;
  };
  // Only http(s) links: a page URL must never become a javascript: link.
  const link = (url) => (/^https?:\/\//i.test(url) ? h('a', { href: url, target: '_blank', rel: 'noopener noreferrer' }, url) : h('span', {}, url));
  const hostLabel = (r) => r.host.id || r.host.os + '/' + r.host.arch;
  const anchor = (i) => 'r' + i;

  document.getElementById('meta').append(
    'Generated ' + DATA.generated + ' by ' + DATA.tool.name + ' ' + DATA.tool.version
    + ' · ' + DATA.results.length + ' comparisons on ' + DATA.pages.length + ' pages');

  // Summary: pages x engines.
  const rows = DATA.results.map((r, i) => h('tr', {},
    h('td', {}, h('a', { href: '#' + anchor(i) }, r.page.slug)),
    h('td', {}, r.engine.engine === r.baseline.engine ? r.engine.engine + ' only' : r.baseline.engine + ' vs ' + r.engine.engine),
    h('td', {}, hostLabel(r)),
    h('td', {}, h('span', { class: 'status ' + r.status }, r.status)),
    h('td', { class: 'num' }, r.stats.report),
    h('td', { class: 'num' }, r.stats.info),
    h('td', { class: 'num' }, (r.stats.matchRate * 100).toFixed(1) + '%'),
    h('td', { class: 'warn' }, r.warnings.join(' | '))));
  const skipped = DATA.skipped.map((s) => h('li', {}, (s.host ? s.host + ': ' : '') + s.engine + ' skipped: ' + s.reason));
  document.getElementById('summary').append(
    h('table', {},
      h('thead', {}, h('tr', {}, ...['page', 'engines', 'host', 'status', 'report', 'info', 'matched', 'warnings'].map((t) => h('th', {}, t)))),
      h('tbody', {}, rows)),
    skipped.length ? h('ul', { class: 'muted' }, skipped) : null);

  // Two panes keep the same document fraction. Setting scrollTop fires a
  // scroll event on the other pane; it is ignored only when one was caused.
  const syncPanes = (a, b) => {
    let ignore = null;
    const follow = (from, to) => from.addEventListener('scroll', () => {
      if (ignore === from) { ignore = null; return; }
      const fraction = from.scrollTop / Math.max(1, from.scrollHeight - from.clientHeight);
      const top = fraction * (to.scrollHeight - to.clientHeight);
      if (Math.abs(to.scrollTop - top) >= 1) { ignore = to; to.scrollTop = top; }
    }, { passive: true });
    follow(a, b);
    follow(b, a);
  };

  const pane = (label, shot) => h('div', { class: 'pane', tabindex: '0', 'aria-label': label + ' screenshot' },
    h('h3', {}, label),
    shot ? h('div', { class: 'shot' }, h('img', { src: shot.src, width: shot.w, height: shot.h, loading: 'lazy', alt: label + ' screenshot' }))
      : h('p', { class: 'missing muted' }, 'no screenshot'));

  const results = document.getElementById('results');
  DATA.results.forEach((r, i) => {
    const s = r.stats;
    const sec = h('section', { id: anchor(i), class: 'result' },
      h('h2', {}, r.page.slug + ' · ' + r.baseline.engine + ' vs ' + r.engine.engine + ' · ' + hostLabel(r)),
      h('p', { class: 'muted' }, link(r.page.url), ' · ', h('span', { class: 'status ' + r.status }, r.status),
        ' · ' + r.engine.engine + ' ' + (r.engine.version || '(version unknown)') + ', ' + r.engine.profile + ' profile'
        + ' · ' + s.report + ' report, ' + s.info + ' info · ' + s.ms + ' ms'),
      ...r.warnings.map((w) => h('p', { class: 'warn' }, w)));
    // A result whose engine is the baseline is a single-engine capture.
    if (r.engine.engine === r.baseline.engine) {
      sec.append(h('div', { class: 'viewer single' }, pane(r.baseline.engine + ' (one engine: nothing to compare)', r.shots && r.shots.baseline)));
    } else {
      const a = pane('baseline · ' + r.baseline.engine, r.shots && r.shots.baseline);
      const b = pane(r.engine.engine, r.shots && r.shots.engine);
      sec.append(h('div', { class: 'viewer' }, a, b));
      syncPanes(a, b);
    }
    results.append(sec);
  });
})();
