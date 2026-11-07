// Compare Chrome (baseline, A) against every other captured engine (B) for
// every page: match elements, find root causes (lib/align.mjs), triage them
// into Report / Info causes (lib/triage.mjs), and write out/findings.json
// plus out/report.html (lib/report.mjs).
//
// usage: node diff.mjs [--show <slug-or-group>]

import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { TOL, analyze, dynamicMask, effectOf, matchTrees, prep, selector, severity, subtree } from './lib/align.mjs';
import { reportHtml } from './lib/report.mjs';
import { impactOf, triage } from './lib/triage.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const OUT = path.join(HERE, 'out');
const A_ENGINE = 'chrome';
const GROUP_ORDER = { planted: 0, own: 1, real: 2 };
const ENGINE_ORDER = ['firefox', 'webkit', 'chromium', 'brave', 'edge'];

async function load(file, engine) {
  try { return prep(JSON.parse(await fs.readFile(file, 'utf8')), engine); } catch (_) { return null; }
}
const snap = (n) => (n ? { b: n.b, ln: n.ln || 0, o: n.o || null } : null);
const pickDelta = (d) => (d ? { dw: d.dw, dh: d.dh, dx: d.dx, dy: d.dy, dln: d.dln, ovf: d.ovf } : null);
const KIND_TITLES = { 'children-moved': 'Children moved', 'children-differ': 'Different children', wrap: 'Wraps differently', width: 'Width differs', height: 'Height differs', position: 'Moved' };
const titleOf = (kinds) => kinds.map((k) => KIND_TITLES[k] || k.replace(/-/g, ' ')).join(' + ');

// Structural signature: element + parent + grandparent (tag and first
// classes, no ids or positions), the kinds, and the change rounded to px.
function signature(c) {
  const part = (n) => (n ? `${n.t}.${(n.c || '').split(' ').slice(0, 2).join('.')}` : '');
  const n = c.node;
  const d = c.d || {};
  return [part(n), part(n.parent), part(n.parent?.parent), c.side, c.kinds.join('+'),
    Math.round(d.dw || 0), Math.round(d.dh || 0), d.dln || 0, c.note || ''].join('|');
}

async function diffPair(dir, page, B_ENGINE) {
  const file = (engine, run) => path.join(dir, `${engine}-${run}.json`);
  let [A, A2, B, B2] = await Promise.all([
    load(file(A_ENGINE, 'a'), A_ENGINE), load(file(A_ENGINE, 'b'), A_ENGINE),
    load(file(B_ENGINE, 'a'), B_ENGINE), load(file(B_ENGINE, 'b'), B_ENGINE),
  ]);
  // Firefox ids stay as they were so round-1 labels keep matching.
  const idBase = B_ENGINE === 'firefox' ? page.slug : `${page.slug}@${B_ENGINE}`;
  const result = { ...page, pair: B_ENGINE, warnings: [], symptoms: [], findings: [] };
  if (!A || !B) {
    result.error = `missing capture: ${[!A && A_ENGINE, !B && B_ENGINE].filter(Boolean).join(', ')} (see ${page.slug}/*.error.txt)`;
    return result;
  }

  // Compare the runs whose page STATES match best (a slideshow may be on
  // different slides in each run); the other run of each engine masks
  // dynamic content. Ties keep run a.
  let runA = 'a';
  let runB = 'a';
  const cost = (x, y) => (x && y ? Math.abs(x.meta.docH - y.meta.docH) + 10 * Math.abs(x.meta.nodes - y.meta.nodes) : Infinity);
  let best = cost(A, B);
  for (const [ra, x] of [['a', A], ['b', A2]]) {
    for (const [rb, y] of [['a', B], ['b', B2]]) {
      const c = cost(x, y);
      if (c < best - 0.5) { best = c; runA = ra; runB = rb; }
    }
  }
  if (runA === 'b') [A, A2] = [A2, A];
  if (runB === 'b') [B, B2] = [B2, B];
  result.runs = { A: runA, B: runB };

  const maskA = A2 ? dynamicMask(A, A2) : new Set();
  const maskB = B2 ? dynamicMask(B, B2) : new Set();
  if (!A2 || !B2) result.warnings.push('second run missing: dynamic content is not masked');
  const M = matchTrees(A, B);
  const r = analyze(A, B, M, maskA, maskB);

  // ------------------------------------------------------------ warnings
  if (A.meta.cw !== B.meta.cw) result.warnings.push(`layout viewport differs: ${A_ENGINE} ${A.meta.cw}px vs ${B_ENGINE} ${B.meta.cw}px (scrollbars?) - widths everywhere will differ`);
  if (A.meta.truncated || B.meta.truncated) result.warnings.push('node cap hit: tree truncated');
  for (const [eng, m] of [[A_ENGINE, A.meta], [B_ENGINE, B.meta]]) {
    if (m.post && (Math.abs(m.post.docH - m.docH) > 2 || m.post.nodes !== m.nodes)) {
      result.warnings.push(`${eng}: the page changed during capture (height ${Math.round(m.docH)} -> ${Math.round(m.post.docH)}px, ${m.nodes} -> ${m.post.nodes} elements): screenshot and geometry may not match`);
    }
  }
  // Regions that changed between two runs of the SAME engine (slideshows,
  // typing effects, rotating content) are ignored; name and shade them.
  const topMasked = (T, mask) => T.all.filter((n) => mask.has(n.uid) && !(n.parent && mask.has(n.parent.uid)) && n.b[2] * n.b[3] > 0);
  const dynA = topMasked(A, maskA);
  const dynB = topMasked(B, maskB);
  result.dynamic = { A: dynA.slice(0, 5).map(selector), B: dynB.slice(0, 5).map(selector) };
  result.dynamicBoxes = { A: dynA.slice(0, 20).map((n) => n.b), B: dynB.slice(0, 20).map((n) => n.b) };
  // The engines may have rendered different page STATES; then findings
  // describe the state gap, not layout: say so instead of listing details.
  const bigGap = (x, y) => Math.abs(x - y) / Math.max(x, y) > 0.02 && Math.abs(x - y) > 200;
  const nodeGap = Math.abs(A.meta.nodes - B.meta.nodes) / Math.max(A.meta.nodes, B.meta.nodes);
  const selfDrift = (A2 && bigGap(A.meta.docH, A2.meta.docH)) || (B2 && bigGap(B.meta.docH, B2.meta.docH));
  if ((nodeGap > 0.01 && Math.abs(A.meta.nodes - B.meta.nodes) > 10) || bigGap(A.meta.docH, B.meta.docH)) {
    const regions = [...new Set([...result.dynamic.A, ...result.dynamic.B])].slice(0, 4).join(', ');
    result.warnings.push(selfDrift
      ? `the page changes over time: two runs of the same engine differ (page height ${Math.round(A.meta.docH)}${A2 ? `/${Math.round(A2.meta.docH)}` : ''} vs ${Math.round(B.meta.docH)}${B2 ? `/${Math.round(B2.meta.docH)}` : ''}px). Changing regions are ignored: ${regions || 'see masked count'}. Clock control would make them comparable.`
      : `engines rendered different page states: ${A.meta.nodes} vs ${B.meta.nodes} elements, page height ${Math.round(A.meta.docH)} vs ${Math.round(B.meta.docH)}px (timers, carousels, A/B or UA-dependent content); findings may describe the state gap, not layout`);
  }
  const failA = A.meta.fonts?.failed || [];
  const failB = B.meta.fonts?.failed || [];
  if (failA.join() !== failB.join()) result.warnings.push(`web fonts failed differently: ${A_ENGINE} [${failA.join(', ')}] vs ${B_ENGINE} [${failB.join(', ')}]`);
  // Requests the browser itself blocked (Brave Shields, Edge tracking
  // prevention): the usual CAUSE behind branded-browser differences.
  const blockedHosts = (meta) => [...new Set((meta.failed || []).filter((f) => /BLOCKED_BY_CLIENT/i.test(f.err)).map((f) => { try { return new URL(f.url).host; } catch (_) { return f.url; } }))];
  result.blocked = { A: blockedHosts(A.meta), B: blockedHosts(B.meta) };
  if (result.blocked.B.length && !result.blocked.A.length) result.warnings.push(`${B_ENGINE} blocked requests ${A_ENGINE} loaded: ${result.blocked.B.slice(0, 8).join(', ')}${result.blocked.B.length > 8 ? ', ...' : ''}`);
  if (A.meta.shotNote || B.meta.shotNote) result.notes = [A.meta.shotNote && `${A_ENGINE}: ${A.meta.shotNote}`, B.meta.shotNote && `${B_ENGINE}: ${B.meta.shotNote}`].filter(Boolean);

  let onlyANodes = 0;
  for (const n of M.onlyA) for (const _ of subtree(n)) onlyANodes++;
  result.stats = {
    nodesA: A.all.length,
    nodesB: B.all.length,
    matched: M.pairOf.size,
    matchRate: M.pairOf.size / A.all.length,
    unmatchedA: onlyANodes,
    dynamicA: maskA.size,
    dynamicB: maskB.size,
    explained: r.explained,
    masked: r.masked,
  };
  result.shots = {
    A: { src: `${page.slug}/${A_ENGINE}-${runA}.png`, ...A.meta.shot },
    B: { src: `${page.slug}/${B_ENGINE}-${runB}.png`, ...B.meta.shot },
  };
  result.symptoms = r.symptoms;

  // ---------------------------------------------- findings -> triage
  const ctx = { engines: { A: A_ENGINE, B: B_ENGINE }, D: r.D, vwA: A.meta.vw, vwB: B.meta.vw };
  const seen = new Set();
  const raw = r.causes.map((c) => {
    const sel = selector(c.node);
    let id = `${idBase}|${c.kinds.join('+')}|${sel}`;
    for (let i = 2; seen.has(id); i++) id = `${idBase}|${c.kinds.join('+')}|${sel}|${i}`;
    seen.add(id);
    const t = triage(c, ctx);
    return {
      key: t.kb ? `kb:${t.kb.id}` : signature(c),
      id,
      kinds: c.kinds,
      sel,
      side: c.side,
      tag: c.node.t,
      elId: c.node.id || null,
      text: c.node.tx || null,
      a: snap(c.side === 'A' ? c.node : null),
      b: snap(c.side === 'B' ? c.node : c.other),
      d: pickDelta(c.d),
      note: c.note || null,
      effect: effectOf(c, r.D),
      severity: severity(c.kinds, c.d),
      ...t,
    };
  });

  // ---------------------------------------- findings -> ranked causes
  // One cause per mechanism (knowledge-base entry) or per structural
  // signature (the same CSS on N repeated items). The headline instance is
  // the most impactful one; the cause is reported if any instance is.
  const groups = new Map();
  for (const f of raw) {
    if (!groups.has(f.key)) groups.set(f.key, []);
    groups.get(f.key).push(f);
  }
  result.findings = [...groups.values()].map((inst) => {
    const head = [...inst].sort((x, y) => y.weight * y.visibility - x.weight * x.visibility || (x.a || x.b).b[1] - (y.a || y.b).b[1])[0];
    const { key, weight, visibility, ...rest } = head;
    return {
      ...rest,
      title: head.kb?.title || titleOf(head.kinds),
      tier: inst.some((f) => f.tier === 'report') ? 'report' : 'info',
      consequences: [...new Set(inst.flatMap((f) => f.consequences))],
      impact: Math.round(impactOf(inst) * 100) / 100,
      count: inst.length,
      members: inst.slice(0, 50).map((f) => f.sel),
      memberIds: inst.map((f) => f.id),
      memberElIds: inst.map((f) => f.elId).filter(Boolean),
      also: inst.filter((f) => f !== head).slice(0, 30).map((f) => ({ a: f.a, b: f.b })),
    };
  }).sort((x, y) => (x.tier === y.tier ? 0 : x.tier === 'report' ? -1 : 1) || y.impact - x.impact || (x.a || x.b).b[1] - (y.a || y.b).b[1]);
  result.stats.rawFindings = raw.length;
  result.stats.report = result.findings.filter((f) => f.tier === 'report').length;
  result.stats.info = result.findings.length - result.stats.report;
  return result;
}

// --------------------------------------------------------------------- run

const dirs = [];
for (const entry of await fs.readdir(OUT, { withFileTypes: true })) {
  if (entry.isDirectory()) {
    try { await fs.access(path.join(OUT, entry.name, 'page.json')); dirs.push(path.join(OUT, entry.name)); } catch (_) { /* not a page */ }
  }
}
const pages = [];
for (const d of dirs) {
  const page = JSON.parse(await fs.readFile(path.join(d, 'page.json'), 'utf8'));
  const engines = (await fs.readdir(d)).map((f) => /^(\w+)-a\.json$/.exec(f)?.[1]).filter((e) => e && e !== A_ENGINE);
  for (const e of engines) pages.push(await diffPair(d, page, e));
}
const engineRank = (e) => { const i = ENGINE_ORDER.indexOf(e); return i < 0 ? 99 : i; };
pages.sort((x, y) => (GROUP_ORDER[x.group] ?? 9) - (GROUP_ORDER[y.group] ?? 9) || x.slug.localeCompare(y.slug) || engineRank(x.pair) - engineRank(y.pair));

let versions = {};
try { versions = JSON.parse(await fs.readFile(path.join(OUT, 'engines.json'), 'utf8')); } catch (_) { /* none */ }
const data = { generated: new Date().toISOString(), engines: { A: A_ENGINE }, versions, tol: TOL, pages };
await fs.writeFile(path.join(OUT, 'findings.json'), JSON.stringify(data, null, 1));
await fs.writeFile(path.join(OUT, 'report.html'), reportHtml(data));

const filterArg = process.argv.indexOf('--show');
const show = filterArg >= 0 ? process.argv[filterArg + 1] : null;
console.log(`${'page'.padEnd(22)}${'vs'.padEnd(10)}${'group'.padEnd(9)}${'match'.padStart(7)}${'report'.padStart(7)}${'info'.padStart(6)}${'raw'.padStart(5)}${'mask'.padStart(6)}  warnings`);
for (const p of pages) {
  if (show && !p.slug.includes(show) && p.group !== show) continue;
  if (p.error) { console.log(`${p.slug.padEnd(22)}${p.pair.padEnd(10)}${p.group.padEnd(9)}  ERROR ${p.error}`); continue; }
  const s = p.stats;
  console.log(`${p.slug.padEnd(22)}${p.pair.padEnd(10)}${p.group.padEnd(9)}${(`${(s.matchRate * 100).toFixed(1)}%`).padStart(7)}${String(s.report).padStart(7)}${String(s.info).padStart(6)}${String(s.rawFindings).padStart(5)}${String(s.masked).padStart(6)}  ${p.warnings.join(' | ')}`);
}
console.log('\nwrote out/findings.json and out/report.html');
