// Command fetch downloads one URL several times in parallel and reports each download. The
// emulator test runs it on the device (static linux binary, adb push) to put parallel traffic
// through the VPN, since the emulator image has no curl.
//
//	SSL_CERT_DIR=/system/etc/security/cacerts fetch -n 8 -d 15s https://example.com/file
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

func main() {
	n := flag.Int("n", 8, "parallel downloads")
	timeout := flag.Duration("t", 25*time.Second, "time limit per download")
	keep := flag.Duration("d", 0, "keep each worker downloading again until this much time has passed")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: fetch [-n 8] [-t 25s] [-d 15s] URL")
		os.Exit(2)
	}
	until := time.Now().Add(*keep)
	// Android has no /etc/resolv.conf, so ask a public resolver (through the VPN, like any app).
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, "1.1.1.1:53")
	}}
	dialer := &net.Dialer{Resolver: resolver, Timeout: 10 * time.Second}
	client := &http.Client{
		Timeout: *timeout,
		// One connection per download, so the downloads can land on different networks.
		Transport: &http.Transport{DialContext: dialer.DialContext, DisableKeepAlives: true},
	}
	var wg sync.WaitGroup
	for i := 1; i <= *n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				start := time.Now()
				resp, err := client.Get(flag.Arg(0))
				if err != nil {
					fmt.Printf("download %d: %v\n", i, err)
					return
				}
				b, err := io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				secs := time.Since(start).Seconds()
				fmt.Printf("download %d: HTTP %d, %d bytes in %.1fs (%.1f Mbit/s)%s\n",
					i, resp.StatusCode, b, secs, float64(b)*8/secs/1e6, errSuffix(err))
				if err != nil || time.Now().After(until) {
					return
				}
			}
		}()
	}
	wg.Wait()
}

func errSuffix(err error) string {
	if err == nil {
		return ""
	}
	return ", stopped: " + err.Error()
}
