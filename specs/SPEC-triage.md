# Spec: triage

Module id: `triage` · Package: `internal/triage` · Depends on: `layout-diff`, `appearance-diff` · Milestone: M1 · Conventions: [CONVENTIONS.md](CONVENTIONS.md)

## Objective

Turn "every genuine difference" into "the differences a developer wants": explain expected engine differences, name each cause's visible consequence, split causes into **Report** and **Info**, attach the evidence a developer needs to trust or dismiss each one, group repeated instances, and rank. Port of the spike's `lib/triage.mjs`, the evidence modules of round 4, and the tree model of round 6.

## Behaviour

### 1. Knowledge base (expected engine differences)

Versioned entries in `assets/kb/`, each with an id, a detection signature, an explanation template, a fix, the engine versions it was validated on, and its fixture page.

| Id | Title | Signature (summary) | Fix |
|---|---|---|---|
| K1 | Fractional line-height rounded differently | line pitch differs by the rounding of a fractional computed line-height (e.g. 22.75 px laid out as 22 px) | whole-pixel line-height |
| K2 | `line-height: normal` resolves differently | `lh = normal`, pitch from font metrics | explicit line-height |
| K4 | Text advance differs | `width` cause, same font, same line count, own-text width ratio in (0.95, 1.05), **and the text's change accounts for the box's change** (≤ Δtext + 2 px); table cells exempt (auto columns follow their widest text) | none, unless it wraps, clips or overflows |
| K5 | Long token (URL) breaks differently | wrap difference on an unbreakable token | `overflow-wrap: anywhere` or `<wbr>` |
| K8 | Form-control default size differs | native control, no explicit size | explicit control size and `box-sizing` |

K4's box-change bound was found by mutation testing (round 6): without it, a shrunken paragraph was explained away as text advance.

### 2. Consequences

`presence`, `visibility`, `clipped` (overflow `hidden/clip`, content < one viewport beyond the box), `scrollbar` (overflow `auto/scroll`), `overflow` (visible overflow: soft), `protrusion` (beyond the viewport in one engine), `truncation`, `line-count`, `interactive-moved` (non-inline), `structural` (≥ max(8 px, 10%), non-inline), `intent-broken`, `content-moved` (text shifted inside its box).

**Deliberate hiding:** a `hidden`/`clip` box whose content is pushed a whole viewport beyond it is the image-replacement pattern (the text that is moved out of sight on purpose, `text-indent: 100%`, `-9999px`), not a cut-off. It is the soft `overflow` consequence, never `clipped`, and the threshold is the same one the `clipped` rule uses: the viewport width on the x axis, 2000 px on the y axis (the spike's approximation of "much more than a viewport"; the capture carries no viewport height).

**Hard** (always Report): presence, visibility, clipped, scrollbar, protrusion, truncation. Visible overflow is soft: in round 4, 45 of 103 corpus Report causes were visible overflow that ink, perception and novelty all called invisible.

### 3. Tier (rule-based, PRD 1.3.0 #4)

Report = a hard consequence, or a consequence not explained by an expected KB entry. Info = the rest. The tree model's score does **not** set the tier unless `SPEC-ground-truth.md`'s gate shows the model tier matches the rules' recall with fewer false alarms, and that switch is approved.

One rule-based exception to the tier (2026-11-19): a cause whose **only** consequence is deliberate hiding is Info — nothing was cut off, the content was moved out of sight on purpose, so there is nothing a person can see change. Any other consequence alongside it, and any hard consequence, keeps the cause in Report.

### 4. Evidence per cause

| Evidence | Source | Meaning |
|---|---|---|
| `mechanism` | probes (`capture`) | which neutralizing probe removes the difference: `lh`, `font`, `controls`, `all`; `layout` if it survives all; `author-font` when the font probe removes it but the page asks for different fonts per engine. Says **why**, and verifies the fix |
| `perception` | geometry | relation changes the eye notices (alignment broken, overlap created, order flipped, spacing changed) and Weber's law for sizes (≥ 8% and ≥ 2 px) |
| `calibration` | geometry | the page's own systematic engine model (text advance ratio, line pitch per typography, control sizes; median/MAD) predicts the change |
| `consensus` | third family | which engine is the odd one (engine, baseline, or all differ) |
| `ink` | `appearance-diff` | visible in the registered crops vs the page's noise floor |
| `intent` | `layout-diff` | relations kept by the other families, broken here |
| `novelty` | shipped corpus table | on how many other crawled sites the same cause fingerprint occurs (baseline engine behaviour if ≥ 3) |

### 5. Score (tree model)

`score` = P(the cause looks like an injected bug rather than natural engine variation) from the LightGBM model in `assets/model/gbdt.txt`, evaluated in pure Go (`github.com/dmitryikh/leaves`) on the 82-feature vector defined by the spike's `lib/features.mjs` (identical names, order and encoding). Used for **ranking** and shown in the report. The fingerprint frequency table (`assets/model/fingerprints.json`) ships with the model.

The round-4 weak-supervision judge (label model) is **not ported**: as a tier it cost recall, and the trees supersede it as a score. The one thing it was needed for — the Mozilla social icons, 9 999 px of deliberately hidden text — is covered by the deliberate-hiding rule above, which is a rule and not a score.

### 6. Grouping and ranking

Findings with the same KB entry or the same structural signature (element, parent and grandparent tag + first classes, kinds, rounded deltas) merge into one cause; the headline is the most impactful instance. Rank: tier, then score, then `impact = max(weight × visibility) × (1 + log2 instances)`.

## Commands

```
go test ./internal/triage/...                              # unit, KB fixtures, feature + model parity
go test -tags e2e ./test/e2e/... -run Planted              # planted suite gates
```

## Project structure

`internal/triage/{kb.go, consequences.go, tier.go, evidence/*.go, features.go, model.go, group.go}`; `assets/kb/*.yaml`; `assets/model/{gbdt.txt, gbdt-meta.json, fingerprints.json}`; `testdata/triage/`.

## Code style

Per conventions. KB entries are data, not code: a signature is a small declarative struct evaluated by one matcher. Feature extraction is one function with a test that compares its names to the model metadata (training/serving skew is a build failure).

## Testing strategy

- **KB fixtures:** `14-kb-line-height` and `15-kb-line-height-normal` in every engine: 0 Report causes, 0 unexplained Info causes.
- **Feature parity:** Go features equal the spike's `featuresOf` exactly on the golden findings.
- **Model parity:** Go scores equal LightGBM's predictions within 1e-9 on `parity.csv` (the spike's JS evaluator achieves 3.5e-18).
- **Planted gates** (`SPEC-ground-truth.md`): recall in the Report tier, 0 Report on controls.
- **Unit:** each consequence and each hard/soft decision, including the overflow modes and the image-replacement exception.

## Boundaries

- **Always:** explain every Info cause that has a KB match; keep tiers rule-based until the gate passes and the switch is approved.
- **Ask first:** adding or changing a KB entry; changing the hard set; switching the tier to the model; retraining the shipped model.
- **Never:** hide a hard consequence; let a score demote a hard consequence; use human labels.

## Success criteria

1. Planted suite: 24/24 planted bugs in the Report tier at the right element (Wilson 95% lower bound ≥ 86%); 0 Report causes on controls in all engines; KB fixtures 0 / 0.
2. Feature parity exact; model parity ≤ 1e-9.
3. Median Report causes per real page ≤ 10 (round 5: median 0, max 21).
4. Every Report cause carries at least: consequences, mechanism (when probes ran), consensus (when a third family exists), ink (when on screen).
5. ≤ 300 ms per page pair for triage including the model.

## Open questions

- The tree model's value is pending: round 6's training run is paused (disk space). If the trees don't beat the rules at equal recall, ship without a score or with the score hidden?
- KB entries need engine-version ranges; Safari 27 may lay out fractional line-heights with subpixel precision (unverified): verify on a macOS runner before K1's text ships.
