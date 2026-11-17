// Package schematest validates JSON against Omnisnap's schemas in tests of
// other packages. Test-only: the shipped binary never imports it (its
// validator is a test dependency).
package schematest

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

var (
	once     sync.Once
	compiled map[string]*jsonschema.Schema
	loadErr  error
)

func schemaDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "../../../schema")
}

func load() {
	files, err := filepath.Glob(filepath.Join(schemaDir(), "*.schema.json"))
	if err != nil {
		loadErr = err
		return
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	names := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			loadErr = err
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			loadErr = err
			return
		}
		name := strings.TrimSuffix(filepath.Base(f), ".schema.json")
		if err := c.AddResource(schema.ID(name), doc); err != nil {
			loadErr = err
			return
		}
		names[name] = schema.ID(name)
	}
	compiled = map[string]*jsonschema.Schema{}
	for name, id := range names {
		s, err := c.Compile(id)
		if err != nil {
			loadErr = err
			return
		}
		compiled[name] = s
	}
}

// Validate fails the test unless data is valid against the named schema
// ("capture", "summary", ...).
func Validate(t testing.TB, name string, data []byte) {
	t.Helper()
	once.Do(load)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	s, ok := compiled[name]
	if !ok {
		t.Fatalf("no schema %q", name)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if err := s.Validate(doc); err != nil {
		t.Errorf("not a valid %s: %v", name, err)
	}
}
