package groundtruth

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NewHandler serves the planted pages in dir at /<slug>.html (and /<slug>, as
// the spike did). Pages are read once, up front: only listed slugs are
// reachable, so nothing else in dir (or outside it) can be served.
func NewHandler(dir string) (http.Handler, error) {
	pages, err := LoadPlanted(dir)
	if err != nil {
		return nil, err
	}
	html := make(map[string][]byte, len(pages))
	for _, p := range pages {
		b, err := os.ReadFile(filepath.Join(dir, p.Slug+".html"))
		if err != nil {
			return nil, fmt.Errorf("groundtruth: %w", err)
		}
		html[p.Slug] = b
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := html[strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".html")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	}), nil
}

// Server serves the planted suite on a free loopback port.
type Server struct {
	Base string // "http://127.0.0.1:<port>"
	srv  *http.Server
}

// Start serves dir's planted pages on 127.0.0.1 until Close.
func Start(dir string) (*Server, error) {
	h, err := NewHandler(dir)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("groundtruth: listen: %w", err)
	}
	s := &Server{
		Base: "http://" + ln.Addr().String(),
		srv:  &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second},
	}
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "groundtruth: planted server:", err)
		}
	}()
	return s, nil
}

// URL is the address of a planted page.
func (s *Server) URL(slug string) string { return s.Base + "/" + slug + ".html" }

// Close stops the server.
func (s *Server) Close() error { return s.srv.Close() }
