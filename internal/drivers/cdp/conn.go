// Package cdp drives Blink browsers (Chrome, Chromium, Edge, Brave, Opera,
// Vivaldi) over the Chrome DevTools Protocol, directly: no chromedriver.
package cdp

import (
	"encoding/json"
	"fmt"

	"github.com/GerMP21/Omnisnap/internal/drivers/wsrpc"
)

// codec is CDP's message format: calls carry a sessionId (the event route)
// when they target a page, replies an id with a result or an error.
type codec struct{}

func (codec) Name() string { return "cdp" }

func (codec) Encode(id int64, route, method string, params any) ([]byte, error) {
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if route != "" {
		msg["sessionId"] = route
	}
	return json.Marshal(msg)
}

func (codec) Decode(b []byte) (int64, wsrpc.Reply, wsrpc.Event, bool) {
	var m struct {
		ID        int64           `json:"id"`
		Result    json.RawMessage `json:"result"`
		Error     *rpcError       `json:"error"`
		Method    string          `json:"method"`
		SessionID string          `json:"sessionId"`
		Params    json.RawMessage `json:"params"`
	}
	if json.Unmarshal(b, &m) != nil {
		return 0, wsrpc.Reply{}, wsrpc.Event{}, false
	}
	if m.ID != 0 {
		r := wsrpc.Reply{Result: m.Result}
		if m.Error != nil {
			r.Err = m.Error
		}
		return m.ID, r, wsrpc.Event{}, true
	}
	if m.Method == "" {
		return 0, wsrpc.Reply{}, wsrpc.Event{}, false
	}
	return 0, wsrpc.Reply{}, wsrpc.Event{Method: m.Method, Route: m.SessionID, Params: m.Params}, true
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

func (e *rpcError) Error() string {
	if e.Data != "" {
		return fmt.Sprintf("%s (%d): %s", e.Message, e.Code, e.Data)
	}
	return fmt.Sprintf("%s (%d)", e.Message, e.Code)
}
