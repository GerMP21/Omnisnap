package discovery

import "github.com/GerMP21/Omnisnap/internal/schema"

// known describes a browser discovery recognizes, and every name it goes by.
type known struct {
	id       string
	name     string
	family   schema.Family
	exes     []string // executable names on PATH (Linux)
	desktop  []string // .desktop file ids (Linux)
	flatpak  string   // Flatpak app id
	snap     string   // snap name
	packages []string // dpkg / pacman package names
}

func (k known) browser(exe string, src Source) Browser {
	return Browser{ID: k.id, Name: k.name, Family: k.family, Protocol: protocolOf(k.family), Executable: exe, Source: src}
}

func protocolOf(f schema.Family) Protocol {
	switch f {
	case schema.FamilyGecko:
		return ProtocolBiDi
	case schema.FamilyWebKit:
		return ProtocolClassic
	default:
		return ProtocolCDP
	}
}

// catalog lists the browsers Omnisnap drives (out of scope: Arc, Tor Browser,
// mobile browsers). Opera and Vivaldi are driven as Blink over CDP.
var catalog = []known{
	{id: "chrome", name: "Google Chrome", family: schema.FamilyBlink,
		exes: []string{"google-chrome-stable", "google-chrome"}, desktop: []string{"google-chrome", "com.google.Chrome"},
		flatpak: "com.google.Chrome", packages: []string{"google-chrome-stable", "google-chrome"}},
	{id: "chrome-beta", name: "Google Chrome Beta", family: schema.FamilyBlink,
		exes: []string{"google-chrome-beta"}, desktop: []string{"google-chrome-beta"}, packages: []string{"google-chrome-beta"}},
	{id: "chromium", name: "Chromium", family: schema.FamilyBlink,
		exes: []string{"chromium", "chromium-browser"}, desktop: []string{"chromium", "chromium-browser", "chromium_chromium"},
		flatpak: "org.chromium.Chromium", snap: "chromium", packages: []string{"chromium", "chromium-browser"}},
	{id: "brave", name: "Brave", family: schema.FamilyBlink,
		exes: []string{"brave-browser", "brave-browser-stable", "brave"}, desktop: []string{"brave-browser", "brave_brave"},
		flatpak: "com.brave.Browser", snap: "brave", packages: []string{"brave-browser", "brave-bin"}},
	{id: "edge", name: "Microsoft Edge", family: schema.FamilyBlink,
		exes: []string{"microsoft-edge-stable", "microsoft-edge"}, desktop: []string{"microsoft-edge"},
		flatpak: "com.microsoft.Edge", packages: []string{"microsoft-edge-stable", "microsoft-edge-stable-bin"}},
	{id: "opera", name: "Opera", family: schema.FamilyBlink,
		exes: []string{"opera"}, desktop: []string{"opera", "opera_opera"},
		flatpak: "com.opera.Opera", snap: "opera", packages: []string{"opera-stable", "opera"}},
	{id: "vivaldi", name: "Vivaldi", family: schema.FamilyBlink,
		exes: []string{"vivaldi-stable", "vivaldi"}, desktop: []string{"vivaldi-stable"},
		flatpak: "com.vivaldi.Vivaldi", snap: "vivaldi", packages: []string{"vivaldi-stable", "vivaldi"}},
	{id: "firefox", name: "Firefox", family: schema.FamilyGecko,
		exes: []string{"firefox"}, desktop: []string{"firefox", "firefox_firefox"},
		flatpak: "org.mozilla.firefox", snap: "firefox", packages: []string{"firefox"}},
	{id: "firefox-esr", name: "Firefox ESR", family: schema.FamilyGecko,
		exes: []string{"firefox-esr"}, desktop: []string{"firefox-esr"}, packages: []string{"firefox-esr"}},
}

// webkitgtk is found at its well-known install paths, not by name.
var webkitgtk = known{id: "webkitgtk", name: "WebKitGTK MiniBrowser", family: schema.FamilyWebKit,
	packages: []string{"libwebkitgtk-6.0-4", "webkitgtk-6.0", "libwebkit2gtk-4.1-0", "webkit2gtk-4.1"}}

func byID(id string) (known, bool) {
	if id == webkitgtk.id {
		return webkitgtk, true
	}
	for _, k := range catalog {
		if k.id == id {
			return k, true
		}
	}
	return known{}, false
}
