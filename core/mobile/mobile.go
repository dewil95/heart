// Package mobile is the API the Android app calls through gomobile bind (Java class
// mobile.Mobile). The app owns everything Android-specific: the VPN adapter, the list of networks
// and pinning sockets to them. This side runs net-merge's balancer on top.
package mobile

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/dewil95/net-merge-android/core/engine"
	"github.com/dewil95/net-merge-android/core/nics"
	"github.com/dewil95/net-merge-android/core/tunfd"
)

// Version is shown in the app.
const Version = "0.1.0"

// DefaultMTU is the MTU the app should give the VpnService adapter (and pass to Start).
const DefaultMTU = tunfd.DefaultMTU

// SocketBinder is implemented by the app's VpnService.
type SocketBinder interface {
	// BindSocket must exempt fd from the VPN (VpnService.protect) and bind it to the network with
	// this handle (Network.bindSocket). It returns false if that network is gone.
	BindSocket(fd int32, networkHandle int64) bool
}

// network is one entry of the JSON list SetNetworks takes.
type network struct {
	Name   string `json:"name"`  // interface name, e.g. wlan0 or rmnet_data0
	Label  string `json:"label"` // shown in the app, e.g. "Wi-Fi"
	Handle int64  `json:"handle"`
	IP4    string `json:"ip4"` // for display; may be empty
	IP6    string `json:"ip6"` // global IPv6, for display; may be empty
	V4     bool   `json:"v4"`  // carries IPv4 (it may have no IPv4 address of its own: 464XLAT)
	V6     bool   `json:"v6"`  // carries IPv6 (has a global IPv6 address)
}

type session struct {
	sched *engine.Scheduler
	bal   *engine.Balancer
	tun   *tunfd.Tun
	stop  chan struct{}
	since time.Time
}

var (
	life sync.Mutex // serializes Start and Stop

	mu     sync.Mutex // guards the fields below
	nets   []network
	run    *session
	v6mode = engine.V6Auto
)

// SetNetworks replaces the list of networks flows may use: a JSON array of objects with name,
// label, handle, ip4, ip6, v4 and v6 (see network). It can be called before Start, and again
// whenever a network appears, changes or goes away.
func SetNetworks(jsonList string) error {
	var in []network
	if err := json.Unmarshal([]byte(jsonList), &in); err != nil {
		return fmt.Errorf("networks: %w", err)
	}
	mu.Lock()
	defer mu.Unlock()
	nets = in
	if run != nil {
		run.sched.Update(toNics(in))
	}
	return nil
}

// toNics leaves every source address unspecified: the network binding makes the kernel pick the
// right one, including on 464XLAT and with IPv6 privacy addresses.
func toNics(in []network) []nics.Nic {
	out := make([]nics.Nic, 0, len(in))
	for _, n := range in {
		if n.Handle == 0 || (!n.V4 && !n.V6) {
			continue
		}
		nic := nics.Nic{Name: n.Name, Handle: n.Handle, Weight: 1}
		if n.V4 {
			nic.IP = net.IPv4zero
		}
		if n.V6 {
			nic.IP6 = net.IPv6unspecified
		}
		out = append(out, nic)
	}
	return out
}

// Start takes ownership of tunFd (ParcelFileDescriptor.detachFd of the established VPN) and
// starts merging. A running session is stopped first. On error the fd is already closed.
func Start(tunFd, mtu int32, binder SocketBinder) error {
	if binder == nil {
		syscall.Close(int(tunFd))
		return errors.New("start: no socket binder")
	}
	life.Lock()
	defer life.Unlock()
	stopLocked()

	nics.SetBinder(func(fd int, handle int64) error {
		if !binder.BindSocket(int32(fd), handle) {
			return errors.New("network is gone")
		}
		return nil
	})
	mu.Lock()
	sched := engine.NewScheduler(toNics(nets))
	mode := v6mode
	mu.Unlock()
	sched.SetAuto(true) // weights follow measured throughput
	bal := engine.NewBalancer(sched)
	bal.SetV6Mode(mode)
	t, err := tunfd.Start(int(tunFd), int(mtu), bal)
	if err != nil {
		nics.SetBinder(nil)
		return err
	}
	s := &session{sched: sched, bal: bal, tun: t, stop: make(chan struct{}), since: time.Now()}
	go sched.RunProbes(s.stop)
	go s.tick()
	mu.Lock()
	run = s
	mu.Unlock()
	return nil
}

func (s *session) tick() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	last := time.Now()
	for {
		select {
		case <-s.stop:
			return
		case now := <-t.C:
			s.sched.Tick(now.Sub(last))
			last = now
		}
	}
}

// Stop ends merging and closes the TUN fd. It is safe to call when not running.
func Stop() {
	life.Lock()
	defer life.Unlock()
	stopLocked()
}

func stopLocked() {
	mu.Lock()
	s := run
	run = nil
	mu.Unlock()
	if s == nil {
		return
	}
	close(s.stop)
	s.tun.Close()
	nics.SetBinder(nil)
}

// SetIPv6Mode is "auto" (IPv6 is balanced only when every network has it, otherwise refused so
// apps fall back to IPv4 and use every network), "on" or "off".
func SetIPv6Mode(mode string) error {
	m, ok := engine.ParseV6Mode(mode)
	if !ok {
		return fmt.Errorf("unknown IPv6 mode %q (want auto, on or off)", mode)
	}
	mu.Lock()
	defer mu.Unlock()
	v6mode = m
	if run != nil {
		run.bal.SetV6Mode(m)
	}
	return nil
}

type linkStatus struct {
	Name     string  `json:"name"`
	Label    string  `json:"label"`
	IP4      string  `json:"ip4,omitempty"`
	IP6      string  `json:"ip6,omitempty"`
	State    string  `json:"state"`  // IPv4 probe: checking, up, no internet; "-" without IPv4
	State6   string  `json:"state6"` // same for IPv6
	Present  bool    `json:"present"`
	Active   int64   `json:"active"` // flows now
	Total    int64   `json:"total"`  // flows since start
	Failed   int64   `json:"failed"`
	RateIn   float64 `json:"rateIn"` // bytes/s over the last second
	RateOut  float64 `json:"rateOut"`
	BytesIn  int64   `json:"bytesIn"`
	BytesOut int64   `json:"bytesOut"`
	Weight   float64 `json:"weight"`
}

type status struct {
	Running bool         `json:"running"`
	Since   int64        `json:"since,omitempty"` // unix ms
	IPv6    string       `json:"ipv6,omitempty"`
	Links   []linkStatus `json:"links"`
}

// Status returns the live numbers as JSON (see status).
func Status() string {
	mu.Lock()
	s := run
	byName := make(map[string]network, len(nets))
	for _, n := range nets {
		byName[n.Name] = n
	}
	mu.Unlock()

	st := status{Links: []linkStatus{}}
	if s != nil {
		st.Running, st.Since, st.IPv6 = true, s.since.UnixMilli(), s.bal.V6State()
		for _, l := range s.sched.Snapshot() {
			n := byName[l.Name]
			st.Links = append(st.Links, linkStatus{
				Name: l.Name, Label: n.Label, IP4: n.IP4, IP6: n.IP6,
				State: l.State, State6: l.State6, Present: l.Present,
				Active: l.Active, Total: l.Total, Failed: l.Failed,
				RateIn: l.RateIn, RateOut: l.RateOut, BytesIn: l.BytesIn, BytesOut: l.BytesOut,
				Weight: l.Weight,
			})
		}
	}
	b, _ := json.Marshal(st)
	return string(b)
}
