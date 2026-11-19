// Command chrome-merge opens one Chrome that uses all your networks at once.
//
// It starts a local proxy that spreads Chrome's connections across every network
// that has internet (cable, Wi-Fi, a tethered phone), sending each new connection
// to the least-busy one. A web page with many resources, or a download manager
// with several parts, then gets the networks' speed combined. A single connection
// (one plain download stream, one video) still uses one network.
//
// No config file: run it and a merged Chrome opens. Everything else on the PC is
// untouched. This is the PC cousin of the Android net-merge app.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/dewil95/net-merge-android/windows/internal/split"
)

const port = 1081 // chrome-on-wifi uses 1080; a different port lets both run

func main() {
	chrome := split.ChromePath()
	if chrome == "" {
		fmt.Println("Google Chrome was not found in the usual place.")
		fmt.Println("Install Chrome, then run this again.")
		pause()
		return
	}

	// Keep only interfaces that really reach the internet, so dead or virtual
	// adapters (VPNs, VM host-only, etc.) don't swallow half the connections.
	fmt.Println("Checking which networks have internet...")
	var links []*split.Link
	for _, in := range split.UsableInterfaces() {
		l := &split.Link{Name: in.Name, Interface: in.Name, IfaceIndex: in.Index, SourceIP: in.IP}
		if l.Reachable(3 * time.Second) {
			links = append(links, l)
			fmt.Printf("  using %s (%s)\n", in.Name, in.IP)
		} else {
			fmt.Printf("  skipping %s (%s): no internet\n", in.Name, in.IP)
		}
	}
	if len(links) < 2 {
		fmt.Println("\nNeed at least two working networks to merge.")
		fmt.Println("Connect both (e.g. the cable and Wi-Fi), then run again.")
		pause()
		return
	}

	bal := split.NewBalancer(links)
	proxy, err := split.Listen(fmt.Sprintf("127.0.0.1:%d", port), "merge", bal.Dial)
	if err != nil {
		fmt.Printf("Could not start (port %d busy?): %v\n", port, err)
		pause()
		return
	}
	defer proxy.Close()

	profile := filepath.Join(exeDir(), "chrome-merge-profile")
	cmd := exec.Command(chrome,
		fmt.Sprintf("--proxy-server=socks5://127.0.0.1:%d", port),
		"--user-data-dir="+profile,
		"--no-first-run",
		"--no-default-browser-check",
	)
	if err := cmd.Start(); err != nil {
		fmt.Printf("Could not open Chrome: %v\n", err)
		pause()
		return
	}

	fmt.Printf("\nChrome is now using all %d networks together.\n", len(links))
	fmt.Println("A separate Chrome window opened — that one is merged, with its own logins.")
	fmt.Println("Pages with many parts, and multi-part downloads, use both networks at once.")
	fmt.Println("(One single download stream still uses one network — use a download manager")
	fmt.Println("with several parts for big files.)")
	fmt.Println()
	fmt.Println("Keep this window open. Close this Chrome (or this window) to stop.")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	chromeClosed := make(chan struct{})
	start := time.Now()
	go func() {
		cmd.Wait()
		if time.Since(start) > 5*time.Second {
			close(chromeClosed)
		}
	}()

	select {
	case <-sig:
	case <-chromeClosed:
		fmt.Println("Merged Chrome closed. Stopping.")
	}
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func pause() {
	fmt.Print("\nPress Enter to close.")
	fmt.Scanln()
}
