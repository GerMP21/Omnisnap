// Compare Chrome (baseline, A) against every other captured engine (B) for
// every page: match elements, find root causes (lib/align.mjs), triage them
// into Report / Info causes (lib/triage.mjs), and write out/findings.json
// plus out/report.html (lib/report.mjs).
//
// Mutant pages (capture.mjs mut) are diffed against their base page and
// LABELED without humans: a same-engine diff (clean vs mutated) shows what
// the injected bug changed; cross-engine causes on those elements, near the
// target, are positives. They feed the tree model (train/), not the report.
//
// usage: node diff.mjs [--show <slug-or-group>]   (OMNISNAP_OUT=<dir> reads elsewhere)

import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { TOL, analyze, deltas, dynamicMask, effectOf, matchTrees, prep, selector, severity, subtree } from './lib/align.mjs';
import { PNG } from 'pngjs';
import { calibrate, calibrated } from './lib/calibrate.mjs';
import { FAMILY, consensusOf } from './lib/consensus.mjs';
import { inkAgreement, noiseFloor } from './lib/ink.mjs';
import { INTENT_TEXT, intentKind, intentViolations } from './lib/intent.mjs';
import { fitLabelModel, posterior, VOTERS, votesFor } from './lib/judge.mjs';
import { loadProbes, mechanismOf, probeContext } from './lib/mechanism.mjs';
import { fingerprint, surprise } from './lib/novelty.mjs';
import { perceptualEvents } from './lib/perception.mjs';
import { FEATURE_NAMES, featuresOf } from './lib/features.mjs';
import { loadModel } from './lib/gbdt.mjs';
import { reportHtml } from './lib/report.mjs';
import { PAGES } from './planted/pages.mjs';
import { HARD, impactOf, triage } from './lib/triage.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const OUT = process.env.OMNISNAP_OUT || path.join(HERE, 'out');
const A_ENGINE = 'chrome';
const GROUP_ORDER = { planted: 0, own: 1, real: 2, crawl: 3, mut: 4 };
const ENGINE_ORDER = ['firefox', 'webkit', 'chromium', 'brave', 'edge'];

async function load(file, engine) {
  try { return prep(JSON.parse(await fs.readFile(file, 'utf8')), engine); } catch (_) { return null; }
}
const snap = (n) => (n ? { b: n.b, ln: n.ln || 0, o: n.o || null } : null);
const pickDelta = (d) => (d ? { dw: d.dw, dh: d.dh, dx: d.dx, dy: d.dy, dln: d.dln, ovf: d.ovf } : null);
const KIND_TITLES = { columns: 'Table columns sized differently', 'text-shifted': 'Text moved inside its box', 'intent-align': 'Breaks an alignment the other engines keep', 'intent-size': 'Breaks equal sizes the other engines keep', 'intent-gap': 'Breaks even spacing the other engines keep', 'intent-column': 'Breaks a grid column the other engines keep', 'intent-overlap': 'Overlaps where the other engines keep it clear', 'children-moved': 'Children moved', 'children-differ': 'Different children', wrap: 'Wraps differently', width: 'Width differs', height: 'Height differs', position: 'Moved' };
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

async function diffPair(dir, page, B_ENGINE, available, mutDir = null, cleanKeys = null) {
  const t0 = performance.now();
  const file = (engine, run) => path.join(dir, `${engine}-${run}.json`);
  // A mutant's B is its own single run; the clean B runs come from the base.
  const dirB = mutDir || dir;
  let [A, A2, B, B2] = await Promise.all([
    load(file(A_ENGINE, 'a'), A_ENGINE), load(file(A_ENGINE, 'b'), A_ENGINE),
    load(path.join(dirB, `${B_ENGINE}-a.json`), B_ENGINE), mutDir ? null : load(file(B_ENGINE, 'b'), B_ENGINE),
  ]);
  const [Bc, Bc2] = mutDir ? await Promise.all([load(file(B_ENGINE, 'a'), B_ENGINE), load(file(B_ENGINE, 'b'), B_ENGINE)]) : [null, null];
  // Cause ids: <slug>[@engine]|<kinds>|<selector> (Firefox keeps the bare slug).
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
  // Mutant: dynamic regions of the clean B runs, carried onto the mutated tree.
  const Ms = mutDir && Bc ? matchTrees(Bc, B) : null;
  const maskBc = Bc && Bc2 ? dynamicMask(Bc, Bc2) : new Set();
  const maskB = mutDir ? new Set([...maskBc].map((u) => Ms?.pairOf.get(u)?.uid).filter((u) => u != null)) : (B2 ? dynamicMask(B, B2) : new Set());
  if (!A2 || (!B2 && !Bc2)) result.warnings.push('second run missing: dynamic content is not masked');
  const M = matchTrees(A, B);
  const r = analyze(A, B, M, maskA, maskB);
  // Table columns: when cells change width far more than any text in the
  // table does, while the table keeps its width, the columns were sized
  // differently (table-layout, column rules): one cause on the table, not
  // one per cell. Auto-layout columns that follow their widest text (text
  // advance, K4) stay as they are.
  const tableOf = (n) => { for (let x = n.parent, i = 0; x && i < 6; x = x.parent, i++) if (x.t === 'table') return x; return null; };
  const byTable = new Map();
  for (const c of r.causes) {
    if (c.side !== 'A' || !c.kinds.includes('width') || !/^t[dh]$/.test(c.node.t)) continue;
    const t = tableOf(c.node);
    if (t) { if (!byTable.has(t.uid)) byTable.set(t.uid, { t, cs: [] }); byTable.get(t.uid).cs.push(c); }
  }
  for (const { t, cs } of byTable.values()) {
    const tb = M.pairOf.get(t.uid);
    if (cs.length < 2 || !tb || Math.abs(tb.b[2] - t.b[2]) > TOL.abs) continue;
    const cells = [...subtree(t)].filter((n) => /^t[dh]$/.test(n.t) && M.pairOf.get(n.uid));
    const maxDw = Math.max(...cells.map((n) => Math.abs(M.pairOf.get(n.uid).b[2] - n.b[2])));
    const maxDtw = Math.max(0, ...cells.filter((n) => n.tw && M.pairOf.get(n.uid).tw).map((n) => Math.abs(M.pairOf.get(n.uid).tw - n.tw)));
    if (maxDw <= Math.max(8, 3 * maxDtw + 2)) continue;
    r.causes = r.causes.filter((c) => !cs.includes(c));
    r.causes.push({ side: 'A', node: t, other: tb, kinds: ['columns'], d: r.D.get(t.uid) || null, note: `column widths differ by up to ${Math.round(maxDw)}px while the cells' text differs by at most ${Math.round(maxDtw)}px` });
  }

  // ------------------------------------- evidence (SCIENTIFIC_GROUNDING.md)
  // 4 + 8. third engine families (Blink counts once), best-matching run
  const thirds = [];
  for (const fam of ['gecko', 'webkit']) {
    if (fam === FAMILY[B_ENGINE]) continue;
    const eng = available.find((e) => FAMILY[e] === fam);
    if (!eng) continue;
    const runs = (await Promise.all(['a', 'b'].map((run) => load(file(eng, run), eng)))).filter(Boolean);
    if (!runs.length) continue;
    const T = runs.sort((x, y) => Math.abs(x.meta.docH - A.meta.docH) - Math.abs(y.meta.docH - A.meta.docH))[0];
    thirds.push({ engine: eng, T, M: matchTrees(A, T) });
  }
  // 8. Design intent: relations >= 2 engine families keep, broken by A or B.
  // A violation joins the cause on the same element (or its parent group);
  // otherwise it becomes a cause of its own.
  const engName = { A: A_ENGINE, B: B_ENGINE };
  const intent = intentViolations({ A, B, M, thirds, maskA, maskB, names: engName });
  const idsUp = (n) => { const out = []; for (let x = n, i = 0; x && i < 4; x = x.parent, i++) if (x.id) out.push(x.id); return out; };
  const intentByUid = new Map();
  // Join a cause on the same element or just below it. A "children moved"
  // cause on the PARENT absorbs the violation when two or more of its
  // children break relations (the parent is the cause); a single violating
  // child stays a cause of its own, because intent names the exact element.
  const violatingKids = new Map();
  for (const v of intent.violations) if (v.node.parent) violatingKids.set(v.node.parent.uid, (violatingKids.get(v.node.parent.uid) || 0) + 1);
  const causeAt = (n) => r.causes.find((c) => c.side === 'A' && (c.node === n || c.node.parent === n || c.node.parent?.parent === n
    || (c.node === n.parent && c.kinds.some((k) => k.startsWith('children')) && (violatingKids.get(n.parent.uid) || 0) >= 2)));
  for (const v of intent.violations) {
    v.text = `${engName[v.engine]}: ${selector(v.node)} breaks ${v.kinds.map((k) => INTENT_TEXT[k] || k).join(', ')} with ${v.others} sibling(s) by ${v.mag}px (kept by ${v.holdsIn.join(', ')})`;
    v.ids = idsUp(v.node);
    let c = causeAt(v.node);
    if (!c) {
      c = { side: 'A', node: v.node, other: M.pairOf.get(v.node.uid), kinds: [intentKind(v.kinds)], d: r.D.get(v.node.uid) || null, note: v.text };
      r.causes.push(c);
    }
    if (!intentByUid.has(c.node.uid)) intentByUid.set(c.node.uid, []);
    intentByUid.get(c.node.uid).push(v);
  }
  result.intent = { relations: intent.relations, reason: intent.reason || null, violations: intent.violations.map(({ node, ...v }) => ({ ...v, sel: selector(node) })) };
  // 1. causal probes captured with each run
  const probeFile = (engine, run) => path.join(dir, `${engine}-${run}.probes.json.gz`);
  const [PA, PB] = await Promise.all([loadProbes(probeFile(A_ENGINE, runA), A_ENGINE), loadProbes(path.join(dirB, `${B_ENGINE}-${runB}.probes.json.gz`), B_ENGINE)]);
  const pctx = r.causes.length ? probeContext(A, B, PA, PB, M) : null;
  // 3. the page's own systematic engine model
  const model = calibrate(A, M);
  // 6. ink: screenshots, and this page's own rasterization noise floor
  let pngA = null;
  let pngB = null;
  let floor = null;
  if (r.causes.length) {
    try {
      pngA = PNG.sync.read(await fs.readFile(path.join(dir, `${A_ENGINE}-${runA}.png`)));
      pngB = PNG.sync.read(await fs.readFile(path.join(dirB, `${B_ENGINE}-${runB}.png`)));
      const steady = A.all.filter((a) => { const d = r.D.get(a.uid); const b = M.pairOf.get(a.uid); return d && !d.any && b && !a.hid && (a.tx || a.t === 'img' || a.t === 'svg') && a.b[2] * a.b[3] >= 200 && a.b[2] * a.b[3] <= 150000; });
      const stride = Math.max(1, Math.floor(steady.length / 120));
      floor = noiseFloor(steady.filter((_, i) => i % stride === 0).map((a) => inkAgreement(pngA, pngB, a.b, M.pairOf.get(a.uid).b)));
    } catch (_) { pngA = null; pngB = null; }
  }
  result.evidenceContext = {
    probes: Boolean(pctx), thirds: thirds.map((t) => t.engine), inkFloor: floor,
    calibration: { advance: model.advance, pitch: [...model.pitch].slice(0, 6), controls: [...model.controls] },
  };
  const evidenceOf = (c) => {
    const a = c.side === 'A' ? c.node : null;
    const b = c.side === 'A' ? c.other : c.node;
    const ink = pngA && a && b ? inkAgreement(pngA, pngB, a.b, b.b) : null;
    return {
      mechanism: mechanismOf(c, pctx),
      perception: perceptualEvents(c, M),
      calibration: calibrated(c, model),
      consensus: consensusOf(c, thirds),
      ink: ink && floor ? { agreement: +ink.agreement.toFixed(3), ink: ink.ink, floor: +floor.floor.toFixed(3), observable: ink.agreement < floor.floor } : null,
      intent: (() => {
        const vs = c.side === 'A' ? intentByUid.get(c.node.uid) : null;
        if (!vs) return { verdict: thirds.length ? 'none' : 'unknown' };
        return { verdict: vs.some((v) => v.engine === 'B') ? 'B-violates' : 'A-violates', violations: vs.map((v) => v.text) };
      })(),
    };
  };

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
    B: { src: `${path.basename(dirB)}/${B_ENGINE}-${runB}.png`, ...B.meta.shot },
  };
  result.symptoms = r.symptoms;

  // ---------------------------------------------- findings -> triage
  const ctx = { engines: { A: A_ENGINE, B: B_ENGINE }, D: r.D, vwA: A.meta.vw, vwB: B.meta.vw };
  const seen = new Set();
  const raw = r.causes.map((c) => {
    const keep = (f) => Object.defineProperty(f, '_c', { value: c, enumerable: false }); // the cause, for mutant labels
    const sel = selector(c.node);
    let id = `${idBase}|${c.kinds.join('+')}|${sel}`;
    for (let i = 2; seen.has(id); i++) id = `${idBase}|${c.kinds.join('+')}|${sel}|${i}`;
    seen.add(id);
    const t = triage(c, ctx);
    return keep({
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
      evidence: evidenceOf(c),
    });
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
    // Perception: the cause is as noticeable as its most noticeable instance.
    const seen = inst.reduce((m, f) => ((f.evidence?.perception?.score || 0) > (m?.score || 0) ? f.evidence.perception : m), head.evidence?.perception);
    return {
      ...rest,
      evidence: { ...head.evidence, perception: seen },
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
  if (mutDir) labelMutant(result, raw, { A, B, Bc, Ms, M, maskBc, cleanKeys });
  result.stats.ms = Math.round(performance.now() - t0);
  result.stats.report = result.findings.filter((f) => f.tier === 'report').length;
  result.stats.info = result.findings.length - result.stats.report;
  return result;
}

// ------------------------------------------------------ mutant labels

// What did the injected bug change? Same engine, clean vs mutated: no engine
// noise, so every changed element (outside the clean runs' dynamic regions)
// is an effect of the mutation. A cross-engine cause is POSITIVE when its
// element changed AND sits in the target's neighbourhood (target, 3 levels
// of ancestors and descendants, siblings and their children): page drift
// elsewhere since the base capture can't become a positive.
function labelMutant(result, raw, { A, B, Bc, Ms, M, maskBc, cleanKeys }) {
  const info = { op: B.meta.mutation?.op || null, rule: B.meta.mutation?.rule || null, effective: false, positives: 0, target: null };
  result.mutant = info;
  if (!Bc || !Ms) { info.error = 'clean run of the mutated engine missing'; return; }
  // The targets' clean counterparts (one element, or every element of the
  // mutated class). A hidden target is gone from the mutated tree: take its
  // parent's clean counterpart and the child that vanished.
  const anchors = [];
  for (const t of B.all.filter((n) => n.mut).slice(0, 30)) { const a = Ms.revPair.get(t.uid); if (a) anchors.push(a); }
  if (!anchors.length) {
    for (const p of B.all.filter((n) => n.mutp).slice(0, 30)) {
      const pc = Ms.revPair.get(p.uid);
      if (pc) anchors.push(...(pc.k.filter((k) => !Ms.pairOf.get(k.uid)).length ? pc.k.filter((k) => !Ms.pairOf.get(k.uid)) : [pc]));
    }
  }
  if (!anchors.length) { info.error = 'target not found'; return; }
  info.target = selector(anchors[0]);
  info.scope = anchors.length;
  const changed = new Set();
  for (const n of Bc.all) {
    if (maskBc.has(n.uid)) continue;
    const m = Ms.pairOf.get(n.uid);
    if (!m || deltas(n, m, Ms).any) changed.add(n.uid);
  }
  const hood = new Set();
  const down = (n, depth) => { hood.add(n.uid); if (depth > 0) for (const k of n.k) down(k, depth - 1); };
  for (const anchor of anchors) {
    down(anchor, 3);
    for (let x = anchor.parent, i = 0; x && i < 3; x = x.parent, i++) hood.add(x.uid);
    for (const s of anchor.parent ? anchor.parent.k : []) down(s, 1);
  }
  info.effective = [...hood].some((u) => changed.has(u));
  // Map each cause to the clean B tree: through its B element, or for a
  // baseline-only element through the clean cross-engine matching.
  let Mc = null;
  const toClean = (f) => {
    const bn = f._c.side === 'B' ? f._c.node : f._c.other;
    if (bn) return Ms.revPair.get(bn.uid) || null;
    if (!Mc) Mc = matchTrees(A, Bc);
    return Mc.pairOf.get(f._c.node.uid) || null;
  };
  // A cause the clean diff already has (same kinds, same element) existed
  // before the mutation: never a positive.
  const key = (id) => id.split('|').slice(1).join('|');
  for (const f of raw) {
    const c = toClean(f);
    f.mutLabel = c && hood.has(c.uid) && changed.has(c.uid) && !(cleanKeys && cleanKeys.has(key(f.id))) ? 1 : 0;
  }
  for (const f of result.findings) f.mutLabel = f.memberIds.some((id) => raw.find((x) => x.id === id)?.mutLabel) ? 1 : 0;
  info.positives = result.findings.filter((f) => f.mutLabel).length;
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
  if (page.group === 'mut') continue;
  const engines = (await fs.readdir(d)).map((f) => /^(\w+)-a\.json$/.exec(f)?.[1]).filter((e) => e && e !== A_ENGINE);
  for (const e of engines) pages.push(await diffPair(d, page, e, [A_ENGINE, ...engines]));
}
// Mutants: diffed against their base page's baseline and third engine.
for (const d of dirs) {
  const page = JSON.parse(await fs.readFile(path.join(d, 'page.json'), 'utf8'));
  if (page.group !== 'mut') continue;
  const base = path.join(OUT, page.base);
  const engines = (await fs.readdir(base)).map((f) => /^(\w+)-a\.json$/.exec(f)?.[1]).filter((e) => e && e !== A_ENGINE);
  const clean = pages.find((p) => p.slug === page.base && p.pair === page.engine);
  const cleanKeys = new Set((clean?.findings || []).flatMap((f) => f.memberIds.map((id) => id.split('|').slice(1).join('|'))));
  pages.push(await diffPair(base, page, page.engine, [A_ENGINE, ...engines], d, cleanKeys));
}
const engineRank = (e) => { const i = ENGINE_ORDER.indexOf(e); return i < 0 ? 99 : i; };
pages.sort((x, y) => (GROUP_ORDER[x.group] ?? 9) - (GROUP_ORDER[y.group] ?? 9) || x.slug.localeCompare(y.slug) || engineRank(x.pair) - engineRank(y.pair));

// ------------------------------------------- corpus-level: novelty + judge
// 5. Fingerprint frequencies over the unlabeled corpus (real + own + crawl).
// 7. Every concept votes; a label model learns voter accuracies from the
//    same unlabeled corpus, starting from a prior set by the round-3 rules
//    tier; P(worth reporting) decides the tier. Hard
//    consequences (lib/triage.mjs HARD) are always reported; tau is calibrated
//    on the planted pages below.
const TAU_GRID = [0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.6, 0.7, 0.8, 0.9];
const CORPUS = new Set(['real', 'own', 'crawl']);
const all = pages.filter((p) => !p.error).flatMap((p) => p.findings.map((f) => ({ p, f })));
const counts = new Map();
for (const { p, f } of all) {
  f.fingerprint = fingerprint(f, p.pair);
  if (!CORPUS.has(p.group)) continue;
  if (!counts.has(f.fingerprint)) counts.set(f.fingerprint, { n: 0, pages: new Set() });
  counts.get(f.fingerprint).n++;
  counts.get(f.fingerprint).pages.add(p.slug);
}
const corpusSize = all.filter(({ p }) => CORPUS.has(p.group)).length;
for (const { p, f } of all) {
  f.novelty = surprise(f.fingerprint, counts, corpusSize, p.base || p.slug); // a mutant counts as its base page
  f.votes = votesFor(f, f.novelty);
}
const labelModel = fitLabelModel(all.filter(({ p }) => CORPUS.has(p.group)).map(({ f }) => ({ votes: f.votes, rules: f.tier })));
for (const { f } of all) {
  f.rulesTier = f.tier;
  f.p = +posterior(f.votes, labelModel.pi[f.rulesTier], labelModel.acc).toFixed(3);
}
// The threshold is CALIBRATED on the synthetic ground truth (planted pages:
// known bugs in Firefox, every other page/engine a control): the strictest tau that keeps planted
// recall >= 90% with no Report cause on a control. JUDGE_TAU overrides.
const hardOf = (f) => (f.consequences || []).some((c) => HARD.has(c));
const plantedPages = pages.filter((p) => !p.error && p.group === 'planted');
const plantedDefs = new Map(PAGES.map((d) => [d.slug, d]));
function plantedAt(tau) {
  const rep = (f) => hardOf(f) || f.p >= tau;
  let n = 0; let k = 0; let leaks = 0;
  for (const p of plantedPages) {
    const d = plantedDefs.get(p.slug);
    if (!d) continue;
    if (d.kind === 'planted' && p.pair === 'firefox') {
      for (const g of d.expect) { n++; if (p.findings.some((f) => rep(f) && (f.memberElIds || []).some((id) => g.anyOf.includes(id)))) k++; }
    } else if (d.kind === 'control' || d.kind === 'kb' || (d.kind === 'planted' && p.pair !== 'firefox')) leaks += p.findings.filter(rep).length;
  }
  return { tau, recall: n ? k / n : 1, found: `${k}/${n}`, leaks };
}
const sweep = TAU_GRID.map(plantedAt);
const ok = sweep.filter((x) => x.recall >= 0.9 && x.leaks === 0);
// No tau reaches 90%: take the best recall without control leaks (largest tau on ties).
const best = sweep.filter((x) => x.leaks === 0).sort((x, y) => y.recall - x.recall || y.tau - x.tau)[0];
const TAU = process.env.JUDGE_TAU ? Number(process.env.JUDGE_TAU) : (ok.length ? ok[ok.length - 1].tau : best?.tau ?? 0.5);
// The TIER stays rule-based (PRD 1.3.0 #4): the judge costs recall on the
// planted suite (round 5: 10/24 vs 19/24). P ranks causes; JUDGE=1 makes it
// decide the tier instead.
// The tree model (train/train.py), when trained: P(the cause looks like an
// injected bug rather than natural engine variation). TIER=gbdt lets it
// decide the tier at the tau chosen in cross-validation.
const TIER = process.env.TIER || (process.env.JUDGE === '1' ? 'judge' : 'rules');
let gbdt = null;
let gbdtMeta = null;
try {
  gbdt = loadModel(path.join(HERE, 'models/gbdt.json'));
  gbdtMeta = JSON.parse(await fs.readFile(path.join(HERE, 'models/gbdt-meta.json'), 'utf8'));
  if (gbdt.features.join() !== FEATURE_NAMES.join()) { console.warn('models/gbdt.json: features differ from lib/features.mjs, retrain; model ignored'); gbdt = null; }
} catch (_) { gbdt = null; }
if (TIER === 'gbdt' && !gbdt) throw new Error('TIER=gbdt needs models/gbdt.json (run train/train.py)');
for (const p of pages.filter((x) => !x.error)) {
  for (const f of p.findings) {
    f.judgeTier = hardOf(f) || f.p >= TAU ? 'report' : 'info';
    if (gbdt) {
      f.gbdt = +gbdt.predict(featuresOf(f, p.pair)).toFixed(4);
      f.gbdtTier = hardOf(f) || f.gbdt >= gbdtMeta.tau ? 'report' : 'info';
    }
    f.tier = TIER === 'judge' ? f.judgeTier : TIER === 'gbdt' ? f.gbdtTier : f.rulesTier;
  }
  p.findings.sort((x, y) => (x.tier === y.tier ? 0 : x.tier === 'report' ? -1 : 1) || y.p - x.p || y.impact - x.impact);
  p.stats.report = p.findings.filter((f) => f.tier === 'report').length;
  p.stats.info = p.findings.length - p.stats.report;
}

let versions = {};
try { versions = JSON.parse(await fs.readFile(path.join(OUT, 'engines.json'), 'utf8')); } catch (_) { /* none */ }
// Mutant pages keep only their labeled causes (training rows): thousands
// of mutants would otherwise duplicate every natural cause of their base.
for (const p of pages) if (p.group === 'mut' && !p.error) p.findings = p.findings.filter((f) => f.mutLabel);
const data = { generated: new Date().toISOString(), engines: { A: A_ENGINE }, versions, tol: TOL, gbdt: gbdt ? { tau: gbdtMeta.tau, trained: gbdtMeta.trained, rows: gbdtMeta.rows, cv: gbdtMeta.cv } : null, judge: { tierBy: TIER, tau: TAU, tauSource: process.env.JUDGE_TAU ? 'JUDGE_TAU' : 'planted calibration', sweep, voters: VOTERS, ...labelModel, corpusFingerprints: counts.size }, pages };
await fs.writeFile(path.join(OUT, 'findings.json'), JSON.stringify(data, null, 1));
await fs.writeFile(path.join(OUT, 'report.html'), reportHtml({ ...data, pages: pages.filter((p) => p.group !== 'mut') }));

const filterArg = process.argv.indexOf('--show');
const show = filterArg >= 0 ? process.argv[filterArg + 1] : null;
console.log(`${'page'.padEnd(22)}${'vs'.padEnd(10)}${'group'.padEnd(9)}${'match'.padStart(7)}${'report'.padStart(7)}${'info'.padStart(6)}${'raw'.padStart(5)}${'mask'.padStart(6)}  warnings`);
for (const p of pages) {
  if (p.group === 'mut' && show !== 'mut') continue;
  if (show && !p.slug.includes(show) && p.group !== show) continue;
  if (p.error) { console.log(`${p.slug.padEnd(22)}${p.pair.padEnd(10)}${p.group.padEnd(9)}  ERROR ${p.error}`); continue; }
  const s = p.stats;
  console.log(`${p.slug.padEnd(22)}${p.pair.padEnd(10)}${p.group.padEnd(9)}${(`${(s.matchRate * 100).toFixed(1)}%`).padStart(7)}${String(s.report).padStart(7)}${String(s.info).padStart(6)}${String(s.rawFindings).padStart(5)}${String(s.masked).padStart(6)}  ${p.warnings.join(' | ')}`);
}
const muts = pages.filter((p) => p.group === 'mut' && !p.error && p.mutant);
if (muts.length) {
  const eff = muts.filter((p) => p.mutant.effective);
  const found = eff.filter((p) => p.findings.some((f) => f.mutLabel));
  console.log(`\nmutants: ${muts.length}, effective ${eff.length}, with a labeled positive cause ${found.length}, positives ${muts.reduce((s, p) => s + p.mutant.positives, 0)}`);
}
console.log('\nwrote out/findings.json and out/report.html');
