package main

import "os"

// knownBrowsers are the Chromium browsers we can auto-detect and launch with a
// per-link proxy. Chrome, Brave and Edge all accept --proxy-server and
// --user-data-dir, which is what we need; Firefox does not take a proxy on the
// command line, so it is set up by hand (see the README).
type browser struct {
	name     string
	paths    []string
	chromium bool
}

var knownBrowsers = []browser{
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

func findBrowser(b browser) string {
	for _, p := range b.paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
