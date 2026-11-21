# Triage

**Role.** Turn layout-diff's causes into the findings a developer wants: explain the expected engine differences, name what each cause does to the page, put it in **Report** or **Info**, attach the evidence for why it happens and whether it shows, score it, group repeated instances and rank the result. Module `triage`, package [`internal/triage`](https://github.com/GerMP21/Omnisnap/tree/main/internal/triage), with the evidence in [`internal/triage/evidence`](https://github.com/GerMP21/Omnisnap/tree/main/internal/triage/evidence) and the model in [`assets/model`](https://github.com/GerMP21/Omnisnap/tree/main/assets/model).

## Inputs and outputs

- **In:** an [`Analysis`](https://github.com/GerMP21/Omnisnap/blob/main/internal/layoutdiff/analyze.go) and both trees, plus [`Options`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/types.go): the probe context, the third families' trees, the match, ink per cause, the page's calibration, the intent verdicts.
- **Out:** the summary's [`Cause`](https://github.com/GerMP21/Omnisnap/blob/main/internal/schema/summary.go)s, ranked, and per cause the model's feature vector and a reference for `findings.json` ([`BuildWithFeatures`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/group.go)).

## How it works

For every cause, in this order:

### 1. Knowledge base

[`Match`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/kb.go) tries the known engine differences:

| Id | Signature | Expected | Fix |
|---|---|---|---|
| K1 | the line pitch differs by the rounding of a fractional computed line-height, same text, lines and font | yes | whole-pixel line-height |
| K2 | `line-height: normal`, pitch from font metrics | yes | an explicit line-height |
| K4 | a `width` cause, same font and line count, own-text width ratio in (0.95, 1.05), and the text's change accounts for the box's (at most Δtext + 2 px); table cells exempt | yes | none, unless it wraps, clips or overflows |
| K5 | a wrap difference on an unbreakable token (a URL, or 25+ characters without a space) | no | `overflow-wrap: anywhere` or `<wbr>` |
| K8 | a native form control with no explicit size | yes | an explicit control size and `box-sizing` |

An **expected** entry explains the difference away (Info, unless a hard consequence). K5 is not expected: it names the cause and its fix but does not demote it. K4's box-change bound was found by mutation testing: without it, a shrunken paragraph was explained away as text advance ([`TestK4Bound`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/kb_test.go)).

### 2. Consequences

[`consequences`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/consequences.go) names what a person can see change:

| Consequence | When |
|---|---|
| `presence` | `only-in-*` or `children-differ` |
| `visibility` | `hidden-in-*` |
| `clipped` | an overflow on an `overflow: hidden`/`clip` box, with the content less than one viewport beyond it (the viewport width on x, 2000 px on y) |
| `scrollbar` | an overflow on an `auto`/`scroll` box |
| `overflow` | any other overflow: visible, soft |
| `protrusion` | the element goes beyond the viewport in one engine only |
| `truncation` | truncated (ellipsis, line clamp) in one engine only |
| `line-count` | the text wraps onto a different number of lines |
| `interactive-moved` | an interactive, non-inline element (or a child of a `children-moved` parent) moved by 4 px or more |
| `structural` | `columns`, or a size or position change of at least `max(8 px, 10%)`, non-inline |
| `intent-broken` | an `intent-*` kind |
| `content-moved` | `text-shifted` |
| `appearance` | an `appearance` kind |

**Hard** consequences are always Report: `presence`, `visibility`, `clipped`, `scrollbar`, `protrusion`, `truncation` ([`consequences.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/consequences.go)). Visible overflow is soft: in round 4, 45 of 103 corpus Report causes were visible overflow that ink, perception and novelty all called invisible.

### 3. Tier

[`tierOf`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/tier.go), checked in order:

1. a hard consequence → **Report** (never demoted);
2. an expected KB entry → **Info**;
3. **line-box metrics on a media box** → Info: the only kind and only consequence are `appearance`, the element is `img`, `svg`, `video`, `canvas`, `picture` or `iframe`, and the `lh` probe removes the difference. Blink and Firefox lay out the strut around an inline image differently; nothing visible changes. On the 457-site crawl this class was 4,414 of 6,142 Report causes (72%), and demoting it took the rules from 13.4 to 3.8 Report causes per clean page;
4. **deliberate hiding** → Info: the only consequence is `overflow`, and a `hidden`/`clip` box has content pushed a whole viewport beyond it (image replacement: `text-indent: 100%`, `-9999px`);
5. any other consequence → **Report**;
6. no consequence → **Info**.

The rule's weight (3 hard, 2 any consequence, 1 none; halved for an expected KB entry) and a visibility factor (1 above y = 800 px, 0.7 above 2400 px, 0.4 below) feed the ranking.

**The model does not set the tier.** [`ShippedTierTau`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/model.go) reads `tier_tau` from `gbdt-meta.json`; the shipped metadata has none, so the switch in [`group.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/group.go) stays off. Why: [Ground truth and the model](Ground-Truth-and-the-Model).

### 4. Evidence

| Evidence | From | Says |
|---|---|---|
| `mechanism` | [`mechanism.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/evidence/mechanism.go) | which probe removes the difference, tried in the order `lh`, `font`, `controls`, then `all`; `layout` if it survives them all; `author-font` when the font probe removes it but the page asks each engine for a different font; `unknown` when the probes cannot decide ([`ProbeMeaning`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/evidence/mechanism.go)). Why it happens, and it verifies the fix. |
| `perception` | [`perception.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/evidence/perception.go) | relation changes the eye notices, weighted ([`EventWeight`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/evidence/perception.go)): overlap created 3, alignment broken 2.5, order changed 2.5, protrudes parent 2, spacing changed 1, size noticeable 1 (Weber: at least 8% and 2 px) |
| `calibration` | [`calibration.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/evidence/calibration.go) | whether the page's own systematic engine model (text advance ratio, line pitch per typography, control sizes; median and MAD) predicts the change |
| `consensus` | [`consensus.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/evidence/consensus.go) | with a third family: `engine-odd`, `baseline-odd` or `all-differ` |
| `ink` | [Appearance diff](Appearance-Diff) | agreement of the drawn pixels, against the page's noise floor |
| `intent` | [Layout diff](Layout-Diff) | the relations the other families keep and this engine breaks |
| `novelty` | [`novelty.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/novelty.go) | how many crawled sites show the same fingerprint (engine families, mechanism, kind, role, perceptual events), from [`fingerprints.json`](https://github.com/GerMP21/Omnisnap/blob/main/assets/model/fingerprints.json): 300 fingerprints over 457 pages. Surprise = −log2(frequency). A fingerprint the table never saw is **absent**, not zero. |

The summary carries a compact form; `findings.json` carries all of it.

### 5. Score

[`Model`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/model.go) is a LightGBM binary model (82 features, defined to match the spike's `lib/features.mjs`), read from its JSON dump [`gbdt.json`](https://github.com/GerMP21/Omnisnap/blob/main/assets/model/gbdt.json) and evaluated in pure Go: the sigmoid of the summed leaf values. `score` = P(the cause looks like an injected bug rather than natural engine variation). If the dump cannot be read, or its feature list differs from this binary's, the score is `null`: hidden, never guessed ([`ShippedModel`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/model.go)).

### 6. Grouping and ranking

[`group.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/group.go): causes with the same KB entry, or the same **structural signature** (the element, its parent and grandparent by tag and first classes, no ids or positions; the kinds; the change rounded to px), merge into one cause. The headline is the most impactful instance, and the cause is reported if any instance is. Order: tier, then score, then `impact = max(weight × visibility) × (1 + log2(instances))`, then position on the page.

## Tests

- [`TestGoldenTriageParity`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/golden_test.go): the spike's tiers and explanations on the golden captures.
- [`consequences_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/consequences_test.go): consequences, tiers, the image-replacement demotion, the line-height-on-media demotion.
- [`model_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/model_test.go): `TestModelParity` (the same probabilities as LightGBM on [`testdata/model/parity.json`](https://github.com/GerMP21/Omnisnap/blob/main/testdata/model/parity.json)), the metadata's feature names, a model with different features refused.
- [`features_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/features_test.go): the feature vector matches the spike's. [`novelty_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/novelty_test.go): stable fingerprints, a bad table, the shipped table.
- The planted gates on the golden captures: [Ground truth and the model](Ground-Truth-and-the-Model).

## Where the code differs from the spec

- **The knowledge base is Go code, not data.** The spec describes versioned entries in `assets/kb/`, each with a detection signature, an explanation template, a fix, the engine versions it was validated on, and its fixture page. The entries are written in [`kb.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/kb.go), and a [`KB`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/types.go) has an id, title, explanation, fix and whether it is expected. There are no engine-version ranges (the spec's own open question: Safari 27 may lay out fractional line-heights with subpixel precision).
- **No `leaves` library.** The spec evaluates `gbdt.txt` with `github.com/dmitryikh/leaves`. The code evaluates the JSON dump `gbdt.json` with its own evaluator, so there is no dependency; `gbdt.txt` is still embedded ([`assets/model/embed.go`](https://github.com/GerMP21/Omnisnap/blob/main/assets/model/embed.go)) but not read.

## Spec

[`specs/SPEC-triage.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-triage.md).

## Source files

- [`internal/triage/types.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/types.go): the types: `Finding`, `Assessment`, `KB`
- [`internal/triage/group.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/group.go): `Build`: grouping causes into findings
- [`internal/triage/kb.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/kb.go): the knowledge base, `Match`
- [`internal/triage/consequences.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/consequences.go): consequences
- [`internal/triage/tier.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/tier.go): `tierOf`
- [`internal/triage/novelty.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/novelty.go): the novelty table
- [`internal/triage/features.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/features.go): the model's 82 features
- [`internal/triage/model.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/model.go): the tree model evaluator and `Compatible`
- [`internal/triage/evidence/mechanism.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/evidence/mechanism.go): the mechanism, from the probes
- [`internal/triage/evidence/perception.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/evidence/perception.go): perceptual events
- [`internal/triage/evidence/consensus.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/evidence/consensus.go): consensus across engines
- [`internal/triage/evidence/calibration.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/triage/evidence/calibration.go): the perception model, calibrated from the baseline
- [`assets/model/embed.go`](https://github.com/GerMP21/Omnisnap/blob/main/assets/model/embed.go): the shipped model and its metadata
