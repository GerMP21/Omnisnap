package bidi

import (
	"encoding/json"
	"fmt"
)

// remoteJSON converts a BiDi RemoteValue (script.evaluate's serialization:
// objects as key/value pairs, special numbers as strings) to plain JSON.
// Values JSON cannot hold (nodes, functions, windows) become null.
func remoteJSON(raw json.RawMessage) (string, error) {
	v, err := remoteValue(raw)
	if err != nil {
		return "", fmt.Errorf("bidi: result: %w", err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("bidi: result: %w", err)
	}
	return string(b), nil
}

func remoteValue(raw json.RawMessage) (any, error) {
	var v struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	switch v.Type {
	case "string", "boolean", "bigint": // bigint values are strings already
		var x any
		err := json.Unmarshal(v.Value, &x)
		return x, err
	case "number":
		var special string
		if json.Unmarshal(v.Value, &special) == nil {
			if special == "-0" {
				return 0, nil
			}
			return nil, nil // NaN, Infinity: not JSON (JSON.stringify gives null too)
		}
		var n float64
		err := json.Unmarshal(v.Value, &n)
		return n, err
	case "array", "set":
		var items []json.RawMessage
		if err := json.Unmarshal(v.Value, &items); err != nil {
			return nil, err
		}
		out := make([]any, 0, len(items))
		for _, it := range items {
			x, err := remoteValue(it)
			if err != nil {
				return nil, err
			}
			out = append(out, x)
		}
		return out, nil
	case "object", "map":
		var pairs [][2]json.RawMessage
		if err := json.Unmarshal(v.Value, &pairs); err != nil {
			return nil, err
		}
		out := make(map[string]any, len(pairs))
		for _, p := range pairs {
			key, err := pairKey(p[0])
			if err != nil {
				return nil, err
			}
			x, err := remoteValue(p[1])
			if err != nil {
				return nil, err
			}
			out[key] = x
		}
		return out, nil
	}
	return nil, nil // undefined, null, and values JSON cannot hold
}

// pairKey is an object key (a string) or a map key (a RemoteValue).
func pairKey(raw json.RawMessage) (string, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	k, err := remoteValue(raw)
	if err != nil {
		return "", err
	}
	if str, ok := k.(string); ok {
		return str, nil
	}
	b, err := json.Marshal(k)
	return string(b), err
}
