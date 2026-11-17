// Package schema holds the Go types of Omnisnap's public contracts
// (SPEC-bundle-schema.md). The JSON Schemas in schema/ are the source of
// truth: change the schema first, then these types; tests check that the
// types marshal to schema-valid JSON.
package schema

// IDPrefix is where the schemas are published: a schema's $id is
// IDPrefix + "<name>/" + Major + ".json".
const IDPrefix = "https://omnisnap.org/schema/"

// Major is the schema major version written by this binary. Additive fields
// keep it; removals or meaning changes need a new major.
const Major = "1"

// ID returns the $id (and $schema value) of the named schema.
func ID(name string) string { return IDPrefix + name + "/" + Major + ".json" }
