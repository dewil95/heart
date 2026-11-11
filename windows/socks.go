package main

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"time"
)

// A tiny SOCKS5 proxy (CONNECT only, no authentication). Each proxy is tied to
// one link: every connection it opens for a browser goes out through that
// link's network interface, and so does the DNS lookup. A browser is pointed at
// it with --proxy-server=socks5://127.0.0.1:<port>, so all of that browser's
// traffic leaves on the chosen interface.
//
// SOCKS5 is RFC 1928. We only need the no-auth handshake and the CONNECT
// command; browsers never ask for BIND or UDP ASSOCIATE over a SOCKS proxy.

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

type proxy struct {
	name string
	ln   net.Listener
	dial dialFunc
}

// listenProxy starts a proxy on addr (e.g. "127.0.0.1:1080") that opens its
// connections with dial. It returns once it is listening; it serves in the
// background until close is called.
func listenProxy(addr, name string, dial dialFunc) (*proxy, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	p := &proxy{name: name, ln: ln, dial: dial}
	go p.serve()
	return p, nil
}

func (p *proxy) addr() string { return p.ln.Addr().String() }

func (p *proxy) close() error { return p.ln.Close() }

func (p *proxy) serve() {
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return // listener closed
		}
		go p.handle(c)
	}
}

const (
	socksVersion = 0x05
	cmdConnect   = 0x01
	atypIPv4     = 0x01
	atypDomain   = 0x03
	atypIPv6     = 0x04
	repSuccess   = 0x00
	repRefused   = 0x05
	repCmdNotSup = 0x07
)

func (p *proxy) handle(client net.Conn) {
	defer client.Close()
	// The browser should be quick to speak; don't let a stuck handshake sit.
	client.SetDeadline(time.Now().Add(30 * time.Second))

	// Greeting: version, number of methods, then that many method bytes.
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(client, hdr); err != nil {
		return
	}
	if hdr[0] != socksVersion {
		return
	}
	methods := make([]byte, int(hdr[1]))
	if _, err := io.ReadFull(client, methods); err != nil {
		return
	}
	// Answer: version, "no authentication required".
	if _, err := client.Write([]byte{socksVersion, 0x00}); err != nil {
		return
	}

	// Request: version, command, reserved, address type.
	req := make([]byte, 4)
	if _, err := io.ReadFull(client, req); err != nil {
		return
	}
	if req[0] != socksVersion {
		return
	}
	if req[1] != cmdConnect {
		reply(client, repCmdNotSup)
		return
	}

	var host string
	switch req[3] {
	case atypIPv4:
		b := make([]byte, 4)
		if _, err := io.ReadFull(client, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case atypIPv6:
		b := make([]byte, 16)
		if _, err := io.ReadFull(client, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case atypDomain:
		l := make([]byte, 1)
		if _, err := io.ReadFull(client, l); err != nil {
			return
		}
		b := make([]byte, int(l[0]))
		if _, err := io.ReadFull(client, b); err != nil {
			return
		}
		host = string(b) // a name: the link's own resolver looks it up
	default:
		reply(client, repCmdNotSup)
		return
	}

	pb := make([]byte, 2)
	if _, err := io.ReadFull(client, pb); err != nil {
		return
	}
	target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb))))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	remote, err := p.dial(ctx, "tcp", target)
	if err != nil {
		reply(client, repRefused)
		return
	}
	defer remote.Close()

	if err := reply(client, repSuccess); err != nil {
		return
	}
	client.SetDeadline(time.Time{}) // the transfer itself has no deadline
	relay(client, remote)
}

// reply sends a SOCKS5 reply with the given status and a zero bound address,
// which is all a CONNECT client needs.
func reply(client net.Conn, status byte) error {
	_, err := client.Write([]byte{socksVersion, status, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
	return err
}

// relay copies in both directions until either side closes, then lets both go.
func relay(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite() // tell the other end we're done sending
		}
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	<-done
}
