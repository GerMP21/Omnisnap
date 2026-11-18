package process

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Helpers shared by the WebDriver-driven families (WebKitGTK, Safari).

// freePort picks a port a driver can bind.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("process: free port: %w", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// driverStatus is what GET /status answers.
type driverStatus struct {
	Value struct {
		Ready   bool   `json:"ready"`
		Message string `json:"message"`
	} `json:"value"`
}

// waitDriver polls the driver's /status until it is ready (the spike's
// 60-iteration loop, 100 ms apart).
func waitDriver(ctx context.Context, base string) error {
	var last error
	for range 60 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/status", nil)
		if err == nil {
			var res *http.Response
			if res, err = http.DefaultClient.Do(req); err == nil {
				var st driverStatus
				err = json.NewDecoder(res.Body).Decode(&st)
				_ = res.Body.Close()
				if err == nil && st.Value.Ready {
					return nil
				}
				if err == nil && st.Value.Message != "" {
					return fmt.Errorf("process: driver not ready: %s", st.Value.Message)
				}
			}
		}
		last = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("process: driver not ready: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("process: driver not ready: %v", last)
}
