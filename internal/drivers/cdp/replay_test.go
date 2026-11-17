package cdp

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const transcriptDir = "../../../testdata/drivers/transcripts/cdp"

// step is one line of a transcript (JSONL): a message the client must send,
// a message the server sends, the server closing the connection, or another
// transcript to include in place.
type step struct {
	Send    json.RawMessage `json:"send,omitempty"`
	Recv    json.RawMessage `json:"recv,omitempty"`
	Close   bool            `json:"close,omitempty"`
	Include string          `json:"include,omitempty"`
}

func loadTranscript(t *testing.T, name string) []step {
	t.Helper()
	f, err := os.Open(filepath.Join(transcriptDir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var steps []step
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var s step
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			t.Fatalf("%s: %v: %s", name, err, line)
		}
		if s.Include != "" {
			steps = append(steps, loadTranscript(t, s.Include)...)
			continue
		}
		steps = append(steps, s)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return steps
}

// replayer is a fake CDP endpoint that plays one transcript. Messages the
// client sends must match the next "send" step (method, sessionId, params;
// the id is free); "recv" steps are then sent with their ids mapped to the
// client's.
type replayer struct {
	t     *testing.T
	steps []step
	srv   *httptest.Server
	done  chan struct{}
	pos   int
}

func replay(t *testing.T, name string) *replayer {
	t.Helper()
	r := &replayer{t: t, steps: loadTranscript(t, name), done: make(chan struct{})}
	r.srv = httptest.NewServer(http.HandlerFunc(r.serve))
	t.Cleanup(r.srv.Close)
	return r
}

// URL is the websocket URL to Dial.
func (r *replayer) URL() string { return "ws" + strings.TrimPrefix(r.srv.URL, "http") }

func (r *replayer) serve(w http.ResponseWriter, req *http.Request) {
	defer close(r.done)
	ws, err := websocket.Accept(w, req, nil)
	if err != nil {
		r.t.Errorf("accept: %v", err)
		return
	}
	ws.SetReadLimit(-1)
	defer func() { _ = ws.CloseNow() }()
	ctx := req.Context()
	ids := map[float64]float64{} // transcript id -> client id
	emit := func() bool {
		for r.pos < len(r.steps) && r.steps[r.pos].Send == nil {
			s := r.steps[r.pos]
			r.pos++
			if s.Close {
				return false
			}
			var m map[string]any
			_ = json.Unmarshal(s.Recv, &m)
			if id, ok := m["id"].(float64); ok {
				cid, ok := ids[id]
				if !ok {
					r.t.Errorf("transcript replies to id %v before it was sent", id)
				}
				m["id"] = cid
			}
			b, _ := json.Marshal(m)
			if err := ws.Write(ctx, websocket.MessageText, b); err != nil {
				return false
			}
		}
		return true
	}
	if !emit() {
		return
	}
	for {
		_, b, err := ws.Read(ctx)
		if err != nil {
			return // the client closed the connection
		}
		if r.pos >= len(r.steps) {
			r.t.Errorf("client sent past the end of the transcript: %s", b)
			continue
		}
		want := r.steps[r.pos].Send
		var got, exp map[string]any
		_ = json.Unmarshal(b, &got)
		_ = json.Unmarshal(want, &exp)
		if !sameCall(got, exp) {
			r.t.Errorf("step %d: client sent\n  %s\nwant\n  %s", r.pos, b, want)
		}
		if id, ok := exp["id"].(float64); ok {
			ids[id], _ = got["id"].(float64)
		}
		r.pos++
		if !emit() {
			return
		}
	}
}

// sameCall compares method, sessionId and params (absent params equal {}).
func sameCall(got, want map[string]any) bool {
	params := func(m map[string]any) any {
		if p, ok := m["params"]; ok {
			return p
		}
		return map[string]any{}
	}
	return got["method"] == want["method"] && got["sessionId"] == want["sessionId"] &&
		reflect.DeepEqual(params(got), params(want))
}

// consumed fails the test unless the whole transcript was played.
func (r *replayer) consumed() {
	r.t.Helper()
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		r.t.Fatal("replayer still running: the client did not close the connection")
	}
	if r.pos != len(r.steps) {
		r.t.Errorf("transcript stopped at step %d of %d", r.pos, len(r.steps))
	}
}
