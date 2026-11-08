// Self-contained HTML report: summary table, then per page and engine pair
// the Report causes, the Info causes (collapsed), and a synced side-by-side
// viewer with each cause's votes and evidence.

export function reportHtml(payload) {
  const json = JSON.stringify(payload).replace(/</g, '\\u003c');
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Layout-diff spike</title>
<style>
:root { --bg:#fff; --fg:#1d1d1f; --muted:#667; --line:#dde; --card:#f5f6f8; --hi:#e8590c; --ok:#2f9e44; --rep:#c92a2a; --info:#5c7cfa; }
@media (prefers-color-scheme: dark) { :root { --bg:#141517; --fg:#e9ecef; --muted:#9aa; --line:#2c2e33; --card:#1d1f23; } }
* { box-sizing: border-box; }
body { margin:0; font:14px/1.45 system-ui, sans-serif; background:var(--bg); color:var(--fg); }
header.top { position:sticky; top:0; z-index:5; background:var(--bg); border-bottom:1px solid var(--line); padding:8px 16px; display:flex; gap:16px; align-items:center; flex-wrap:wrap; }
main { padding:16px; max-width:1700px; margin:0 auto; }
a { color: inherit; }
table { border-collapse:collapse; width:100%; }
th, td { text-align:left; padding:4px 8px; border-bottom:1px solid var(--line); }
td.n { text-align:right; font-variant-numeric:tabular-nums; }
section.page { margin:36px 0; border-top:2px solid var(--line); padding-top:12px; }
.muted { color:var(--muted); } .warn { color:#e67700; }
ol.causes { padding-left:24px; margin:6px 0; }
ol.causes li { margin:6px 0; padding:4px 6px; border-radius:4px; cursor:pointer; }
ol.causes li.active { background:var(--card); outline:1px solid var(--ok); }
.tier { display:inline-block; font-size:11px; font-weight:700; padding:0 6px; border-radius:8px; color:#fff; margin-right:6px; }
.tier.report { background:var(--rep); } .tier.info { background:var(--info); }
.t { font-weight:600; }
.votes { font-size:11px; margin-top:2px; } .votes .v { display:inline-block; padding:0 4px; margin:1px 2px 0 0; border-radius:6px; border:1px solid var(--line); }
.votes .pos { border-color:var(--rep); color:var(--rep); } .votes .neg { border-color:var(--info); color:var(--info); }
.pscore { font-variant-numeric:tabular-nums; font-size:11px; margin-left:6px; }
.chip { display:inline-block; font-size:11px; padding:0 5px; border:1px solid var(--line); border-radius:8px; margin-left:4px; }
.fix { color:var(--ok); }
code { font-size:12px; overflow-wrap:anywhere; }
details summary { cursor:pointer; }
.viewer { display:grid; grid-template-columns:1fr 1fr; gap:12px; margin-top:12px; }
.pane { height:72vh; overflow:auto; border:1px solid var(--line); background:#fff; }
.pane h4 { position:sticky; top:0; margin:0; padding:4px 8px; background:var(--card); color:var(--fg); z-index:2; font-size:12px; }
.shot { position:relative; }
.shot img { width:100%; display:block; }
.box { position:absolute; border:2px solid var(--hi); background:rgba(232,89,12,.07); pointer-events:none; }
.box span { position:absolute; top:-17px; left:-2px; background:var(--hi); color:#fff; font-size:11px; padding:0 4px; border-radius:2px; }
.box.active { border-color:var(--ok); background:rgba(47,158,68,.15); }
.box.active span { background:var(--ok); }
.dyn { position:absolute; pointer-events:none; background:repeating-linear-gradient(45deg, rgba(120,120,120,.28) 0 6px, rgba(120,120,120,.08) 6px 12px); outline:1px dashed #888; }
.dyn span { position:absolute; top:2px; left:2px; background:#555; color:#fff; font-size:10px; padding:0 3px; }
@media (max-width: 900px) { .viewer { grid-template-columns:1fr; } }
</style></head><body>
<header class="top"><strong>Layout-diff spike</strong><span id="eng" class="muted"></span></header>
<main>
<p><span class="tier report">REPORT</span> causes have a visible consequence that is not an expected engine difference; presence, visibility, clipped content,
a new scrollbar, protrusion beyond the viewport and truncation are always reported. <span class="tier info">INFO</span> causes are the rest. P is the judge's
probability that a cause is worth reporting, from a weak-supervision model (no human labels) over nine voters (docs/SCIENTIFIC_GROUNDING.md); it ranks causes
but does not set the tier unless the diff ran with JUDGE=1. <b>+</b>/<b>&minus;</b> chips show each vote, hover for the evidence. Gray hatched areas changed between two runs of the same
engine (dynamic content) and are ignored.</p>
<div id="summary"></div><div id="pages"></div>
</main>
<script type="application/json" id="data">${json}</script>
<script>
const DATA = JSON.parse(document.getElementById('data').textContent);

function h(tag, attrs, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (k === 'class') e.className = v;
    else if (k.startsWith('on')) e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v);
  }
  for (const k of kids) if (k != null) e.append(k);
  return e;
}
const px = (v) => (v > 0 ? '+' : '') + v + 'px';
function deltas(f, pair) {
  if (!f.d) return f.note || ('present only in ' + (f.side === 'A' ? DATA.engines.A : pair));
  const d = f.d, out = [];
  if (d.dln) out.push('lines ' + f.a.ln + ' to ' + f.b.ln);
  if (d.dw) out.push('width ' + px(d.dw));
  if (d.dh) out.push('height ' + px(d.dh));
  if (d.dx) out.push('x ' + px(d.dx));
  if (d.dy) out.push('y ' + px(d.dy));
  if (d.ovf) out.push('overflow ' + d.ovf + ' appears/disappears');
  return out.join(', ') || 'children moved';
}
// One chip per non-abstaining voter, with the evidence behind it.
function evidenceText(f, v) {
  const e = f.evidence || {};
  switch (v) {
    case 'mechanism': return e.mechanism ? e.mechanism.verdict + (e.mechanism.explanation ? ': ' + e.mechanism.explanation : '') + (e.mechanism.results ? ' | survives ' + JSON.stringify(e.mechanism.results) : '') : '';
    case 'perception': return e.perception ? 'score ' + e.perception.score + ': ' + (e.perception.events || []).map((x) => x.type + (x.edge ? ' ' + x.edge : '')).join(', ') : '';
    case 'calibration': return e.calibration?.by ? JSON.stringify(e.calibration.by) : '';
    case 'consensus': return e.consensus ? e.consensus.verdict + ' ' + JSON.stringify(e.consensus.votes || '') : '';
    case 'ink': return e.ink ? 'ink agreement ' + e.ink.agreement + ' vs page noise floor ' + e.ink.floor : '';
    case 'intent': return e.intent ? e.intent.verdict + ': ' + (e.intent.violations || []).join(' | ') : '';
    case 'novelty': return f.novelty ? 'same fingerprint on ' + f.novelty.otherPages + ' other site(s): ' + f.fingerprint : '';
    case 'catalog': return f.kb ? f.kb.id + ' ' + f.kb.title : '';
    default: return (f.consequences || []).join(', ');
  }
}
function votesLine(f) {
  if (!f.votes) return null;
  const e = f.evidence || {};
  const chips = Object.entries(f.votes).filter(([, x]) => x !== 0).map(([v, x]) => h('span', { class: 'v ' + (x > 0 ? 'pos' : 'neg'), title: evidenceText(f, v) }, (x > 0 ? '+' : '\u2212') + v));
  const facts = [];
  if (e.consensus && ['A-odd', 'B-odd'].includes(e.consensus.verdict)) facts.push(e.consensus.verdict === 'A-odd' ? 'baseline is the odd engine' : 'this engine is the odd one');
  if (e.mechanism && e.mechanism.verdict !== 'unknown') facts.push('mechanism: ' + e.mechanism.verdict);
  return h('div', { class: 'votes' }, ...chips, facts.length ? h('span', { class: 'muted' }, ' ' + facts.join(' · ')) : null);
}
const ver = (e) => { const v = DATA.versions[e]; return v ? (v.version || v) : e; };
document.getElementById('eng').textContent = 'baseline ' + ver(DATA.engines.A) + ' · vs ' + Object.keys(DATA.versions).filter((e) => e !== DATA.engines.A).map(ver).join(', ') + ' · generated ' + DATA.generated;
const anchor = (p) => p.slug + '--' + p.pair;

const tbody = h('tbody');
for (const p of DATA.pages) {
  const s = p.stats || {};
  tbody.append(h('tr', {},
    h('td', {}, h('a', { href: '#' + anchor(p) }, p.slug)), h('td', {}, p.pair), h('td', {}, p.group),
    h('td', { class: 'n' }, s.matchRate != null ? (s.matchRate * 100).toFixed(1) + '%' : 'n/a'),
    h('td', { class: 'n' }, String(s.report ?? '')), h('td', { class: 'n' }, String(s.info ?? '')),
    h('td', { class: 'n' }, String(s.rawFindings ?? '')), h('td', { class: 'n' }, String(s.masked ?? '')),
    h('td', { class: 'warn' }, p.error || p.warnings.join(' | '))));
}
document.getElementById('summary').append(h('table', {}, h('thead', {}, h('tr', {},
  ...['page', 'vs', 'group', 'matched', 'report', 'info', 'raw findings', 'masked', 'warnings'].map((t) => h('th', {}, t)))), tbody));

const pagesEl = document.getElementById('pages');
for (const p of DATA.pages) {
  const sec = h('section', { class: 'page', id: anchor(p) });
  sec.append(h('h2', {}, p.slug + ' · ' + DATA.engines.A + ' vs ' + p.pair), h('div', { class: 'muted' }, h('a', { href: p.url, target: '_blank', rel: 'noopener' }, p.url), ' · ' + p.group + (p.runs ? ' · compared runs ' + DATA.engines.A + '-' + p.runs.A + ' / ' + p.pair + '-' + p.runs.B : '')));
  for (const n of p.notes || []) sec.append(h('p', { class: 'muted' }, n));
  if (p.error) { sec.append(h('p', { class: 'warn' }, p.error)); pagesEl.append(sec); continue; }
  const s = p.stats;
  sec.append(h('p', { class: 'muted' }, 'matched ' + s.matched + ' of ' + s.nodesA + ' elements · ' + s.rawFindings + ' root-cause findings in ' + p.findings.length + ' causes · ' + s.explained + ' changed elements explained as effects · ' + s.masked + ' masked as dynamic'));
  for (const w of p.warnings) sec.append(h('p', { class: 'warn' }, w));
  if (p.symptoms.length) sec.append(h('p', { class: 'muted' }, 'symptoms (explained by a cause below): ' + p.symptoms.map((x) => x.kind + ' on ' + x.sel).join('; ')));

  const paneA = h('div', { class: 'pane' }, h('h4', {}, 'A · ' + DATA.engines.A));
  const paneB = h('div', { class: 'pane' }, h('h4', {}, 'B · ' + p.pair));
  const shotA = h('div', { class: 'shot' }, h('img', { src: p.shots.A.src, loading: 'lazy', alt: DATA.engines.A + ' screenshot' }));
  const shotB = h('div', { class: 'shot' }, h('img', { src: p.shots.B.src, loading: 'lazy', alt: p.pair + ' screenshot' }));
  paneA.append(shotA); paneB.append(shotB);
  const place = (el, dims, b) => {
    const [x, y, w, hh] = b;
    el.style.left = (x / dims.w * 100) + '%'; el.style.top = (y / dims.h * 100) + '%';
    el.style.width = Math.max(w / dims.w * 100, 0.3) + '%'; el.style.height = Math.max(hh / dims.h * 100, 0.2) + '%';
  };
  for (const [shot, dims, boxes] of [[shotA, p.shots.A, p.dynamicBoxes?.A], [shotB, p.shots.B, p.dynamicBoxes?.B]]) {
    for (const b of boxes || []) { const el = h('div', { class: 'dyn' }, h('span', {}, 'dynamic (ignored)')); place(el, dims, b); shot.append(el); }
  }
  const addBox = (shot, dims, bx, i) => {
    if (!bx) return null;
    const el = h('div', { class: 'box', 'data-i': String(i) }, h('span', {}, String(i + 1)));
    place(el, dims, bx.b); shot.append(el);
    return bx.b[1] / dims.h;
  };

  const lists = { report: h('ol', { class: 'causes' }), info: h('ol', { class: 'causes' }) };
  p.findings.forEach((f, i) => {
    const ya = addBox(shotA, p.shots.A, f.a, i);
    const yb = addBox(shotB, p.shots.B, f.b, i);
    for (const m of f.also || []) { addBox(shotA, p.shots.A, m.a, i); addBox(shotB, p.shots.B, m.b, i); }
    const li = h('li', { value: String(i + 1) },
      h('span', { class: 'tier ' + f.tier }, f.tier.toUpperCase()), h('span', { class: 't' }, f.title), f.count > 1 ? h('b', {}, ' ×' + f.count) : null,
      f.p != null ? h('span', { class: 'pscore muted', title: 'rules tier: ' + f.rulesTier + ', judge tier: ' + f.judgeTier }, 'P=' + f.p.toFixed(2) + (f.judgeTier && f.judgeTier !== f.tier ? ' (judge: ' + f.judgeTier + ')' : '')) : null,
      ...(f.consequences || []).map((c) => h('span', { class: 'chip' }, c)),
      votesLine(f),
      h('div', {}, h('code', {}, f.sel), ' · ' + deltas(f, p.pair), f.effect ? h('span', { class: 'muted' }, ' · effect: ' + f.effect.sel + ' ' + px(f.effect.dh)) : null),
      f.kb ? h('div', { class: 'muted' }, f.kb.id + ': ' + f.kb.explanation) : (f.note ? h('div', { class: 'muted' }, f.note) : null),
      f.kb?.fix ? h('div', { class: 'fix' }, 'fix: ' + f.kb.fix) : null,
      f.text ? h('div', { class: 'muted' }, '"' + f.text + '"') : null,
      f.count > 1 ? h('details', { onclick: (e) => e.stopPropagation() }, h('summary', { class: 'muted' }, f.count + ' instances'), h('div', { class: 'muted' }, f.members.join(' · '))) : null);
    li.addEventListener('click', () => {
      sec.querySelectorAll('.active').forEach((x) => x.classList.remove('active'));
      li.classList.add('active');
      sec.querySelectorAll('.box[data-i="' + i + '"]').forEach((x) => x.classList.add('active'));
      if (ya != null) paneA.scrollTop = ya * shotA.offsetHeight - 80;
      if (yb != null) paneB.scrollTop = yb * shotB.offsetHeight - 80;
    });
    lists[f.tier].append(li);
  });
  const nR = p.stats.report, nI = p.stats.info;
  sec.append(h('h3', {}, 'Report (' + nR + ')'), nR ? lists.report : h('p', { class: 'muted' }, 'nothing to report'));
  if (nI) sec.append(h('details', {}, h('summary', {}, 'Info (' + nI + '): expected engine differences or no visible consequence'), lists.info));

  let lock = false;
  const sync = (from, to) => from.addEventListener('scroll', () => {
    if (lock) { lock = false; return; }
    lock = true;
    to.scrollTop = from.scrollTop / Math.max(1, from.scrollHeight - from.clientHeight) * (to.scrollHeight - to.clientHeight);
  });
  sync(paneA, paneB); sync(paneB, paneA);
  sec.append(h('div', { class: 'viewer' }, paneA, paneB));
  pagesEl.append(sec);
}

</script></body></html>
`;
}
