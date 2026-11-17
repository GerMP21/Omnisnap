package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestMainExitCodesAndOutput(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string // substring; "" means stdout must be empty
		wantStderr string // substring; "" means stderr must be empty
	}{
		{
			name:       "version prints the version",
			args:       []string{"omnisnap", "version"},
			wantCode:   ExitOK,
			wantStdout: "omnisnap " + Version + "\n",
		},
		{
			name:       "no arguments is a usage error",
			args:       []string{"omnisnap"},
			wantCode:   ExitUsage,
			wantStderr: "usage: omnisnap",
		},
		{
			name:       "unknown command is a usage error",
			args:       []string{"omnisnap", "frobnicate"},
			wantCode:   ExitUsage,
			wantStderr: `unknown command "frobnicate"`,
		},
		{
			name:       "version takes no arguments",
			args:       []string{"omnisnap", "version", "extra"},
			wantCode:   ExitUsage,
			wantStderr: "version takes no arguments",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Main(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			assertOutput(t, "stdout", stdout.String(), tt.wantStdout)
			assertOutput(t, "stderr", stderr.String(), tt.wantStderr)
		})
	}
}

func assertOutput(t *testing.T, stream, got, want string) {
	t.Helper()
	if want == "" {
		if got != "" {
			t.Errorf("%s = %q, want empty", stream, got)
		}
		return
	}
	if !strings.Contains(got, want) {
		t.Errorf("%s = %q, want it to contain %q", stream, got, want)
	}
}
