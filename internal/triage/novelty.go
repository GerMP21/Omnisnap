package triage

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	modelassets "github.com/GerMP21/Omnisnap/assets/model"
)

// Novelty (Task 56): every cause gets a FINGERPRINT (engine families,
// mechanism, kind, element role, perceptual events). Across a crawl,
// fingerprints that occur everywhere (form controls, line-height rounding)
// carry little information; rare ones are surprising. This is inverse document
// frequency for engine differences: surprise = -log2(frequency).
//
// The frequency table is produced by the lab crawl (Task 76) and shipped as
// assets/model/fingerprints.json; without it novelty is absent, never an
// error.

// Fingerprint identifies a cause's class across pages.
func Fingerprint(kinds []string, mechanism, consensus, tag, text string, events []string, pair string) string {
	classed := map[string]bool{}
	for _, k := range kinds {
		classed[kindClass(k)] = true
	}
	var ks []string
	for k := range classed {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	ev := append([]string{}, events...)
	sort.Strings(ev)
	ev = uniqueStrings(ev)
	mech := mechanism
	if mech == "" {
		mech = "unknown"
	}
	return fmt.Sprintf("blink~%s|%s|%s|%s|%s", pair, mech, strings.Join(ks, "+"),
		roleOf(tag, text), joinOr(ev, "+", "no-events"))
}

// Novelty is a cause's cross-corpus surprise.
type Novelty struct {
	Fingerprint string  `json:"fingerprint"`
	Count       int     `json:"count"`
	OtherPages  int     `json:"otherPages"`
	Total       int     `json:"total"`
	Bits        float64 `json:"bits"`
}

// Table is the shipped frequency table.
type Table struct {
	Total        int `json:"total"`
	Fingerprints map[string]struct {
		N     int `json:"n"`
		Pages int `json:"pages"`
	} `json:"fingerprints"`
}

// LoadTable parses assets/model/fingerprints.json. A missing table is not an
// error: novelty is simply absent.
func LoadTable(data []byte) (*Table, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var t Table
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("triage: fingerprints: %w", err)
	}
	return &t, nil
}

// shippedTable is the corpus table, loaded once. An empty table (no crawl yet)
// means novelty is absent.
var shippedTable *Table

// ShippedTable returns the shipped frequency table, or nil when there is none.
func ShippedTable() *Table {
	if shippedTable == nil {
		t, err := LoadTable(modelassets.Fingerprints)
		if err != nil {
			t = nil
		}
		shippedTable = t
	}
	if shippedTable != nil && shippedTable.Total == 0 {
		return nil // the empty table that ships before the first crawl
	}
	return shippedTable
}

// Surprise scores a fingerprint for one page.
//
// A fingerprint the corpus never saw is absent, not zero: "this class occurs
// on no other page" and "this class was never observed" are different numbers,
// and the model was trained on the difference (an absent class is the surprise
// the corpus could not have shown). A class the corpus saw on exactly one page
// (this one) scores zero.
func (t *Table) Surprise(fp, slug string) *Novelty {
	if t == nil {
		return nil
	}
	e, ok := t.Fingerprints[fp]
	if !ok {
		return nil
	}
	// The table stores how many pages show the fingerprint; this page is one of
	// them when it appears at all.
	other := e.Pages
	if other > 0 {
		other--
	}
	bits := 0.0
	if t.Total > 0 {
		bits = -math.Log2(math.Max(float64(e.N), 1) / float64(t.Total))
	}
	return &Novelty{Fingerprint: fp, Count: e.N, OtherPages: other, Total: t.Total, Bits: bits}
}

func uniqueStrings(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func joinOr(xs []string, sep, dflt string) string {
	if len(xs) == 0 {
		return dflt
	}
	return strings.Join(xs, sep)
}
