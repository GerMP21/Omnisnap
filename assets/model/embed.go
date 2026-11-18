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

// GBDTText is the native text model (github.com/dmitryikh/leaves reads this
// format; Omnisnap's evaluator reads the JSON dump instead, so no dependency
// is needed).
//
//go:embed gbdt.txt
var GBDTText []byte
