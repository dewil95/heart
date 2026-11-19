// Command netmerge-split sends each chosen app out a chosen network on Windows,
// driven by a netmerge-split.json config next to the program. For example,
// Chrome over the cable and Brave over Wi-Fi, each with its own IP address.
//
// This separates apps onto different networks. It does not merge one app's
// traffic across networks (that is what the Android net-merge app does). For
// the common "just put Chrome on Wi-Fi" case, use the chrome-on-wifi program.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	"github.com/dewil95/net-merge-android/windows/internal/split"
)

func main() {
	dir := exeDir()
	cfgPath := filepath.Join(dir, "netmerge-split.json")

	cfg, err := split.LoadConfig(cfgPath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Println(err)
			fmt.Println("Fix or delete this file and run again.")
			pause()
			return
		}
		fmt.Println("First run: looking at your networks and browsers...")
		cfg = split.GenerateConfig()
		if err := split.SaveConfig(cfgPath, cfg); err != nil {
			fmt.Println("could not write config:", err)
		} else {
			fmt.Printf("Wrote %s\nEdit it to choose which app uses which link, then run again.\n\n", cfgPath)
		}
	}

	if len(cfg.Links) == 0 {
		fmt.Println("No usable networks found. Connect your cable and Wi-Fi, then run again.")
		pause()
		return
	}

	ifs := split.UsableInterfaces()
	live := map[string]bool{}
	var proxies []*split.Proxy
	for _, lc := range cfg.Links {
		l, err := split.ResolveLink(lc, ifs)
		if err != nil {
			fmt.Printf("  link %-8q skipped: %v\n", lc.Name, err)
			continue
		}
		p, err := split.Listen(fmt.Sprintf("127.0.0.1:%d", lc.Port), lc.Name, l.Dial)
		if err != nil {
			fmt.Printf("  link %-8q skipped: cannot use port %d: %v\n", lc.Name, lc.Port, err)
			continue
		}
		proxies = append(proxies, p)
		live[lc.Name] = true
		fmt.Printf("  link %-8q -> %s (%s) on 127.0.0.1:%d\n", lc.Name, l.Interface, l.SourceIP, lc.Port)
	}
	if len(proxies) == 0 {
		fmt.Println("None of the links could start. Check the names/IPs in your config.")
		pause()
		return
	}

	fmt.Println()
	for _, a := range cfg.Apps {
		lc := cfg.LinkConfig(a.Link)
		if lc == nil || !live[a.Link] {
			fmt.Printf("  app  %-8q skipped: link %q is not available\n", a.Name, a.Link)
			continue
		}
		launchApp(dir, a, lc.Port)
	}

	fmt.Println("\nRunning. Keep this window open. Close it (or press Ctrl+C) to stop.")
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
	for _, p := range proxies {
		p.Close()
	}
}

// launchApp starts one app pointed at its link's proxy. Chromium browsers need
// a separate --user-data-dir so a fresh instance opens (and honours the proxy)
// even if that browser is already running; it also gives each one its own
// logins, which is the point of putting them on different IPs.
func launchApp(dir string, a split.AppConfig, port int) {
	if _, err := os.Stat(a.Path); err != nil {
		fmt.Printf("  app  %-8q skipped: not found at %s\n", a.Name, a.Path)
		return
	}
	var args []string
	if a.Chromium {
		profile := filepath.Join(dir, "profiles", a.Name)
		args = []string{
			fmt.Sprintf("--proxy-server=socks5://127.0.0.1:%d", port),
			"--user-data-dir=" + profile,
			"--no-first-run",
			"--no-default-browser-check",
		}
	}
	args = append(args, a.Args...)

	cmd := exec.Command(a.Path, args...)
	if err := cmd.Start(); err != nil {
		fmt.Printf("  app  %-8q could not start: %v\n", a.Name, err)
		return
	}
	if a.Chromium {
		fmt.Printf("  app  %-8q started on link %q (127.0.0.1:%d)\n", a.Name, a.Link, port)
	} else {
		fmt.Printf("  app  %-8q started on link %q: set its proxy to SOCKS5 127.0.0.1:%d by hand\n", a.Name, a.Link, port)
	}
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// pause keeps the console window open when double-clicked and about to exit on
// an error, so the message can be read.
func pause() {
	fmt.Print("\nPress Enter to close.")
	fmt.Scanln()
}
