package discovery

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

func mathFloat64(bits uint64) float64 { return math.Float64frombits(bits) }

func utf16Decode(u []uint16) []rune { return utf16.Decode(u) }

// xmlPlistDecoder decodes an XML plist. encoding/xml's token stream is enough:
// a plist is nested elements, and only a few types appear in Info.plist.
type xmlPlistDecoder struct{ dec *xml.Decoder }

func newXMLPlistDecoder(b []byte) *xmlPlistDecoder {
	return &xmlPlistDecoder{dec: xml.NewDecoder(bytes.NewReader(b))}
}

// value returns the first value inside <plist>.
func (d *xmlPlistDecoder) value() (plistValue, error) {
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return nil, fmt.Errorf("discovery: plist: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "plist" {
			continue
		}
		return d.element(se)
	}
}

func (d *xmlPlistDecoder) element(se xml.StartElement) (plistValue, error) {
	switch se.Name.Local {
	case "dict":
		out := map[string]plistValue{}
		key := ""
		for {
			tok, err := d.dec.Token()
			if err != nil {
				return nil, fmt.Errorf("discovery: plist: %w", err)
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					key = d.text(t)
					continue
				}
				v, err := d.element(t)
				if err != nil {
					return nil, err
				}
				if key != "" {
					out[key] = v
					key = ""
				}
			case xml.EndElement:
				if t.Name.Local == "dict" {
					return out, nil
				}
			}
		}
	case "array":
		var out []plistValue
		for {
			tok, err := d.dec.Token()
			if err != nil {
				return nil, fmt.Errorf("discovery: plist: %w", err)
			}
			switch t := tok.(type) {
			case xml.StartElement:
				v, err := d.element(t)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			case xml.EndElement:
				if t.Name.Local == "array" {
					return out, nil
				}
			}
		}
	case "string":
		return d.text(se), nil
	case "integer":
		n, err := strconv.ParseInt(strings.TrimSpace(d.text(se)), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("discovery: plist: integer: %w", err)
		}
		return n, nil
	case "real":
		f, err := strconv.ParseFloat(strings.TrimSpace(d.text(se)), 64)
		if err != nil {
			return nil, fmt.Errorf("discovery: plist: real: %w", err)
		}
		return f, nil
	case "true", "false":
		_ = d.skip(se)
		return se.Name.Local == "true", nil
	case "data", "date":
		return d.text(se), nil
	}
	return nil, fmt.Errorf("discovery: plist: unsupported element <%s>", se.Name.Local)
}

// text reads an element's character data and its end tag.
func (d *xmlPlistDecoder) text(se xml.StartElement) string {
	var b strings.Builder
	for {
		tok, err := d.dec.Token()
		if err != nil {
			if err == io.EOF {
				return b.String()
			}
			return b.String()
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			if t.Name.Local == se.Name.Local {
				return b.String()
			}
		}
	}
}

func (d *xmlPlistDecoder) skip(se xml.StartElement) error {
	for {
		tok, err := d.dec.Token()
		if err != nil {
			return err
		}
		if end, ok := tok.(xml.EndElement); ok && end.Name.Local == se.Name.Local {
			return nil
		}
	}
}
