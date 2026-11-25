package schema

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const goldenDir = "../../testdata/golden"

// goldenCaptures lists every golden capture: planted (committed) and real
// (local only, when present).
func goldenCaptures(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(goldenDir, "*", "*", "*-[ab].json"))
	if err != nil || len(files) < 186 {
		t.Fatalf("found %d golden captures, want at least the 186 planted ones: %v", len(files), err)
	}
	return files
}

func TestCaptureGoldenValidates(t *testing.T) {
	s := schemas(t)["capture"]
	for _, f := range goldenCaptures(t) {
		t.Run(goldenName(f), func(t *testing.T) {
			t.Parallel()
			v, err := readJSONValue(f)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Validate(v); err != nil {
				t.Error(err)
			}
		})
	}
}

// goldenName is a capture's path below testdata/golden, e.g. planted/07-font/webkit-a.json.
func goldenName(path string) string {
	rel, err := filepath.Rel(goldenDir, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func TestCaptureGoldenRoundTrip(t *testing.T) {
	for _, f := range goldenCaptures(t) {
		t.Run(goldenName(f), func(t *testing.T) {
			t.Parallel()
			in, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var c Capture
			if err := json.Unmarshal(in, &c); err != nil {
				t.Fatal(err)
			}
			out, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			if !sameJSON(t, in, out) {
				t.Errorf("round trip changed the capture (%d -> %d bytes)", len(in), len(out))
			}
		})
	}
}

func TestCaptureLineHeight(t *testing.T) {
	for _, tc := range []struct {
		json string
		want LineHeight
	}{
		{`"normal"`, LineHeight{Normal: true}},
		{`24`, LineHeight{PX: 24}},
		{`28.08`, LineHeight{PX: 28.08}},
	} {
		var lh LineHeight
		if err := json.Unmarshal([]byte(tc.json), &lh); err != nil || lh != tc.want {
			t.Errorf("%s: got %+v, %v; want %+v", tc.json, lh, err, tc.want)
		}
		b, err := json.Marshal(lh)
		if err != nil || string(b) != tc.json {
			t.Errorf("%+v marshals to %s, %v; want %s", lh, b, err, tc.json)
		}
	}
	var lh LineHeight
	if err := json.Unmarshal([]byte(`"tall"`), &lh); err == nil {
		t.Error(`"tall": want an error`)
	}
}

func TestCaptureFlag(t *testing.T) {
	n := Node{T: "a", IA: true, B: [4]float64{0, 0, 10, 10}, D: "inline"}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"ia":1`)) || bytes.Contains(b, []byte(`"hid"`)) {
		t.Errorf("%s: want ia as 1 and no hid", b)
	}
	var back Node
	if err := json.Unmarshal(b, &back); err != nil || !back.IA || back.Hid {
		t.Errorf("unmarshal %s: %+v, %v", b, back, err)
	}
}

// nt (change fix-selector-sibling-index) is written only when set, so a node
// that is the only one of its tag, and every older capture, has none.
func TestCaptureSiblingPosition(t *testing.T) {
	for _, tc := range []struct {
		n    Node
		want string
	}{
		{Node{T: "li", NT: 3, B: [4]float64{0, 0, 10, 10}, D: "list-item"}, `"nt":3`},
		{Node{T: "main", B: [4]float64{0, 0, 10, 10}, D: "block"}, ""},
	} {
		b, err := json.Marshal(tc.n)
		if err != nil {
			t.Fatal(err)
		}
		if got := bytes.Contains(b, []byte(`"nt"`)); got != (tc.want != "") || (tc.want != "" && !bytes.Contains(b, []byte(tc.want))) {
			t.Errorf("%s: want %q", b, tc.want)
		}
		var back Node
		if err := json.Unmarshal(b, &back); err != nil || back.NT != tc.n.NT {
			t.Errorf("unmarshal %s: NT %d, %v; want %d", b, back.NT, err, tc.n.NT)
		}
	}
}
