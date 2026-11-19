// Command chrome-on-wifi opens one Chrome that uses Wi-Fi. Everything else on
// the PC keeps using its normal connection (the cable, when a cable is the
// default). No config file: run it and a Wi-Fi Chrome opens.
//
// It starts a small local proxy pinned to the Wi-Fi interface and launches
// Chrome pointed at it, in its own profile folder so a fresh window opens and
// really uses Wi-Fi (and keeps its own logins between runs). Nothing else is
// touched, so the rest of the system is unaffected.
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

const port = 1080

func main() {
	wifi, ok := split.WifiInterface()
	if !ok {
		fmt.Println("No Wi-Fi connection found.")
		fmt.Println("Connect to a Wi-Fi network, then run this again.")
		pause()
		return
	}
	chrome := split.ChromePath()
	if chrome == "" {
		fmt.Println("Google Chrome was not found in the usual place.")
		fmt.Println("Install Chrome, then run this again.")
		pause()
		return
	}

	link := &split.Link{Name: "wifi", Interface: wifi.Name, IfaceIndex: wifi.Index, SourceIP: wifi.IP}
	proxy, err := split.Listen(fmt.Sprintf("127.0.0.1:%d", port), "wifi", link.Dial)
	if err != nil {
		fmt.Printf("Could not start (port %d busy?): %v\n", port, err)
		pause()
		return
	}
	defer proxy.Close()

	profile := filepath.Join(exeDir(), "chrome-wifi-profile")
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

	fmt.Printf("Chrome is now using Wi-Fi (%s on %s).\n", wifi.IP, wifi.Name)
	fmt.Println("Everything else on this PC keeps using your normal connection.")
	fmt.Println()
	fmt.Println("A separate Chrome window opened — that one is on Wi-Fi, with its own")
	fmt.Println("logins. Your usual Chrome is unchanged.")
	fmt.Println()
	fmt.Println("Keep this window open. Close this Wi-Fi Chrome (or this window) to stop.")

	// Stop when the user presses Ctrl+C / closes the window, or when the Wi-Fi
	// Chrome is closed. Chrome sometimes hands off to an already-running process
	// and returns at once; ignore such an immediate exit and wait for Ctrl+C.
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
		fmt.Println("Wi-Fi Chrome closed. Stopping.")
	}
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// pause keeps a double-clicked window open so an error can be read.
func pause() {
	fmt.Print("\nPress Enter to close.")
	fmt.Scanln()
}
