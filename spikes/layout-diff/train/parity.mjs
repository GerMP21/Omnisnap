// Parity test: lib/gbdt.mjs must reproduce LightGBM's predictions on the
// rows train/train.py wrote to out/parity.csv. Also checks that the model's
// feature names match lib/features.mjs.
//
// usage: node train/parity.mjs

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { FEATURE_NAMES } from '../lib/features.mjs';
import { loadModel } from '../lib/gbdt.mjs';

const HERE = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const OUT = process.env.OMNISNAP_OUT || path.join(HERE, 'out');
const model = loadModel(path.join(HERE, 'models/gbdt.json'));
if (model.features.join() !== FEATURE_NAMES.join()) {
  console.error('FAIL: model feature names differ from lib/features.mjs');
  process.exit(1);
}
const [head, ...lines] = fs.readFileSync(path.join(OUT, 'parity.csv'), 'utf8').trim().split('\n');
const cols = head.split(',');
const iPred = cols.indexOf('pred');
const iFeat = cols.indexOf(FEATURE_NAMES[0]);
let worst = 0;
for (const line of lines) {
  const cells = line.split(',');
  const x = cells.slice(iFeat).map(Number);
  worst = Math.max(worst, Math.abs(model.predict(x) - Number(cells[iPred])));
}
const ok = worst < 1e-9;
console.log(`${ok ? 'PASS' : 'FAIL'}: ${lines.length} rows, ${model.trees} trees, max |JS - LightGBM| = ${worst.toExponential(2)}`);
process.exit(ok ? 0 : 1);
