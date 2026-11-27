package capture

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/GerMP21/Omnisnap/internal/schema"
)

// Backlog B17: a capture whose browser laid the page out but painted none of
// its text (Safari 27.2 on macOS runner image 20261117.0516.1) is a failed
// capture, not something to compare.

// textRow is a page of n text boxes, 200x20 px each, stacked 30 px apart.
func textRow(n int) schema.Node {
	root := schema.Node{T: "body", B: [4]float64{0, 0, 400, float64(30*n + 10)}}
	for i := range n {
		root.K = append(root.K, schema.Node{T: "p", Text: &schema.Text{TX: "text"}, B: [4]float64{10, float64(10 + 30*i), 200, 20}})
	}
	return root
}

// shot is a 400xh PNG on a flat background, with "ink" (a dark bar) in the
// first inked text boxes of textRow.
func shot(t *testing.T, h, inked int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 400, h))
	for y := range h {
		for x := range 400 {
			img.Set(x, y, color.RGBA{0xf6, 0xf5, 0xf0, 0xff})
		}
	}
	for i := range inked {
		for y := 15 + 30*i; y < 25+30*i; y++ {
			for x := 20; x < 120; x++ {
				img.Set(x, y, color.RGBA{0x20, 0x20, 0x20, 0xff})
			}
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCheckPainted(t *testing.T) {
	cases := []struct {
		name         string
		boxes, inked int
		mutate       func(*schema.Node)
		unpainted    bool
	}{
		{name: "every text box painted", boxes: 20, inked: 20},
		{name: "most painted, like a page with collapsed sections", boxes: 20, inked: 15},
		{name: "exactly 10 percent painted", boxes: 20, inked: 2},
		{name: "nothing painted", boxes: 20, inked: 0, unpainted: true},
		{name: "under 10 percent painted", boxes: 30, inked: 2, unpainted: true},
		{name: "fewer than 10 text boxes: not judged", boxes: 9, inked: 0},
		{name: "hidden text does not count", boxes: 20, inked: 0, mutate: func(n *schema.Node) {
			for i := range n.K {
				n.K[i].Hid = true
			}
		}},
		{name: "boxes under 4 px do not count", boxes: 20, inked: 0, mutate: func(n *schema.Node) {
			for i := range n.K {
				n.K[i].B[3] = 3
			}
		}},
		{name: "boxes beyond the screenshot do not count", boxes: 20, inked: 0, mutate: func(n *schema.Node) {
			for i := range n.K {
				n.K[i].B[1] += 10000
			}
		}},
		{name: "elements without own text do not count", boxes: 20, inked: 0, mutate: func(n *schema.Node) {
			for i := range n.K {
				n.K[i].Text = nil
			}
		}},
		{name: "nested text boxes count", boxes: 20, inked: 0, unpainted: true, mutate: func(n *schema.Node) {
			*n = schema.Node{T: "html", B: n.B, K: []schema.Node{*n}}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := textRow(c.boxes)
			if c.mutate != nil {
				c.mutate(&root)
			}
			err := checkPainted(shot(t, int(root.B[3])+10, c.inked), root)
			if got := errors.Is(err, ErrUnpainted); got != c.unpainted {
				t.Fatalf("unpainted = %v (%v); want %v", got, err, c.unpainted)
			}
		})
	}
}

// The message names what was measured, so the error file explains itself.
func TestCheckPaintedMessage(t *testing.T) {
	err := checkPainted(shot(t, 1000, 1), textRow(30))
	if err == nil || !strings.Contains(err.Error(), "painted none of the page's text") || !strings.Contains(err.Error(), "1 of 30 visible text boxes") {
		t.Fatalf("message: %v", err)
	}
}

// Faint marks below the ink threshold (shadows, a 1-level gradient) are not
// text: Safari's blank captures still drew box shadows.
func TestCheckPaintedIgnoresFaintMarks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 400))
	for y := range 400 {
		for x := range 400 {
			// A soft shadow: up to 20 levels darker, under the 24 threshold.
			d := uint8(min(20, (x+y)%21))
			img.Set(x, y, color.RGBA{0xf6 - d, 0xf5 - d, 0xf0 - d, 0xff})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if err := checkPainted(b.Bytes(), textRow(12)); !errors.Is(err, ErrUnpainted) {
		t.Fatalf("faint marks counted as ink: %v", err)
	}
}

func TestCheckPaintedBadPNG(t *testing.T) {
	if err := checkPainted([]byte("not a png"), textRow(20)); err == nil || errors.Is(err, ErrUnpainted) {
		t.Fatalf("a PNG that does not decode must be an ordinary error, got %v", err)
	}
}

// A page whose text is painted captures normally; the same page with a blank
// screenshot fails with ErrUnpainted (the "unpainted" case of
// TestFailureWritesErrorFileOnly checks the files it leaves).
func TestPagePaintedCheck(t *testing.T) {
	f := newFake()
	f.textBoxes, f.ink = 20, true
	if _, err := Page(context.Background(), f, opts(t)); err != nil {
		t.Fatalf("painted page: %v", err)
	}
	f = newFake()
	f.textBoxes = 20
	if _, err := Page(context.Background(), f, opts(t)); !errors.Is(err, ErrUnpainted) {
		t.Fatalf("blank screenshot: %v; want ErrUnpainted", err)
	}
	// The probes cost seconds on a real page: an unpainted capture skips them.
	for _, c := range f.calls {
		if c == "probe apply" {
			t.Errorf("probes ran after an unpainted screenshot: %v", f.calls)
			break
		}
	}
}
