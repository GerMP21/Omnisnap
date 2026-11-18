# Spec Delta: mutant policy and mutant labels

Status: implemented (2026-11-18)

Original: [SPEC-ground-truth.md](SPEC-ground-truth.md) §2 "Mutation testing" and
§4 "Dataset, training, evaluation". Only the areas below change; everything
else inherits.

## Objective

Resolve two decisions from the 2026-11-18 review of Tasks 70–71:

1. What the lab does with a mutant that moves nothing.
2. How a finding in `findings.json` is joined to a mutant's positive label.

Both decisions exist because the first implementation made the dataset depend
on a retry rule (which silently changes which bug a seed produces) and on
selector strings (which drift when a hidden or reordered sibling changes
`:nth-of-type`).

## Current behavior

- `omnisnap-lab mutate` applies one mutant, extracts, reverts, and — when the
  label finds no change in the target's neighbourhood — rotates to the next
  operator and retries, up to the 19 operators. `meta.mutation.attempts`
  records how many rotations were needed. A mutant that never moves anything
  is written as an error. On the planted suite this replaced 3 of 16 mutants.
- `MutantLabel.Positives` are the clean tree's selectors; `PositiveCauses`
  joins a cause to a positive by comparing `schema.Cause.Selector` and
  `Instances` with those strings.
- `findings.json` causes carry no element identity, so nothing in the bundle
  can say "this cause is the element the label marked".

## New behavior

### 1. Record every mutant (option B is the default)

- Each scheduled mutant is applied **once**: the operator is `ops[seed %
  len(ops)]`, fixed by the seed. If it moves nothing, the mutant is still
  captured and recorded; there is no fallback operator and no error.
- The lab writes `labels.json` with `effective: false`, `changed: 0`,
  `positives: []` for such a mutant. `attempts` stays absent/1.
- `labels.json` carries both forms of the answer: `positives` (selectors, for
  people) and `positiveUids` (the clean capture's node ids, sorted, the exact
  join the dataset uses). Uids refer to the on-disk clean run of the same
  engine, which is the file the dataset will load too.
- Determinism is the point: the same page and seed produce the same operator
  on every run, so a dataset can be regenerated from a capture set.

### 2. Option A stays available behind a flag

- `--require-effective` (lab) and `MutateOptions.RequireEffective` (API) turn
  the current rotation back on: try the next operator until the label sees a
  change, then record `attempts`. If no operator moves the page, the mutant is
  an error, as today.
- The flag is documented as "training data only": it biases the set toward
  bugs this labeler can see.

### 3. Dataset rows for ineffective mutants

- An ineffective mutant contributes label-0 rows for the causes the diff
  reports on it (they are all "not caused by the injection") and is counted
  separately in the dataset metadata (`mutantsIneffective`).
- It is not silently dropped: dropping by effectiveness is the same
  circularity the retry rule had, one step later.

### 4. Labels join by node identity (option B)

- `findings.json` causes gain two findings-only fields: `side` (`"A"` for the
  baseline engine, `"B"` for the compared engine) and `nodeId` (the node's
  `layoutdiff` uid in that side's capture of the pair). They are **not** added
  to `schema.Cause`/`summary.json`, so the published surfaces do not change.
- `MutantLabel` exposes the clean tree's positive uids.
- A cause maps to the clean tree exactly:
  - side B → the same-engine match of the mutant against its clean run
    (`MatchTrees(clean, mutant).RevPair[nodeId]`);
  - side A → the cross-engine match the spike uses
    (`MatchTrees(baselineClean, cleanMutatedEngine).PairOf[nodeId]`).
- The selector join remains only as a fallback for bundles written before
  this change, and the lab logs when it falls back.

## Success criteria

1. `go test -tags browsers ./internal/groundtruth/... -run Mutate` (default)
   records 4 mutants per page and engine with no retries and no error, even
   when an operator moves nothing; the same test with `RequireEffective: true`
   records 16/16 effective on two pages and two engines.
2. A unit test maps a side-A cause and a side-B cause to the clean tree by
   node id, and a hidden target still yields exactly one positive.
3. `labels.json` always carries `op`, `target`, `scope`, `effective`,
   `changed`, `positives`, `positiveUids`, and `attempts` only when retries
   happened. Two runs of the same label write identical bytes.
4. Two runs of `omnisnap-lab mutate` on the same bundle set choose the same
   operator for every seed.
5. `schema/summary.schema.json` is unchanged; `go test ./internal/schema/`
   passes with the findings fixtures.

## Boundaries

- Always: `mutate.js` stays byte-identical (pinned by `SHA256SUMS`; changing
  an injected script needs approval).
- Ask first: adding `side`/`nodeId` to any shared schema, or changing the
  dataset's label rule for ineffective mutants.
- Never: silently substitute an operator in the default mode; drop an
  ineffective mutant without a counter.

## Out of scope (deferred)

- Re-capturing the existing mutant bundles under the new default.
- Training or the model decision (Task 76).
- Extending `nodeId` to `summary.json` or the viewer.

## Open questions

- The lab's mutate command currently takes `--bundles`; SPEC-ground-truth §2's
  command list still shows `--per-engine 3 --resume --data`. Align the spec's
  command block with the implemented flags in this change, or keep both forms?
- Should `labels.json` also carry the *cause ids* (not only element uids)? A
  cause id is `slugBase|kinds|selector` and only exists after a diff of the
  mutant, which nothing produces yet. The uids answer the identity need; the
  ids are worth adding once the lab can diff mutant bundles (asked 2026-11-18).
