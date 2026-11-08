// Feature vector of one CAUSE for the tree model (train/train.py,
// lib/gbdt.mjs). One function serves both training rows and inference, so
// the two can never drift apart. Every value is a finite number (no NaN):
// a missing measurement is -1, with the evidence's presence as its own
// feature. Dependency-free: ported to Go together with the model.
//
// Deliberately NOT features: anything that names the page, the site or the
// mutation, and the judge's P (derived from the same evidence).

const ROLE = (f) => {
  if (['input', 'select', 'textarea', 'button'].includes(f.tag)) return 'control';
  if (f.tag === 'a') return 'link';
  if (/^h[1-6]$/.test(f.tag)) return 'heading';
  if (['img', 'svg', 'video', 'canvas', 'picture', 'iframe'].includes(f.tag)) return 'media';
  return f.text ? 'text' : 'container';
};
const KIND = (k) => (k.startsWith('only-in-') || k === 'children-differ' ? 'presence' : k.startsWith('hidden-in-') ? 'hidden' : k.startsWith('truncated-in-') ? 'truncated' : k.startsWith('intent-') ? 'intent' : k);
const KINDS = ['presence', 'hidden', 'truncated', 'intent', 'wrap', 'width', 'height', 'position', 'children-moved', 'overflow-x', 'overflow-y', 'text-shifted', 'columns'];
const CONSEQUENCES = ['presence', 'visibility', 'clipped', 'scrollbar', 'overflow', 'protrusion', 'truncation', 'line-count', 'interactive-moved', 'structural', 'intent-broken', 'content-moved'];
const KB = ['K1', 'K2', 'K4', 'K5', 'K8'];
const MECH = ['lh', 'font', 'controls', 'all', 'layout', 'author-font'];
const EVENTS = ['overlap-created', 'alignment-broken', 'order-changed', 'protrudes-parent', 'spacing-changed', 'size-noticeable'];
const CONSENSUS = ['B-odd', 'A-odd', 'all-differ'];
const ROLES = ['control', 'link', 'heading', 'media', 'text', 'container'];
const ENGINES = ['firefox', 'webkit'];

const num = (x, dflt = -1) => (Number.isFinite(x) ? x : dflt);
const log1 = (x) => Math.log1p(Math.max(0, x));

export const FEATURE_NAMES = [
  'count_log', 'side_b', 'dw_abs', 'dh_abs', 'dx_abs', 'dy_abs', 'dln', 'rel_w', 'rel_h', 'area_log', 'top_log', 'first_viewport',
  'severity', 'impact', 'rules_report', 'text_len', 'kb_expected', 'perception_score', 'calibrated', 'ink_present', 'ink_agreement', 'ink_floor',
  'ink_below_floor', 'ink_amount_log', 'intent_b', 'intent_a', 'novelty_other_pages', 'probes_present', 'third_engine',
  ...KINDS.map((k) => `kind_${k}`), ...CONSEQUENCES.map((c) => `cons_${c}`), ...KB.map((k) => `kb_${k}`), ...MECH.map((m) => `mech_${m}`),
  ...EVENTS.map((e) => `event_${e}`), ...CONSENSUS.map((c) => `consensus_${c}`), ...ROLES.map((r) => `role_${r}`), ...ENGINES.map((e) => `engine_${e}`),
];

export function featuresOf(f, pair) {
  const ev = f.evidence || {};
  const box = (f.a || f.b || { b: [0, 0, 0, 0] }).b;
  const d = f.d || {};
  const kinds = new Set((f.kinds || []).map(KIND));
  const cons = new Set(f.consequences || []);
  const events = new Set((ev.perception?.events || []).map((e) => e.type));
  const w = Math.max(1, box[2]);
  const h = Math.max(1, box[3]);
  const v = [
    log1(f.count || 1),
    f.side === 'B' ? 1 : 0,
    Math.abs(num(d.dw, 0)), Math.abs(num(d.dh, 0)), Math.abs(num(d.dx, 0)), Math.abs(num(d.dy, 0)), num(d.dln, 0),
    Math.abs(num(d.dw, 0)) / w, Math.abs(num(d.dh, 0)) / h,
    log1(w * h), log1(box[1]), box[1] < 800 ? 1 : 0,
    num(f.severity, 1), num(f.impact, 0), f.rulesTier === 'report' || (f.rulesTier == null && f.tier === 'report') ? 1 : 0,
    (f.text || '').length, f.kb?.expected ? 1 : 0,
    ev.perception?.score == null ? -1 : ev.perception.score,
    ev.calibration?.explained ? 1 : 0,
    ev.ink ? 1 : 0, ev.ink ? num(ev.ink.agreement) : -1, ev.ink ? num(ev.ink.floor) : -1,
    ev.ink ? (ev.ink.agreement < ev.ink.floor ? 1 : 0) : -1, ev.ink ? log1(num(ev.ink.ink, 0)) : -1,
    ev.intent?.verdict === 'B-violates' ? 1 : 0, ev.intent?.verdict === 'A-violates' ? 1 : 0,
    f.novelty ? num(f.novelty.otherPages, 0) : -1,
    ev.mechanism && ev.mechanism.verdict !== 'unknown' ? 1 : 0,
    ev.consensus && ev.consensus.verdict !== 'unknown' ? 1 : 0,
    ...KINDS.map((k) => (kinds.has(k) ? 1 : 0)),
    ...CONSEQUENCES.map((c) => (cons.has(c) ? 1 : 0)),
    ...KB.map((k) => (f.kb?.id === k ? 1 : 0)),
    ...MECH.map((m) => (ev.mechanism?.verdict === m ? 1 : 0)),
    ...EVENTS.map((e) => (events.has(e) ? 1 : 0)),
    ...CONSENSUS.map((c) => (ev.consensus?.verdict === c ? 1 : 0)),
    ...ROLES.map((r) => (ROLE(f) === r ? 1 : 0)),
    ...ENGINES.map((e) => (pair === e ? 1 : 0)),
  ];
  if (v.length !== FEATURE_NAMES.length) throw new Error(`feature vector ${v.length} != ${FEATURE_NAMES.length} names`);
  return v.map((x) => (Number.isFinite(x) ? Math.round(x * 1e4) / 1e4 : -1));
}
