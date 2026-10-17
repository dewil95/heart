//go:build linux

package tunfd

import (
	"bufio"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/fdbased"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"

	"github.com/dewil95/net-merge-android/core/engine"
	"github.com/dewil95/net-merge-android/core/nics"
)

// hostIPv4 returns a non-loopback address of this machine: the "phone" stack can't send to
// 127/8 over a non-loopback link, but the balancer's real sockets can reach this one locally.
func hostIPv4(t *testing.T) net.IP {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skip(err)
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil {
			return ipn.IP.To4()
		}
	}
	t.Skip("no non-loopback IPv4 address")
	return nil
}

// TestFlowThroughTUN sends a TCP connection into the TUN fd, the way an app's connection arrives
// from VpnService, and checks that it comes out of the balancer on a real socket and back.
func TestFlowThroughTUN(t *testing.T) {
	ip := hostIPv4(t)
	ln, err := net.Listen("tcp4", net.JoinHostPort(ip.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				c.Write([]byte("hello\n"))
				io.Copy(c, c)
			}()
		}
	}()

	// A packet socketpair stands in for the TUN: one end for net-merge, one for the "phone".
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		t.Fatal(err)
	}
	sched := engine.NewScheduler([]nics.Nic{{Name: "test0", IP: net.IPv4zero, Weight: 1}})
	tun, err := Start(fds[0], 1500, engine.NewBalancer(sched))
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()

	ep, err := fdbased.New(&fdbased.Options{FDs: []int{fds[1]}, MTU: 1500})
	if err != nil {
		t.Fatal(err)
	}
	phone := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
	})
	defer phone.Close()
	if err := phone.CreateNIC(1, ep); err != nil {
		t.Fatal(err)
	}
	local := tcpip.AddressWithPrefix{Address: tcpip.AddrFromSlice(net.IPv4(198, 18, 0, 2).To4()), PrefixLen: 24}
	if err := phone.AddProtocolAddress(1, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber, AddressWithPrefix: local}, stack.AddressProperties{}); err != nil {
		t.Fatal(err)
	}
	phone.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: 1}})

	port := ln.Addr().(*net.TCPAddr).Port
	c, err := gonet.DialTCP(phone, tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFromSlice(ip), Port: uint16(port)}, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReader(c)
	if line, err := r.ReadString('\n'); err != nil || line != "hello\n" {
		t.Fatalf("greeting = %q, %v", line, err)
	}
	c.Write([]byte("ping\n"))
	if line, err := r.ReadString('\n'); err != nil || line != "ping\n" {
		t.Fatalf("echo = %q, %v", line, err)
	}
	if st := sched.Snapshot(); len(st) != 1 || st[0].Total != 1 {
		t.Fatalf("the flow should have gone through the balancer: %+v", st)
	}
}
