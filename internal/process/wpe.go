//go:build linux

package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// WPEOptions configure the headless WPE lane: WPEWebDriver drives MiniBrowser
// built against libWPEWebKit. It needs no compositor and no window manager, and
// MiniBrowser's --size fixes the viewport exactly, which the GTK port only
// manages inside a private Weston display.
//
// Capability shape: {"browserName": "MiniBrowser", "wpe:browserOptions": {
// "binary", "args"}}; "setWindowRect" is false, so the viewport comes from
// --size at launch rather than from /window/rect.
type WPEOptions struct {
	Driver  string   // WPEWebDriver
	Browser string   // MiniBrowser from the wpewebkit package
	Width   int      // viewport width at launch (0: 1280)
	Height  int      // viewport height at launch (0: 800)
	Args    []string // extra MiniBrowser args after --automation --headless
	Root    string   // "" = OS temp dir
	// NoFontParity leaves the browser's own fonts alone (WEBKIT_FONT_PARITY=0
	// semantics); by default MiniBrowser gets a [websettings] config with the
	// same four families the GTK port passes on the command line.
	NoFontParity bool
}

// StartWPE launches WPEWebDriver and waits until it answers GET /status.
func StartWPE(ctx context.Context, o WPEOptions) (*WebKit, error) {
	w, h := o.Width, o.Height
	if w <= 0 || h <= 0 {
		w, h = 1280, 800
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	hnd, err := spawnEnv(ctx, o.Root, o.Driver, []string{fmt.Sprintf("--port=%d", port)}, func(tempDir) ([]string, []string) {
		// Headless WPE has no GPU either; the same switch the GTK lane uses.
		env := []string{"WEBKIT_DISABLE_DMABUF_RENDERER=1"}
		if sandboxDisabled() {
			env = append(env, "WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1")
		}
		return nil, env
	})
	if err != nil {
		return nil, err
	}
	args := []string{"--automation", "--headless", fmt.Sprintf("--size=%dx%d", w, h)}
	if !o.NoFontParity {
		// WPE MiniBrowser has no --*-font-family flags; --config-file applies
		// the same settings under [websettings]. Without it the generic
		// families fall back to the system defaults and every text width
		// differs from Blink's.
		conf := filepath.Join(hnd.tmp.dir, "wpe-settings.conf")
		body := "[websettings]\n" +
			"default-font-family=Liberation Serif\n" +
			"serif-font-family=Liberation Serif\n" +
			"sans-serif-font-family=Liberation Sans\n" +
			"monospace-font-family=Noto Sans Mono\n"
		if err := os.WriteFile(conf, []byte(body), 0o600); err != nil {
			_ = hnd.Close(context.WithoutCancel(ctx))
			return nil, fmt.Errorf("process: wpe font config: %w", err)
		}
		args = append(args, "--config-file="+conf)
	}
	args = append(args, o.Args...)
	wk := &WebKit{
		Handle: hnd,
		URL:    fmt.Sprintf("http://127.0.0.1:%d", port),
		Capabilities: map[string]any{
			"alwaysMatch": map[string]any{
				"browserName": "MiniBrowser",
				"wpe:browserOptions": map[string]any{
					"binary": o.Browser,
					"args":   args,
				},
			},
		},
		// No private display: WPE's headless platform is in-process, so
		// nothing appears on the user's desktop.
		visible: false,
	}
	if err := waitDriver(ctx, wk.URL); err != nil {
		_ = wk.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	return wk, nil
}

// FindWPE returns the installed WPE driver and browser, honouring the same
// environment overrides as the other launchers.
func FindWPE() (driver, browser string) {
	driver = lookupEnv("OMNISNAP_WPE_DRIVER", "WPEWebDriver")
	browser = lookupEnv("OMNISNAP_WPE_BROWSER", "")
	if browser == "" {
		for _, p := range []string{
			"/usr/lib/wpe-webkit-2.0/MiniBrowser",
			"/usr/lib64/wpe-webkit-2.0/MiniBrowser",
			"/usr/lib/x86_64-linux-gnu/wpe-webkit-2.0/MiniBrowser",
		} {
			if fileExists(p) {
				browser = p
				break
			}
		}
	}
	return driver, browser
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func lookupEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if fallback == "" {
		return ""
	}
	if p, err := exec.LookPath(fallback); err == nil {
		return p
	}
	return ""
}
