// Package modelassets ships the trained tree model (Task 63) and its
// metadata. The JSON dump is evaluated by internal/triage; gbdt.txt is the
// native LightGBM text model, kept for provenance and for other tools.
package modelassets

import _ "embed"

// GBDT is LightGBM's dump_model() JSON, 82 features, binary objective.
//
//go:embed gbdt.json
var GBDT []byte

// Meta carries the feature names, the training date, the CV result and the
// probability threshold chosen in cross-validation.
//
//go:embed gbdt-meta.json
var Meta []byte

// Fingerprints is the corpus frequency table (Task 73, Task 76): how many
// pages of the crawl show each cause class. It is what makes novelty a
// cross-corpus number ("this happens on 40% of other sites") instead of a
// per-page guess. The lab crawl writes it; an empty table means novelty is
// absent, never an error.
//
//go:embed fingerprints.json
var Fingerprints []byte

// GBDTText is the native text model (github.com/dmitryikh/leaves reads this
// format; Omnisnap's evaluator reads the JSON dump instead, so no dependency
// is needed).
//
//go:embed gbdt.txt
var GBDTText []byte
