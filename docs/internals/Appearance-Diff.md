# Appearance diff

**Role.** Measure what geometry cannot see. Two jobs: find graphics drawn differently at the same position and size (an image swapped, an SVG fill, a canvas), and give every cause **ink** evidence, which says whether its change is visible in the pixels. Module `appearance-diff`, package [`internal/appearance`](https://github.com/GerMP21/Omnisnap/tree/main/internal/appearance), called from [`pipeline.Run`](https://github.com/GerMP21/Omnisnap/blob/main/internal/pipeline/diff.go).

## Inputs and outputs

- **In:** both engines' run-a screenshots, the matched trees, the deltas and the dynamic masks.
- **Out:** `appearance` causes (one per graphic drawn differently, with its SSIM in the note) and, per cause, an ink measurement against the page's noise floor.

## How it works

### Registration

Each element is cropped at **its own box in each engine's screenshot**: piecewise registration through the matched DOM boxes. A cascade of reflow therefore lines up: a paragraph pushed 40 px down in Firefox is compared with itself, not with what is 40 px below it in Chrome. Crops beyond the screenshot (pages taller than 16,000 px) are skipped. Crops over 400,000 pixels are sampled every second pixel ([`ink.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ink.go)).

### Graphics drawn differently: SSIM

[`Detect`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/graphics.go) looks at every matched element whose tag is a graphic ([`Graphics`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ssim.go): `img`, `svg`, `canvas`, `video`, `picture`, `iframe`), visible on both sides, with **no geometric change** (that is layout-diff's business) and not masked as dynamic. It computes the SSIM of the two crops' luma with a ±3 px offset search, so a one-pixel registration error is not a drawn difference ([`Similarity`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ssim.go)). Below **0.90**, the element gets an `appearance` cause ([`ssim.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ssim.go)). A global SSIM is enough: the crops are single elements, not photographs.

### Ink agreement: evidence

[`InkEvidence`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ink.go):

1. The **ink mask** of a crop is the pixels that differ from its background luma by more than 32. The background is the histogram mode in 4-level bins, so a border or an edge line cannot become the background (the spike's first version used the border median and counted a bordered card's whole interior as ink). The mask is dilated by one pixel in eight directions.
2. **Agreement** = (A's ink near B's ink + B's ink near A's ink) / (A's ink + B's ink).
3. A crop with less than 20 px of ink has nothing drawn: no evidence.

The **noise floor** is per page ([`inkFloor`](https://github.com/GerMP21/Omnisnap/blob/main/internal/pipeline/diff.go), [`NoiseFloor`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ink.go)): the 5th percentile of agreement over up to 120 unchanged, visible text or graphics elements of at least 200 px² that are neither masked nor changed, clamped to [0.5, 0.97]; 0.85 with fewer than 10 samples. A cause is **observable** when its agreement is below the floor. Fonts render slightly differently in every engine, so a fixed threshold would call every text element changed; the floor measures what "unchanged" looks like on this page.

Luma is computed with each product rounded on its own: Go may fuse `x*y+z` into one FMA instruction (it does on arm64), and the unrounded sums moved pixels across the mask threshold (18,109 ink pixels where the spike, in JavaScript, counted 18,082) ([`ink.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ink.go)).

## Failure handling

No screenshot on either side: no appearance causes and no ink evidence. A crop outside the screenshot, or a degenerate one: that element is skipped.

## Tests

- [`ssim_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ssim_test.go): identical crops, a shifted crop (found by the offset search), different content, unrelated noise, a crop outside the screenshot.
- [`ink_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ink_test.go): identical, shifted by one pixel, missing, a bordered box, recoloured, beyond the screenshot, the noise floor, different box widths.
- [`TestInkParityWithTheSpike`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ink_parity_test.go): the same agreement as the spike on golden crops, to the pixel.
- With browsers, [`test/e2e/appearance_test.go`](https://github.com/GerMP21/Omnisnap/blob/main/test/e2e/appearance_test.go) on the planted appearance pages.

## Where the code differs from the spec

- **Background images are not compared.** The spec includes "elements with a background image" in the SSIM step (and keeps it as an open question). The code compares only the graphics tags; the capture does not record background images.
- **No page-level DSSIM state alarm.** The spec computes a page-level DSSIM "only as a state alarm" for engines that rendered different page states. The code has no page-level DSSIM; the different-states warning comes from node counts and page heights ([Layout diff](Layout-Diff), warnings).

## Spec

[`specs/SPEC-appearance-diff.md`](https://github.com/GerMP21/Omnisnap/blob/main/specs/SPEC-appearance-diff.md).

## Source files

- [`internal/appearance/ink.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ink.go): ink: crops, masks and `InkEvidence`
- [`internal/appearance/ssim.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/ssim.go): `Similarity`
- [`internal/appearance/graphics.go`](https://github.com/GerMP21/Omnisnap/blob/main/internal/appearance/graphics.go): graphics whose pixels differ where geometry matched
