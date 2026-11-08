// Evaluate a LightGBM model from its JSON dump (Booster.dump_model()), as the
// Go core will (github.com/dmitryikh/leaves reads the native text model,
// models/gbdt.txt, written next to this dump). Only
// what train/train.py produces: binary objective, numerical splits, no NaN
// in the inputs (lib/features.mjs never emits one). train/parity.mjs checks
// this evaluator against LightGBM's own predictions.

import fs from 'node:fs';

function walk(node, x) {
  while (node.leaf_value === undefined) {
    if (node.decision_type !== '<=') throw new Error(`unsupported split ${node.decision_type}`);
    node = x[node.split_feature] <= node.threshold ? node.left_child : node.right_child;
  }
  return node.leaf_value;
}

export function loadModel(file) {
  const dump = JSON.parse(fs.readFileSync(file, 'utf8'));
  if (dump.objective && !String(dump.objective).startsWith('binary')) throw new Error(`unsupported objective ${dump.objective}`);
  const trees = dump.tree_info.map((t) => t.tree_structure);
  return {
    features: dump.feature_names,
    trees: trees.length,
    // P(label 1) = sigmoid(sum of leaf values)
    predict(x) {
      let raw = 0;
      for (const t of trees) raw += walk(t, x);
      return 1 / (1 + Math.exp(-raw));
    },
  };
}
