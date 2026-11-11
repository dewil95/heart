//go:build !windows

package main

import "syscall"

// Pinning a socket to an interface is implemented for Windows (IP_UNICAST_IF).
// On other systems this is a no-op, so the proxy, the config logic and the
// tests still build and run — handy for developing and testing on Linux or Mac.
func setUnicastIf(_ syscall.RawConn, _ string, _ int) error {
	return nil
}
