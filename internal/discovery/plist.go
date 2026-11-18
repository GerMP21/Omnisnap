package discovery

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Info.plist parsing (SPEC-discovery.md, Task 32): XML plist and binary
// ("bplist00") — macOS ships Chrome's and Safari's Info.plist in either form
// depending on the release. Only what discovery needs is decoded: a top-level
// dictionary of strings, integers, booleans and arrays.

// plistValue is a decoded plist value.
type plistValue = any

// parsePlist decodes an XML or binary plist.
func parsePlist(b []byte) (map[string]plistValue, error) {
	switch {
	case bytes.HasPrefix(bytes.TrimSpace(b), []byte("<?xml")) || bytes.HasPrefix(bytes.TrimSpace(b), []byte("<plist")):
		return parseXMLPlist(b)
	case bytes.HasPrefix(b, []byte("bplist00")):
		return parseBinaryPlist(b)
	default:
		return nil, fmt.Errorf("discovery: plist: unknown format")
	}
}

// parseXMLPlist decodes the subset used by Info.plist.
func parseXMLPlist(b []byte) (map[string]plistValue, error) {
	dec := newXMLPlistDecoder(b)
	v, err := dec.value()
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]plistValue)
	if !ok {
		return nil, fmt.Errorf("discovery: plist: top-level value is not a dictionary")
	}
	return m, nil
}

// parseBinaryPlist decodes bplist00. Object references, offsets and the
// trailer follow CoreFoundation's format (see "Binary Property Lists").
func parseBinaryPlist(b []byte) (map[string]plistValue, error) {
	if len(b) < 8+32 || !bytes.HasPrefix(b, []byte("bplist00")) {
		return nil, fmt.Errorf("discovery: bplist: too short")
	}
	trailer := b[len(b)-32:]
	offsetSize := int(trailer[6])
	refSize := int(trailer[7])
	numObjects := int(binary.BigEndian.Uint64(trailer[8:]))
	top := int(binary.BigEndian.Uint64(trailer[16:]))
	tableOffset := int(binary.BigEndian.Uint64(trailer[24:]))
	if offsetSize < 1 || refSize < 1 || numObjects < 1 || tableOffset < 8 || tableOffset+numObjects*offsetSize > len(b) {
		return nil, fmt.Errorf("discovery: bplist: bad trailer")
	}
	offsets := make([]int, numObjects)
	for i := range numObjects {
		p := tableOffset + i*offsetSize
		offsets[i] = int(readBE(b[p : p+offsetSize]))
	}
	d := &binaryPlist{b: b, offsets: offsets, refSize: refSize, seen: map[int]bool{}}
	v, err := d.object(top, 0)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]plistValue)
	if !ok {
		return nil, fmt.Errorf("discovery: bplist: top-level value is not a dictionary")
	}
	return m, nil
}

type binaryPlist struct {
	b       []byte
	offsets []int
	refSize int
	seen    map[int]bool // recursion guard
}

func readBE(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}

func (d *binaryPlist) object(ref, depth int) (plistValue, error) {
	if ref < 0 || ref >= len(d.offsets) || depth > 64 {
		return nil, fmt.Errorf("discovery: bplist: bad object ref %d", ref)
	}
	p := d.offsets[ref]
	if p >= len(d.b) {
		return nil, fmt.Errorf("discovery: bplist: object %d out of range", ref)
	}
	marker := d.b[p]
	kind, info := marker>>4, int(marker&0x0f)
	p++
	switch kind {
	case 0x0: // null, false, true, fill
		switch info {
		case 0x8:
			return false, nil
		case 0x9:
			return true, nil
		default:
			return nil, nil
		}
	case 0x1: // int
		n := 1 << info
		if p+n > len(d.b) {
			return nil, fmt.Errorf("discovery: bplist: int out of range")
		}
		v := int64(readBE(d.b[p : p+n]))
		if info >= 3 { // signed 64-bit
			return v, nil
		}
		return v, nil
	case 0x2: // real
		n := 1 << info
		if p+n > len(d.b) {
			return nil, fmt.Errorf("discovery: bplist: real out of range")
		}
		switch n {
		case 4:
			return float64(binary.BigEndian.Uint32(d.b[p:])), nil
		case 8:
			return mathFloat64(binary.BigEndian.Uint64(d.b[p:])), nil
		}
		return nil, nil
	case 0x5: // ASCII string
		s, np, err := d.count(info, p)
		if err != nil {
			return nil, err
		}
		return string(d.b[np : np+s]), nil
	case 0x6: // UTF-16BE string
		s, np, err := d.count(info, p)
		if err != nil {
			return nil, err
		}
		raw := d.b[np : np+s*2]
		u := make([]uint16, len(raw)/2)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(raw[i*2:])
		}
		return string(utf16Decode(u)), nil
	case 0xA: // array
		s, np, err := d.count(info, p)
		if err != nil {
			return nil, err
		}
		var out []plistValue
		for i := range s {
			ref := int(readBE(d.b[np+i*d.refSize : np+(i+1)*d.refSize]))
			v, err := d.object(ref, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case 0xD: // dict
		s, np, err := d.count(info, p)
		if err != nil {
			return nil, err
		}
		out := map[string]plistValue{}
		for i := range s {
			kr := int(readBE(d.b[np+i*d.refSize : np+(i+1)*d.refSize]))
			vr := int(readBE(d.b[np+(s+i)*d.refSize : np+(s+i+1)*d.refSize]))
			kv, err := d.object(kr, depth+1)
			if err != nil {
				return nil, err
			}
			key, ok := kv.(string)
			if !ok {
				continue
			}
			v, err := d.object(vr, depth+1)
			if err != nil {
				return nil, err
			}
			out[key] = v
		}
		return out, nil
	}
	return nil, fmt.Errorf("discovery: bplist: unsupported object type 0x%x", marker)
}

// count decodes a length, inline or as a following integer object.
func (d *binaryPlist) count(info, p int) (n, next int, err error) {
	if info != 0x0f {
		return info, p, nil
	}
	if p >= len(d.b) {
		return 0, 0, fmt.Errorf("discovery: bplist: length out of range")
	}
	m := d.b[p]
	if m>>4 != 0x1 {
		return 0, 0, fmt.Errorf("discovery: bplist: bad length marker 0x%x", m)
	}
	size := 1 << (m & 0x0f)
	if p+1+size > len(d.b) {
		return 0, 0, fmt.Errorf("discovery: bplist: length out of range")
	}
	return int(readBE(d.b[p+1 : p+1+size])), p + 1 + size, nil
}
