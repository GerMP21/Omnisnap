package process

import (
	"os"
	"path/filepath"
	"testing"
)

// Task 65: a template copy drops Singleton locks (a copied lock makes the next
// launch think the profile is in use) and keeps everything else.
func TestCopyProfile(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copy")
	files := map[string]string{
		"Preferences":                          `{"x":1}`,
		"Default/Cookies":                      "cookie",
		"Default/Extensions/abc/manifest.json": `{}`,
		"SingletonLock":                        "lock",
		"SingletonSocket":                      "sock",
	}
	for name, body := range files {
		p := filepath.Join(src, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := CopyProfile(src, dst); err != nil {
		t.Fatal(err)
	}
	for name := range files {
		_, err := os.Stat(filepath.Join(dst, name))
		got := err == nil
		want := name != "SingletonLock" && name != "SingletonSocket"
		if got != want {
			t.Errorf("%s copied = %v; want %v", name, got, want)
		}
	}
	// Content and mode survive.
	b, err := os.ReadFile(filepath.Join(dst, "Preferences"))
	if err != nil || string(b) != files["Preferences"] {
		t.Errorf("Preferences = %q, %v", b, err)
	}
}
