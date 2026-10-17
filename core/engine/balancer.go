package engine

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/dewil95/net-merge-android/core/nics"
)

// Balancer dials outbound flows through the Scheduler. Front-ends (SOCKS5 now, TUN later)
// only call DialTCP and Relay, so the balancing logic lives in one place.
type Balancer struct {
	S              *Scheduler
	ConnectTimeout time.Duration
	FailCooldown   time.Duration
	// RaceAfter starts a second attempt on another NIC when the first hasn't connected by then;
	// the first to connect wins. 0 = no racing.
	RaceAfter time.Duration
	v6mode    atomic.Int32
}

// V6Mode decides what happens to IPv6 flows entering the TUN.
type V6Mode int32

const (
	// V6Auto balances IPv6 only when every adapter has it. When adapters differ, IPv6 flows are
	// refused so apps fall back to IPv4, which spreads over all adapters. (Apps that prefer IPv6
	// would otherwise funnel onto the few adapters that have it.)
	V6Auto V6Mode = iota
	// V6On always allows IPv6, balanced across the adapters that have it.
	V6On
	// V6Off refuses IPv6 entirely.
	V6Off
)

func ParseV6Mode(s string) (V6Mode, bool) {
	switch s {
	case "auto":
		return V6Auto, true
	case "on":
		return V6On, true
	case "off":
		return V6Off, true
	}
	return V6Auto, false
}

func (b *Balancer) SetV6Mode(m V6Mode) { b.v6mode.Store(int32(m)) }
func (b *Balancer) V6Mode() V6Mode     { return V6Mode(b.v6mode.Load()) }

// V6Allowed is consulted for every new IPv6 flow.
func (b *Balancer) V6Allowed() bool {
	switch b.V6Mode() {
	case V6Off:
		return false
	case V6On:
		return b.S.V6Usable()
	}
	return b.S.V6CoversAll()
}

// V6State describes the current IPv6 behaviour for `status` and the tray.
func (b *Balancer) V6State() string {
	switch b.V6Mode() {
	case V6Off:
		return "off (refused, apps use IPv4)"
	case V6On:
		return "on (balanced over the adapters that have IPv6)"
	}
	if b.S.V6CoversAll() {
		return "auto: balanced (every adapter has IPv6)"
	}
	return "auto: refused, apps fall back to IPv4 so every adapter is used (not every adapter has IPv6)"
}

func NewBalancer(s *Scheduler) *Balancer {
	return &Balancer{S: s, ConnectTimeout: 8 * time.Second, FailCooldown: 15 * time.Second, RaceAfter: time.Second}
}

// Windows socket error codes; the portable syscall.E* constants don't match them on Windows.
const (
	wsaeAddrNotAvail syscall.Errno = 10049
	wsaeNetDown      syscall.Errno = 10050
	wsaeNetUnreach   syscall.Errno = 10051
	wsaeTimedOut     syscall.Errno = 10060
	wsaeConnRefused  syscall.Errno = 10061
	wsaeHostUnreach  syscall.Errno = 10065
)

// SOCKS5 reply codes (also used as the generic failure reason).
const (
	repOK              = 0x00
	repFailure         = 0x01
	repNetUnreachable  = 0x03
	repHostUnreachable = 0x04
	repRefused         = 0x05
	repCmdUnsupported  = 0x07
	repAtypUnsupported = 0x08
)

type dialClass struct {
	rep      byte
	retry    bool // worth trying another NIC
	linkDown bool // this NIC itself is broken: sideline it
}

func classify(err error) dialClass {
	is := func(wsa, std syscall.Errno) bool { return errors.Is(err, wsa) || errors.Is(err, std) }
	var dns *net.DNSError
	var ne net.Error
	switch {
	case errors.As(err, &dns):
		return dialClass{rep: repHostUnreachable}
	case is(wsaeAddrNotAvail, syscall.EADDRNOTAVAIL), is(wsaeNetDown, syscall.ENETDOWN):
		return dialClass{rep: repNetUnreachable, retry: true, linkDown: true}
	case is(wsaeNetUnreach, syscall.ENETUNREACH):
		return dialClass{rep: repNetUnreachable, retry: true, linkDown: true}
	case is(wsaeHostUnreach, syscall.EHOSTUNREACH), is(wsaeTimedOut, syscall.ETIMEDOUT):
		return dialClass{rep: repHostUnreachable, retry: true}
	case is(wsaeConnRefused, syscall.ECONNREFUSED):
		return dialClass{rep: repRefused}
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return dialClass{rep: repHostUnreachable, retry: true}
	}
	return dialClass{rep: repFailure}
}

type dialResult struct {
	conn net.Conn
	sl   *Slot
	err  error
}

// DialTCP connects to host:port over one NIC, failing over to the others on link-level errors.
// If the first NIC hasn't connected after RaceAfter, another NIC is tried in parallel and the first
// to connect wins, so a network that went dead or stalls costs about RaceAfter instead of the whole
// connect timeout; the slow one is suspected and re-probed. On success the caller owns the
// returned slot and must Relay or Release it.
func (b *Balancer) DialTCP(ctx context.Context, host string, port int) (net.Conn, *Slot, Family, error) {
	// The destination's family decides which NIC address is used; a host name stays IPv4.
	f, network := Fam4, "tcp4"
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		f, network = Fam6, "tcp6"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	tried := map[string]struct{}{}
	rep := byte(repFailure)
	var lastErr error = errors.New("no usable network adapter")
	if f == Fam6 {
		rep, lastErr = repNetUnreachable, errors.New("no usable IPv6 adapter")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // stops the attempts that lost
	results := make(chan dialResult, 4)
	inFlight := map[*Slot]time.Time{} // when each attempt started
	start := func() bool {
		sl := b.S.AcquireFor(f, tried)
		if sl == nil {
			return false
		}
		tried[sl.key] = struct{}{}
		inFlight[sl] = time.Now()
		go func() {
			c, err := b.dialOne(ctx, sl, f, network, addr)
			results <- dialResult{c, sl, err}
		}()
		return true
	}
	if !start() {
		return nil, nil, f, &dialError{rep: rep, err: lastErr}
	}
	var race <-chan time.Time
	if b.RaceAfter > 0 {
		t := time.NewTimer(b.RaceAfter)
		defer t.Stop()
		race = t.C
	}
	for len(inFlight) > 0 {
		select {
		case <-race:
			start() // nothing happens if there is no other NIC
		case r := <-results:
			delete(inFlight, r.sl)
			if r.err == nil {
				for sl, t := range inFlight {
					if time.Since(t) >= b.RaceAfter {
						b.S.Suspect(sl, f) // still connecting after RaceAfter while another NIC answered
					}
				}
				go b.drain(results, len(inFlight), f)
				return r.conn, r.sl, f, nil
			}
			b.S.ReleaseFor(r.sl, f)
			r.sl.noteErr(r.err)
			c := classify(r.err)
			if c.linkDown {
				b.S.Fail(r.sl, f, b.FailCooldown)
			} else if c.retry {
				b.S.Suspect(r.sl, f) // a timeout: check the NIC now rather than at its next probe
			}
			rep, lastErr = c.rep, r.err
			if c.retry && ctx.Err() == nil && len(inFlight) == 0 {
				start()
			}
		}
	}
	return nil, nil, f, &dialError{rep: rep, err: lastErr}
}

func (b *Balancer) dialOne(ctx context.Context, sl *Slot, f Family, network, addr string) (net.Conn, error) {
	d := net.Dialer{
		Control:   nics.ForceEgress(sl.handle()), // pins the flow to this network and keeps it out of the VPN
		Timeout:   b.ConnectTimeout,
		KeepAlive: 30 * time.Second,
	}
	if ip := sl.ip(f); !ip.IsUnspecified() {
		d.LocalAddr = &net.TCPAddr{IP: ip} // and to the source address, when one is given
	}
	return d.DialContext(ctx, network, addr)
}

// drain collects the attempts that lost a race: their slots are released and a connection that
// still got through is closed.
func (b *Balancer) drain(results <-chan dialResult, n int, f Family) {
	for ; n > 0; n-- {
		r := <-results
		if r.err == nil {
			r.conn.Close()
		}
		b.S.ReleaseFor(r.sl, f)
	}
}

type dialError struct {
	rep byte
	err error
}

func (e *dialError) Error() string { return e.err.Error() }
func (e *dialError) Unwrap() error { return e.err }

var bufPool = sync.Pool{New: func() any { b := make([]byte, 128<<10); return &b }}

// Relay copies both directions until both sides finish, counting bytes on the slot, then
// closes both conns and releases the slot. Half-close is honoured so request/response
// protocols that shut down their write side still receive the reply.
func Relay(client, upstream net.Conn, sl *Slot, f Family, s *Scheduler) {
	var wg sync.WaitGroup
	var once sync.Once
	release := func() { once.Do(func() { s.ReleaseFor(sl, f) }) } // at the first half-close, see slotConn
	wg.Add(1)
	go func() {
		defer wg.Done()
		pump(upstream, client, &sl.bytesOut[f])
		release()
	}()
	pump(client, upstream, &sl.bytesIn[f])
	release()
	wg.Wait()
	client.Close()
	upstream.Close()
}

func pump(dst, src net.Conn, counter interface{ Add(int64) int64 }) {
	bp := bufPool.Get().(*[]byte)
	defer bufPool.Put(bp)
	buf := *bp
	for {
		n, err := src.Read(buf)
		if n > 0 {
			counter.Add(int64(n))
			if _, werr := dst.Write(buf[:n]); werr != nil {
				dst.Close()
				src.Close()
				return
			}
		}
		if err != nil {
			if err == io.EOF {
				if cw, ok := dst.(interface{ CloseWrite() error }); ok {
					cw.CloseWrite()
				} else {
					dst.Close()
				}
			} else {
				dst.Close()
				src.Close()
			}
			return
		}
	}
}
