// Package bidi drives Gecko (Firefox) over WebDriver BiDi, directly: no
// geckodriver. Firefox serves BiDi on --remote-debugging-port (its CDP is
// gone since Firefox 129).
package bidi

import (
	"encoding/json"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/drivers/wsrpc"
)

// codec is BiDi's message format: every message has a type (success, error
// or event); there are no per-target sessions, so events have no route.
type codec struct{}

func (codec) Name() string { return "bidi" }

func (codec) Encode(id int64, _ string, method string, params any) ([]byte, error) {
	if params == nil {
		params = map[string]any{} // BiDi requires the params object
	}
	return json.Marshal(map[string]any{"id": id, "method": method, "params": params})
}

func (codec) Decode(b []byte) (int64, wsrpc.Reply, wsrpc.Event, bool) {
	var m struct {
		Type    string          `json:"type"`
		ID      int64           `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   string          `json:"error"`
		Message string          `json:"message"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if json.Unmarshal(b, &m) != nil {
		return 0, wsrpc.Reply{}, wsrpc.Event{}, false
	}
	switch m.Type {
	case "success":
		return m.ID, wsrpc.Reply{Result: m.Result}, wsrpc.Event{}, m.ID != 0
	case "error":
		return m.ID, wsrpc.Reply{Err: &bidiError{Code: m.Error, Message: m.Message}}, wsrpc.Event{}, m.ID != 0
	case "event":
		return 0, wsrpc.Reply{}, wsrpc.Event{Method: m.Method, Params: m.Params}, true
	}
	return 0, wsrpc.Reply{}, wsrpc.Event{}, false
}

// bidiError is a WebDriver error: a code ("invalid argument", "unknown
// error", ...) and the browser's message.
type bidiError struct {
	Code    string
	Message string
}

func (e *bidiError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
