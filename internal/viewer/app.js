// Omnisnap report (M1): summary table, causes, evidence, knowledge-base,
// design-intent list, dynamic-region shading, synced screenshot panes with
// clickable box overlays, keyboard navigation, theme toggle and host filter.
// Plain ES2020, no framework. Every string is inserted with textContent.
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

  // Theme toggle: follows the system preference unless the user overrides it.
  const THEME_KEY = 'omnisnap-theme';
  const applyTheme = (mode) => {
    document.documentElement.classList.toggle('theme-dark', mode === 'dark');
    document.documentElement.classList.toggle('theme-light', mode === 'light');
  };
  const storedTheme = () => { try { return localStorage.getItem(THEME_KEY); } catch { return null; } };
  const saveTheme = (mode) => { try { localStorage.setItem(THEME_KEY, mode); } catch {} };
  const systemDark = () => window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
  const cycleTheme = () => {
    const current = document.documentElement.classList.contains('theme-dark') ? 'dark'
      : document.documentElement.classList.contains('theme-light') ? 'light' : (systemDark() ? 'dark' : 'light');
    const next = current === 'dark' ? 'light' : 'dark';
    applyTheme(next);
    saveTheme(next);
  };
  const initTheme = () => applyTheme(storedTheme() || (systemDark() ? 'dark' : 'light'));
  initTheme();
  document.getElementById('theme').addEventListener('click', cycleTheme);

  // Host filter: only shown for merged bundles; persisted in localStorage so
  // #rN anchors keep working without rewriting the URL.
  const HOST_KEY = 'omnisnap-host-filter';
  const hosts = DATA.hosts || [];
  const hostIds = hosts.map((h) => h.id || h.os + '/' + h.arch);
  const isMultiHost = hostIds.length > 1;
  const storedHost = () => { try { return localStorage.getItem(HOST_KEY); } catch { return null; } };
  const saveHost = (id) => { try { localStorage.setItem(HOST_KEY, id); } catch {} };
  const hostSelectorEl = document.getElementById('host-selector');
  const hostSelect = document.getElementById('host-select');

  document.getElementById('meta').append(
    'Generated ' + DATA.generated + ' by ' + DATA.tool.name + ' ' + DATA.tool.version
    + ' · ' + DATA.results.length + ' comparisons on ' + DATA.pages.length + ' pages');

  // Summary: pages x engines with warning counts. Each row is tagged with its
  // host id so the host selector can filter the table too.
  const rows = DATA.results.map((r, i) => h('tr', { 'data-host': hostLabel(r) },
    h('td', {}, h('a', { href: '#' + anchor(i) }, r.page.slug)),
    h('td', {}, r.engine.engine === r.baseline.engine ? r.engine.engine + ' only' : r.baseline.engine + ' vs ' + r.engine.engine),
    h('td', {}, hostLabel(r)),
    h('td', {}, h('span', { class: 'status ' + r.status }, r.status)),
    h('td', { class: 'num' }, String(r.stats.report)),
    h('td', { class: 'num' }, String(r.stats.info)),
    h('td', { class: 'num' }, (r.stats.matchRate * 100).toFixed(1) + '%'),
    h('td', { class: 'num warn' }, String(r.warnings.length))));
  const skipped = DATA.skipped.map((s) => h('li', { 'data-host': s.host || '' }, (s.host ? s.host + ': ' : '') + s.engine + ' skipped: ' + s.reason));
  // Not the native append with a null: it would print "null" (backlog B18).
  document.getElementById('summary').append(
    h('table', {},
      h('thead', {}, h('tr', {}, ...['page', 'engines', 'host', 'status', 'report', 'info', 'matched', 'warnings'].map((t) => h('th', {}, t)))),
      h('tbody', {}, rows)),
    ...(skipped.length ? [h('ul', { class: 'muted skipped-list' }, skipped)] : []));

  // Two panes keep the same scroll fraction. Programmatic scrolls mark the
  // pane so the partner does not echo them back and fight the sync.
  const syncPanes = (a, b) => {
    const ignore = new Map();
    const setFraction = (pane, fraction) => {
      const range = pane.scrollHeight - pane.clientHeight;
      if (range <= 0) return;
      const top = Math.max(0, Math.min(range, fraction * range));
      if (Math.abs(pane.scrollTop - top) < 1) return;
      ignore.set(pane, true);
      pane.scrollTop = top;
      requestAnimationFrame(() => ignore.delete(pane));
    };
    const follow = (from, to) => from.addEventListener('scroll', () => {
      if (ignore.has(from)) { ignore.delete(from); return; }
      const range = from.scrollHeight - from.clientHeight;
      if (range <= 0) return;
      const fraction = from.scrollTop / range;
      setFraction(to, fraction);
    }, { passive: true });
    follow(a, b);
    follow(b, a);
    return { setFraction };
  };

  const syncs = new WeakMap();

  const auditCheckName = {
    'page-horizontal-scroll': 'horizontal page scroll',
    'clipped-text': 'clipped text',
    'truncated-text': 'truncated text',
    'overlap-interactive': 'overlapping interactive elements',
    'offscreen-interactive': 'offscreen interactive element',
    'zero-size-interactive': 'zero-size interactive element',
    'protrudes-parent': 'protrudes outside parent'
  };
  const renderAudit = (r) => {
    if (!r.audit) return null;
    const sideItems = (label, findings) => {
      if (!findings || !findings.length) return null;
      return h('div', { class: 'audit-side' },
        h('h4', {}, label + ' (' + findings.length + ')'),
        h('ul', {}, findings.map((f) => h('li', { class: 'audit-finding severity-' + f.severity },
          h('span', { class: 'audit-check' }, auditCheckName[f.check] || f.check),
          ' · ',
          h('code', {}, f.selector)))));
    };
    const baseline = sideItems('Baseline', r.audit.baseline);
    const engine = sideItems('Compared engine', r.audit.engine);
    if (!baseline && !engine) return null;
    return h('details', { class: 'audit' },
      h('summary', {}, 'Audit findings'),
      baseline,
      engine);
  };

  const chip = (text, cls) => h('span', { class: 'chip' + (cls ? ' ' + cls : '') }, text);
  const chips = (items, cls) => items && items.length ? h('div', { class: 'chips' }, items.map((i) => chip(i, cls))) : null;

  const formatDelta = (d) => {
    const parts = [];
    if (d.dw) parts.push((d.dw > 0 ? '+' : '') + d.dw + ' px width');
    if (d.dh) parts.push((d.dh > 0 ? '+' : '') + d.dh + ' px height');
    if (d.dx) parts.push((d.dx > 0 ? '+' : '') + d.dx + ' px left');
    if (d.dy) parts.push((d.dy > 0 ? '+' : '') + d.dy + ' px top');
    if (d.dln) parts.push((d.dln > 0 ? '+' : '') + d.dln + ' ln');
    return parts.length ? parts.join(' · ') : 'no size change';
  };

  const renderEvidence = (e) => {
    const list = [];
    if (e.mechanism) list.push(chip('mechanism: ' + e.mechanism));
    if (e.consensus) list.push(chip('consensus: ' + e.consensus));
    if (e.perception != null) list.push(chip('perception: ' + e.perception.toFixed(2)));
    if (e.ink != null) list.push(chip('ink: ' + (e.ink * 100).toFixed(1) + '%'));
    if (e.intent && e.intent.length) list.push(...e.intent.map((i) => chip('intent: ' + i)));
    return list.length ? h('div', { class: 'chips evidence' }, list) : null;
  };

  const renderKBDetail = (kb) => {
    if (!kb) return null;
    return h('div', { class: 'kb-detail' },
      h('h4', {}, kb.title || kb.id),
      h('p', {}, kb.explanation),
      kb.fix ? h('p', {}, h('strong', {}, 'Fix: '), kb.fix) : null);
  };

  const renderIntent = (intent) => {
    if (!intent || !intent.length) return null;
    return h('details', { class: 'intent' },
      h('summary', {}, 'Design intent (' + intent.length + ')'),
      h('ul', {}, intent.map((v) => h('li', {},
        h('strong', {}, v.engine === 'baseline' ? 'Baseline' : 'Engine'),
        ' breaks ',
        h('code', {}, v.selector),
        ': ' + v.relations.join(', '),
        v.px ? ' (' + v.px + ' px)' : '',
        v.keptBy && v.keptBy.length ? ' · kept by ' + v.keptBy.join(', ') : ''))));
  };

  const renderBox = (cause, side, box, shot, resultIdx) => {
    if (!box || !shot) return null;
    const [x, y, w, h_] = box;
    const style = 'left:' + (x / shot.w * 100) + '%;top:' + (y / shot.h * 100) + '%;width:' + (w / shot.w * 100) + '%;height:' + (h_ / shot.h * 100) + '%;';
    const el = h('div', {
      class: 'box',
      'data-cause-id': cause.id,
      'data-side': side,
      title: cause.title,
      role: 'button',
      tabindex: '0',
      style
    });
    el.addEventListener('click', () => activateCause(resultIdx, cause.id));
    el.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        activateCause(resultIdx, cause.id);
      }
    });
    return el;
  };

  const renderDynamicOverlay = (dynamic, side) => {
    const selectors = dynamic && dynamic[side];
    if (!selectors || !selectors.length) return null;
    return h('div', { class: 'dynamic-overlay' },
      h('span', {}, 'Dynamic: ' + selectors.join(', ')));
  };

  const pane = (label, side, shot, dynamic, causes, resultIdx) => {
    const hasDynamic = dynamic && (dynamic.baseline.length || dynamic.engine.length);
    // Built with h, which skips nulls: the native append would turn a null
    // (no dynamic regions, a cause without a box on this side) into a "null"
    // text node drawn over the screenshot (backlog B18).
    const overlays = h('div', { class: 'overlays' + (hasDynamic ? ' dynamic-shade' : '') },
      shot ? renderDynamicOverlay(dynamic, side) : null,
      shot ? causes.map((c) => renderBox(c, side, c.boxes[side], shot, resultIdx)) : null);
    return h('div', { class: 'pane', tabindex: '0', 'aria-label': label + ' screenshot' },
      h('h3', {}, label),
      shot
        ? h('div', { class: 'shot' },
            h('img', { src: shot.src, width: shot.w, height: shot.h, loading: 'lazy', alt: label + ' screenshot' }),
            overlays)
        : h('p', { class: 'missing muted' }, 'no screenshot'));
  };

  const activateCause = (resultIdx, causeId) => {
    const sec = document.getElementById(anchor(resultIdx));
    if (!sec) return;
    sec.querySelectorAll('.cause-active, .box-active').forEach((el) => el.classList.remove('cause-active', 'box-active'));
    sec.querySelectorAll('[data-cause-id]').forEach((el) => {
      if (el.dataset.causeId === causeId) {
        if (el.classList.contains('cause-item')) el.classList.add('cause-active');
        if (el.classList.contains('box')) el.classList.add('box-active');
      }
    });
    const r = DATA.results[resultIdx];
    const cause = r.causes.find((c) => c.id === causeId);
    if (!cause) return;
    const panes = sec.querySelectorAll('.pane');
    if (panes.length !== 2) return;
    const controller = syncs.get(sec);
    if (!controller) return;
    const shot = cause.boxes.baseline ? r.shots.baseline : r.shots.engine;
    const box = cause.boxes.baseline || cause.boxes.engine;
    if (!shot || !box) return;
    const frac = Math.max(0, Math.min(1, (box[1] + box[3] / 2) / shot.h));
    controller.setFraction(panes[0], frac);
    controller.setFraction(panes[1], frac);
  };

  const renderCauseItem = (cause, resultIdx) => {
    const item = h('div', { class: 'cause-item cause-' + cause.tier, 'data-cause-id': cause.id, 'data-result-idx': String(resultIdx), role: 'button', tabindex: '0' },
      h('div', { class: 'cause-header' },
        h('span', { class: 'tier tier-' + cause.tier }, cause.tier),
        h('strong', { class: 'cause-title' }, cause.title),
        chips(cause.kinds, 'kind')),
      h('div', { class: 'cause-body' },
        h('code', { class: 'cause-selector' }, cause.selector),
        h('span', { class: 'delta' }, formatDelta(cause.delta))),
      chips(cause.consequences, 'consequence'),
      renderEvidence(cause.evidence),
      cause.score != null ? h('div', { class: 'score' }, 'score: ' + cause.score.toFixed(2)) : null,
      renderKBDetail(cause.kbDetail),
      cause.count > 1 ? h('details', { class: 'instances' },
        h('summary', {}, cause.count + ' instances'),
        h('ul', {}, cause.instances.slice(0, 50).map((s) => h('li', {}, h('code', {}, s))))) : null);
    item.addEventListener('click', () => activateCause(resultIdx, cause.id));
    item.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        activateCause(resultIdx, cause.id);
      }
    });
    return item;
  };

  const renderCauses = (causes, resultIdx) => {
    const report = causes.filter((c) => c.tier === 'report');
    const info = causes.filter((c) => c.tier === 'info');
    const children = [];
    if (report.length) {
      children.push(h('div', { class: 'cause-group cause-group-report' },
        h('h3', {}, 'Report causes (' + report.length + ')'),
        ...report.map((c) => renderCauseItem(c, resultIdx))));
    }
    if (info.length) {
      children.push(h('details', { class: 'cause-group cause-group-info' },
        h('summary', {}, 'Info causes (' + info.length + ')'),
        ...info.map((c) => renderCauseItem(c, resultIdx))));
    }
    return children.length ? h('div', { class: 'causes' }, children) : null;
  };

  const renderResultSection = (r, i) => {
    const s = r.stats;
    const sec = h('section', { id: anchor(i), class: 'result', tabindex: '-1' },
      h('h2', { class: 'result-title' }, r.page.slug + ' · ' + r.baseline.engine + ' vs ' + r.engine.engine + ' · ' + hostLabel(r)),
      h('p', { class: 'muted' }, link(r.page.url), ' · ', h('span', { class: 'status ' + r.status }, r.status),
        ' · ' + r.engine.engine + ' ' + (r.engine.version || '(version unknown)') + ', ' + r.engine.profile + ' profile'
        + ' · ' + s.report + ' report, ' + s.info + ' info · ' + s.ms + ' ms'),
      ...r.warnings.map((w) => h('p', { class: 'warn' }, w)),
      renderAudit(r),
      renderCauses(r.causes, i),
      renderIntent(r.intent));
    if (r.engine.engine === r.baseline.engine) {
      sec.append(h('div', { class: 'viewer single' }, pane(r.baseline.engine + ' (one engine: nothing to compare)', 'baseline', r.shots && r.shots.baseline, r.dynamic, r.causes, i)));
    } else {
      const a = pane('baseline · ' + r.baseline.engine, 'baseline', r.shots && r.shots.baseline, r.dynamic, r.causes, i);
      const b = pane(r.engine.engine, 'engine', r.shots && r.shots.engine, r.dynamic, r.causes, i);
      sec.append(h('div', { class: 'viewer' }, a, b));
      syncs.set(sec, syncPanes(a, b));
    }
    return sec;
  };

  const results = document.getElementById('results');
  const fragment = document.createDocumentFragment();

  if (isMultiHost) {
    // Group by page slug, then by host id, keeping original rN anchors stable.
    const pageGroups = new Map();
    const pageOrder = [];
    DATA.results.forEach((r) => {
      if (!pageGroups.has(r.page.slug)) {
        pageGroups.set(r.page.slug, { page: r.page, results: [] });
        pageOrder.push(r.page.slug);
      }
      pageGroups.get(r.page.slug).results.push(r);
    });
    pageOrder.forEach((slug) => {
      const pg = pageGroups.get(slug);
      const hostGroups = new Map();
      const hostOrder = [];
      pg.results.forEach((r) => {
        const id = hostLabel(r);
        if (!hostGroups.has(id)) {
          hostGroups.set(id, { results: [] });
          hostOrder.push(id);
        }
        hostGroups.get(id).results.push(r);
      });
      const pageSec = h('section', { class: 'page-group', 'data-slug': slug },
        h('h2', { class: 'page-header' }, slug),
        h('p', { class: 'muted page-url' }, link(pg.page.url)));
      hostOrder.forEach((id) => {
        const hostDiv = h('div', { class: 'host-group', 'data-host': id },
          h('h3', { class: 'host-header' }, id));
        hostGroups.get(id).results.forEach((r) => {
          const i = DATA.results.indexOf(r);
          hostDiv.append(renderResultSection(r, i));
        });
        pageSec.append(hostDiv);
      });
      fragment.append(pageSec);
    });
  } else {
    DATA.results.forEach((r, i) => {
      fragment.append(renderResultSection(r, i));
    });
  }
  results.append(fragment);

  // Host selector and filtering.
  const applyHostFilter = (hostId) => {
    if (!isMultiHost) return;
    if (hostSelect && hostSelect.value !== hostId) hostSelect.value = hostId;
    document.querySelectorAll('#summary tbody tr').forEach((tr) => {
      tr.hidden = !!(hostId && tr.dataset.host !== hostId);
    });
    document.querySelectorAll('.skipped-list li').forEach((li) => {
      li.hidden = !!(hostId && li.dataset.host && li.dataset.host !== hostId);
    });
    document.querySelectorAll('.host-group').forEach((g) => {
      g.hidden = !!(hostId && g.dataset.host !== hostId);
    });
    document.querySelectorAll('.page-group').forEach((g) => {
      const visibleHost = g.querySelector('.host-group:not([hidden])');
      g.hidden = hostId ? !visibleHost : false;
    });
  };

  if (isMultiHost && hostSelectorEl && hostSelect) {
    hostSelectorEl.removeAttribute('hidden');
    hostIds.forEach((id) => hostSelect.append(h('option', { value: id }, id)));
    hostSelect.addEventListener('change', () => {
      applyHostFilter(hostSelect.value);
      saveHost(hostSelect.value);
    });
    const stored = storedHost();
    const hashTarget = location.hash && document.getElementById(location.hash.slice(1));
    if (stored && !(hashTarget && hashTarget.classList.contains('result'))) {
      applyHostFilter(stored);
    }
  }

  // Keyboard navigation: j/k and Home/End jump between visible result sections.
  const resultSections = () => Array.from(document.querySelectorAll('.result')).filter((s) => !s.closest('[hidden]'));
  const focusResult = (sec) => {
    if (!sec) return;
    sec.focus({ preventScroll: true });
    sec.scrollIntoView({ behavior: 'smooth', block: 'start' });
    history.replaceState(null, '', '#' + sec.id);
  };
  const currentResultIndex = () => {
    const active = document.activeElement;
    const secs = resultSections();
    const idx = active && active.classList && active.classList.contains('result') ? secs.indexOf(active) : -1;
    if (idx >= 0) return idx;
    const top = window.scrollY;
    let best = 0;
    let bestDist = Infinity;
    secs.forEach((sec, i) => {
      const d = Math.abs(sec.offsetTop - top);
      if (d < bestDist) { bestDist = d; best = i; }
    });
    return best;
  };
  document.addEventListener('keydown', (e) => {
    if (e.target && (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.isContentEditable)) return;
    const secs = resultSections();
    if (!secs.length) return;
    let next = -1;
    if (e.key === 'j') next = currentResultIndex() + 1;
    else if (e.key === 'k') next = currentResultIndex() - 1;
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = secs.length - 1;
    if (next >= 0 && next < secs.length) {
      e.preventDefault();
      focusResult(secs[next]);
    }
  });

  // If the page opens with a hash, focus that result for keyboard navigation.
  if (location.hash) {
    const target = document.getElementById(location.hash.slice(1));
    if (target && target.classList.contains('result')) target.setAttribute('tabindex', '-1');
  }
})();
