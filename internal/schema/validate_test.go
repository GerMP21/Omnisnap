package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	schemaDir  = "../../schema"
	fixtureDir = "../../testdata/schema"
)

var (
	compileOnce sync.Once
	compiled    map[string]*jsonschema.Schema
	compileErr  error
)

// schemas compiles every schema/*.schema.json. Compiling checks each against
// the draft 2020-12 meta-schema; schemas are registered under their $id so
// cross-schema $refs resolve locally, never over the network.
func schemas(t *testing.T) map[string]*jsonschema.Schema {
	t.Helper()
	compileOnce.Do(func() { compiled, compileErr = compileAll() })
	if compileErr != nil {
		t.Fatal(compileErr)
	}
	return compiled
}

func compileAll() (map[string]*jsonschema.Schema, error) {
	files, err := filepath.Glob(filepath.Join(schemaDir, "*.schema.json"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("no schemas in %s: %v", schemaDir, err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	ids := map[string]string{}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".schema.json")
		doc, err := readJSONValue(f)
		if err != nil {
			return nil, err
		}
		m, _ := doc.(map[string]any)
		if got := m["$schema"]; got != "https://json-schema.org/draft/2020-12/schema" {
			return nil, fmt.Errorf("%s: $schema %v, want draft 2020-12", f, got)
		}
		id := IDPrefix + name + "/" + Major + ".json"
		if got := m["$id"]; got != id {
			return nil, fmt.Errorf("%s: $id %v, want %s", f, got, id)
		}
		if err := c.AddResource(id, doc); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		ids[name] = id
	}
	out := map[string]*jsonschema.Schema{}
	for name, id := range ids {
		s, err := c.Compile(id)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", name, err)
		}
		out[name] = s
	}
	return out, nil
}

func TestSchemasCompile(t *testing.T) {
	got := schemas(t)
	for _, name := range []string{"job", "progress", "host", "capture", "summary"} {
		if got[name] == nil {
			t.Errorf("schema/%s.schema.json missing", name)
		}
	}
}

// Every schema has valid and invalid fixtures (SPEC-bundle-schema.md,
// "Testing strategy"): testdata/schema/<name>/{valid,invalid}/*.json.
func TestFixtures(t *testing.T) {
	all := schemas(t)
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, wantValid := range []bool{true, false} {
			sub := map[bool]string{true: "valid", false: "invalid"}[wantValid]
			files, _ := filepath.Glob(filepath.Join(fixtureDir, name, sub, "*.json"))
			if len(files) == 0 {
				t.Errorf("%s: no %s fixtures in %s", name, sub, filepath.Join(fixtureDir, name, sub))
			}
			for _, f := range files {
				t.Run(name+"/"+sub+"/"+filepath.Base(f), func(t *testing.T) {
					v, err := readJSONValue(f)
					if err != nil {
						t.Fatal(err)
					}
					err = all[name].Validate(v)
					if wantValid && err != nil {
						t.Errorf("want valid, got: %v", err)
					}
					if !wantValid {
						if err == nil {
							t.Error("want invalid, but it validates")
						} else {
							t.Logf("rejected as expected: %v", err) // -v shows why: check it is the planted reason
						}
					}
				})
			}
		}
	}
}

// validate marshals v and validates it against the named schema.
func validate(t *testing.T, name string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if err := schemas(t)[name].Validate(doc); err != nil {
		t.Errorf("%s: %s\ndoes not validate: %v", name, b, err)
	}
}

func readJSONValue(path string) (any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	v, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}
