//go:build windows

package main

import "syscall"

// IP_UNICAST_IF / IPV6_UNICAST_IF force the outgoing interface for a socket.
// Unlike binding the source address, this overrides the routing table's choice
// of interface, so an app really leaves on the link we picked even if another
// link has a lower-metric default route. No driver and no admin rights needed.
const (
	ipprotoIP     = 0
	ipUnicastIf   = 31
	ipprotoIPv6   = 41
	ipv6UnicastIf = 31
)

// For IPv4 the option value is the interface index in network byte order; for
// IPv6 it is the plain host-order index. (Windows is little-endian, so the v4
// value always has its bytes swapped.)
func htonl(v uint32) uint32 {
	return (v&0x000000ff)<<24 | (v&0x0000ff00)<<8 | (v&0x00ff0000)>>8 | (v&0xff000000)>>24
}

func setUnicastIf(c syscall.RawConn, network string, index int) error {
	if index <= 0 {
		return nil
	}
	var serr error
	cerr := c.Control(func(fd uintptr) {
		h := syscall.Handle(fd)
		switch network {
		case "tcp6", "udp6":
			serr = syscall.SetsockoptInt(h, ipprotoIPv6, ipv6UnicastIf, index)
		default: // tcp4, udp4 (and bare tcp/udp, which resolve to v4 here)
			serr = syscall.SetsockoptInt(h, ipprotoIP, ipUnicastIf, int(htonl(uint32(index))))
		}
	})
	if cerr != nil {
		return cerr
	}
	return serr
}
