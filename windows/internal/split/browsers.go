package split

import "os"

// Browser is a Chromium browser we can auto-detect and launch with a per-link
// proxy. Chrome, Brave and Edge all accept --proxy-server and --user-data-dir,
// which is what we need; Firefox does not take a proxy on the command line.
type Browser struct {
	Name     string
	Paths    []string
	Chromium bool
}

var KnownBrowsers = []Browser{
	{"Chrome", []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	}, true},
	{"Brave", []string{
		`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
		`C:\Program Files (x86)\BraveSoftware\Brave-Browser\Application\brave.exe`,
	}, true},
	{"Edge", []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	}, true},
}

// FindBrowser returns the first path that exists, or "" if none.
func FindBrowser(b Browser) string {
	for _, p := range b.Paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ChromePath finds Google Chrome, or "" if it isn't installed.
func ChromePath() string {
	for _, b := range KnownBrowsers {
		if b.Name == "Chrome" {
			return FindBrowser(b)
		}
	}
	return ""
}
