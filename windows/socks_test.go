package main

import (
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

// echoServer accepts one-shot connections and echoes everything back.
func echoServer(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { io.Copy(c, c); c.Close() }(c)
		}
	}()
	return ln
}

// socksConnect does the SOCKS5 no-auth handshake and a CONNECT to host:port,
// returning the open connection ready to carry data.
func socksConnect(t *testing.T, proxyAddr, host string, port int) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	c.SetDeadline(time.Now().Add(5 * time.Second))

	// Greeting: one method, no-auth.
	if _, err := c.Write([]byte{socksVersion, 1, 0x00}); err != nil {
		t.Fatal(err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(c, resp); err != nil {
		t.Fatal(err)
	}
	if resp[0] != socksVersion || resp[1] != 0x00 {
		t.Fatalf("bad method reply %v", resp)
	}

	// CONNECT request with a domain-style address so the proxy's resolver runs.
	req := []byte{socksVersion, cmdConnect, 0x00, atypDomain, byte(len(host))}
	req = append(req, host...)
	req = append(req, 0, 0)
	binary.BigEndian.PutUint16(req[len(req)-2:], uint16(port))
	if _, err := c.Write(req); err != nil {
		t.Fatal(err)
	}

	reply := make([]byte, 10) // v4 bound address is 10 bytes total
	if _, err := io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	if reply[0] != socksVersion || reply[1] != repSuccess {
		t.Fatalf("CONNECT failed, reply %v", reply)
	}
	c.SetDeadline(time.Time{})
	return c
}

func TestProxyConnectAndRelay(t *testing.T) {
	target := echoServer(t)
	defer target.Close()
	_, portStr, _ := net.SplitHostPort(target.Addr().String())
	port, _ := strconv.Atoi(portStr)

	// A link with index 0: on Linux setUnicastIf is a no-op, so this dials
	// normally and exercises the full SOCKS path including the resolver.
	l := &Link{Name: "test", IfaceIndex: 0}
	p, err := listenProxy("127.0.0.1:0", "test", l.dial)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()

	conn := socksConnect(t, p.addr(), "localhost", port)
	defer conn.Close()

	msg := []byte("hello over socks")
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(msg))
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(msg) {
		t.Fatalf("echo mismatch: got %q want %q", got, msg)
	}
}

func TestProxyRefusesWhenTargetDown(t *testing.T) {
	// Grab a port, then close it so nothing is listening there.
	tmp, _ := net.Listen("tcp", "127.0.0.1:0")
	_, portStr, _ := net.SplitHostPort(tmp.Addr().String())
	port, _ := strconv.Atoi(portStr)
	tmp.Close()

	l := &Link{Name: "test", IfaceIndex: 0}
	p, err := listenProxy("127.0.0.1:0", "test", l.dial)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()

	c, err := net.Dial("tcp", p.addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	c.Write([]byte{socksVersion, 1, 0x00})
	io.ReadFull(c, make([]byte, 2))
	req := []byte{socksVersion, cmdConnect, 0x00, atypIPv4, 127, 0, 0, 1, 0, 0}
	binary.BigEndian.PutUint16(req[len(req)-2:], uint16(port))
	c.Write(req)
	reply := make([]byte, 10)
	if _, err := io.ReadFull(c, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != repRefused {
		t.Fatalf("want refused reply, got status %d", reply[1])
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/c.json"
	in := &Config{
		Links: []LinkConfig{{Name: "cable", Interface: "Ethernet", IP: "192.168.1.50", Port: 1080}},
		Apps:  []AppConfig{{Name: "Chrome", Path: `C:\chrome.exe`, Link: "cable", Chromium: true}},
	}
	if err := saveConfig(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Links) != 1 || out.Links[0].Name != "cable" || out.Links[0].Port != 1080 {
		t.Fatalf("links round-trip wrong: %+v", out.Links)
	}
	if out.linkConfig("cable") == nil || out.linkConfig("nope") != nil {
		t.Fatal("linkConfig lookup wrong")
	}
}

func TestResolveLinkMatchesByIP(t *testing.T) {
	ifs := []iface{
		{name: "Ethernet", index: 5, ip: net.IPv4(192, 168, 1, 50)},
		{name: "Wi-Fi", index: 9, ip: net.IPv4(192, 168, 1, 77), wifi: true},
	}
	// No interface name given, so it must match on IP.
	l, err := resolveLink(LinkConfig{Name: "wifi", IP: "192.168.1.77"}, ifs)
	if err != nil {
		t.Fatal(err)
	}
	if l.IfaceIndex != 9 || l.Interface != "Wi-Fi" {
		t.Fatalf("matched wrong adapter: %+v", l)
	}
	if _, err := resolveLink(LinkConfig{Name: "x", IP: "10.0.0.1"}, ifs); err == nil {
		t.Fatal("expected no-match error")
	}
}
