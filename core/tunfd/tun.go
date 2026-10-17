// Package tunfd is the system-wide front-end on Android. VpnService owns the virtual adapter and
// its routes and hands over the adapter's file descriptor; every flow read from it is terminated
// in a userland TCP/IP stack and re-dialed through the Balancer.
package tunfd

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"syscall"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip/stack"

	"github.com/xjasonlyu/tun2socks/v2/core"
	"github.com/xjasonlyu/tun2socks/v2/core/device"
	"github.com/xjasonlyu/tun2socks/v2/core/device/fdbased"
	"github.com/xjasonlyu/tun2socks/v2/core/option"
	tlog "github.com/xjasonlyu/tun2socks/v2/log"
	M "github.com/xjasonlyu/tun2socks/v2/metadata"
	"github.com/xjasonlyu/tun2socks/v2/proxy/reject"
	"github.com/xjasonlyu/tun2socks/v2/tunnel"

	"github.com/dewil95/net-merge-android/core/engine"
)

// DefaultMTU is the TUN's MTU, set both on the VpnService adapter and in the userland stack. TUN
// traffic never leaves the phone (flows are re-dialed with normal sockets), so it isn't bound by
// the links' 1500, and bigger packets mean far fewer trips through the userland stack, which
// matters for CPU and battery. Not 65535: the TUN library allocates a full-MTU buffer for every
// inbound packet, even a 40-byte ACK.
const DefaultMTU = 9000

// udpSessionTimeout ends idle UDP sessions sooner than tun2socks' 60s. Torrent DHT opens a session
// per peer queried, each holding a socket, two goroutines and two 64 KB buffers. 30s stays above the
// ~25s keepalives common in VPN and NAT-traversal protocols.
const udpSessionTimeout = 30 * time.Second

// tunProxy plugs the Balancer into tun2socks: every TCP/UDP flow that enters the TUN is dialed
// out through a pinned network. There is no SOCKS hop in between.
type tunProxy struct{ b *engine.Balancer }

func (p tunProxy) DialContext(ctx context.Context, m *M.Metadata) (net.Conn, error) {
	return p.b.DialPinned(ctx, m.DstIP.Unmap().String(), int(m.DstPort))
}

func (p tunProxy) DialUDP(m *M.Metadata) (net.PacketConn, error) {
	f := engine.Fam4
	if d := m.DstIP.Unmap(); d.Is6() {
		f = engine.Fam6
	}
	return p.b.ListenUDP(context.Background(), f)
}

// Tun is the userland stack running on a VpnService adapter.
type Tun struct {
	dev device.Device
	stk *stack.Stack
}

// Start takes ownership of fd (from ParcelFileDescriptor.detachFd) and starts relaying every flow
// on it through b. The fd is closed by Close, or right away if Start fails.
func Start(fd, mtu int, b *engine.Balancer) (*Tun, error) {
	// "error", not "warn": tun2socks warns once per dropped UDP packet ("symmetric NAT ... drop
	// packet"), which a torrent client triggers continuously.
	if lvl, e := tlog.ParseLevel("error"); e == nil {
		tlog.SetLogger(tlog.Must(tlog.NewLeveled(lvl)))
	}
	tunnel.T().SetUDPTimeout(udpSessionTimeout)

	dev, err := fdbased.Open(strconv.Itoa(fd), uint32(mtu), 0)
	if err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("open TUN fd %d: %w", fd, err)
	}
	g := &gate{LinkEndpoint: dev}
	stk, err := core.CreateStack(&core.Config{
		LinkEndpoint:     g,
		TransportHandler: tunnel.T(),
		Options:          []option.Option{option.WithTCPModerateReceiveBuffer(true)},
	})
	if err != nil {
		dev.Close()
		return nil, err
	}
	installForwarders(stk, b.V6Allowed)
	tunnel.T().SetProxy(tunProxy{b})
	g.open()
	return &Tun{dev: dev, stk: stk}, nil
}

// gate holds back inbound packets until the stack is fully set up. tun2socks creates the NIC,
// which starts reading the fd, before installForwarders can replace its TCP/UDP handlers. On the
// desktop nothing arrives until the routes are added afterwards; here the VPN is already up and
// apps' packets may be waiting, so delivering them early would race with the handler swap.
type gate struct {
	stack.LinkEndpoint
	mu     sync.Mutex
	d      stack.NetworkDispatcher
	opened bool
}

func (g *gate) Attach(d stack.NetworkDispatcher) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.d = d
	if g.opened || d == nil {
		g.LinkEndpoint.Attach(d)
	}
}

func (g *gate) open() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.opened = true
	if g.d != nil {
		g.LinkEndpoint.Attach(g.d)
	}
}

// Close stops relaying and closes the fd, which also takes the VPN down on the Android side.
func (t *Tun) Close() {
	tunnel.T().SetProxy(&reject.Reject{})
	t.dev.Close()
	done := make(chan struct{})
	go func() { t.stk.Close(); t.stk.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}
