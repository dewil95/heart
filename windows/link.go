package main

import (
	"context"
	"net"
	"syscall"
	"time"
)

// A Link is one network path (a cable, a Wi-Fi, a phone tethered over USB). It
// knows the interface to send through and the DNS servers to use. Every socket
// it opens is pinned to that interface with IP_UNICAST_IF (see bind_windows.go),
// so the traffic leaves on that link even when another link has a "better"
// (lower-metric) default route.
type Link struct {
	Name       string
	Interface  string
	IfaceIndex int
	SourceIP   net.IP
	DNS        []string // "host:port"; defaults to 1.1.1.1 and 8.8.8.8
}

func (l *Link) control(network, address string, c syscall.RawConn) error {
	return setUnicastIf(c, network, l.IfaceIndex)
}

// resolver looks names up over this link, so DNS answers reflect the route the
// traffic will actually take and the lookups don't leak onto another link.
func (l *Link) resolver() *net.Resolver {
	servers := l.DNS
	if len(servers) == 0 {
		servers = []string{"1.1.1.1:53", "8.8.8.8:53"}
	}
	d := &net.Dialer{Timeout: 5 * time.Second, Control: l.control}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var last error
			for _, s := range servers {
				// DNS is small; UDP is enough and matches what the OS does.
				if c, err := d.DialContext(ctx, "udp", s); err == nil {
					return c, nil
				} else {
					last = err
				}
			}
			return nil, last
		},
	}
}

func (l *Link) dialer() *net.Dialer {
	return &net.Dialer{
		Timeout:  20 * time.Second,
		Control:  l.control,
		Resolver: l.resolver(),
	}
}

// dial is what the proxy hands each browser connection to.
func (l *Link) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	return l.dialer().DialContext(ctx, network, addr)
}
