// Package nics describes the networks flows can leave through. On Android an app can't list
// adapters or bind sockets to them itself: the app learns the networks from ConnectivityManager
// and pushes them in, and a socket is pinned to one by handing its fd back to the app, which calls
// Network.bindSocket and VpnService.protect on it.
package nics

import (
	"fmt"
	"net"
	"sync/atomic"
	"syscall"
)

// Nic is one network (Wi-Fi, mobile data, Ethernet, ...) with the source addresses flows are
// pinned to: its IPv4 address and/or its global IPv6 address (nil = the network can't carry that
// family). An unspecified address (0.0.0.0 or ::) means the network carries the family but the
// source address is left to the OS. On Android the network binding picks it, which also covers
// 464XLAT on IPv6-only mobile networks (no IPv4 address at all) and IPv6 privacy addresses.
type Nic struct {
	Name   string
	Handle int64 // Network.getNetworkHandle(), used to pin sockets; 0 = don't pin
	IP     net.IP
	IP6    net.IP
	Weight float64
}

// Binder pins a socket to the network with the given handle. It must also keep the socket out of
// the VPN, or the flow would loop straight back into the TUN.
type Binder func(fd int, handle int64) error

var binder atomic.Pointer[Binder]

// SetBinder installs the app's binder; nil removes it.
func SetBinder(b Binder) {
	if b == nil {
		binder.Store(nil)
		return
	}
	binder.Store(&b)
}

// ForceEgress returns a dialer Control func that pins the socket to one network. A failed bind
// (the network just went away) is reported as ENETUNREACH, so the balancer sidelines that network
// and retries on another.
func ForceEgress(handle int64) func(network, address string, c syscall.RawConn) error {
	b := binder.Load()
	if handle == 0 || b == nil {
		return nil
	}
	return func(network, address string, c syscall.RawConn) error {
		var berr error
		if err := c.Control(func(fd uintptr) { berr = (*b)(int(fd), handle) }); err != nil {
			return err
		}
		if berr != nil {
			return fmt.Errorf("pin socket to network %d: %v: %w", handle, berr, syscall.ENETUNREACH)
		}
		return nil
	}
}

// Octets would report a network's total traffic (all apps) from the OS's counters. Android
// doesn't give apps per-interface counters, so only net-merge's own traffic is measured.
func Octets(handle int64) (in, out uint64, ok bool) { return 0, 0, false }
