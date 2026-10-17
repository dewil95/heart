package engine

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dewil95/net-merge-android/core/nics"
)

func nicH(name, ip string, handle int64) nics.Nic {
	return nics.Nic{Name: name, Handle: handle, IP: net.ParseIP(ip), Weight: 1}
}

func TestReconnectRevalidates(t *testing.T) {
	s := NewScheduler([]nics.Nic{nicH("wlan0", "0.0.0.0", 100)})
	sl := s.slots[0]
	s.SetState(sl, Fam4, true)
	s.Update([]nics.Nic{nicH("wlan0", "0.0.0.0", 100)})
	if sl.state[Fam4] != stateUp || sl.handle() != 100 {
		t.Fatal("an unchanged network should keep its state")
	}
	s.Update([]nics.Nic{nicH("wlan0", "0.0.0.0", 200)}) // joined another Wi-Fi
	if sl.handle() != 200 {
		t.Fatalf("handle = %d, want 200", sl.handle())
	}
	if sl.state[Fam4] != stateUnknown {
		t.Fatal("a new network behind the same interface must be probed again")
	}
}

// localEcho accepts and closes connections on 127.0.0.1, standing in for a probe target.
func localEcho(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return ln.Addr().String()
}

type bindLog struct {
	mu      sync.Mutex
	handles []int64
	fail    map[int64]bool
}

func (l *bindLog) bind(fd int, handle int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.handles = append(l.handles, handle)
	if l.fail[handle] {
		return errors.New("network is gone")
	}
	return nil
}

func TestSocketsArePinnedThroughBinder(t *testing.T) {
	target := localEcho(t)
	log := &bindLog{}
	nics.SetBinder(log.bind)
	t.Cleanup(func() { nics.SetBinder(nil) })

	// 0.0.0.0: no source address is bound, the binder alone pins the socket.
	s := NewScheduler([]nics.Nic{nicH("rmnet_data0", "0.0.0.0", 7)})
	old := probeTargets[Fam4]
	defer func() { probeTargets[Fam4] = old }()
	probeTargets[Fam4] = []string{target}
	if !probe(s.slots[0], Fam4) {
		t.Fatal("probe should pass through the binder")
	}
	if len(log.handles) != 1 || log.handles[0] != 7 {
		t.Fatalf("binder calls = %v, want [7]", log.handles)
	}
}

func TestBindFailureFailsOver(t *testing.T) {
	target := localEcho(t)
	host, port, _ := net.SplitHostPort(target)
	log := &bindLog{fail: map[int64]bool{1: true}}
	nics.SetBinder(log.bind)
	t.Cleanup(func() { nics.SetBinder(nil) })

	s := NewScheduler([]nics.Nic{nicH("gone", "0.0.0.0", 1), nicH("ok", "0.0.0.0", 2)})
	s.SetState(s.slots[0], Fam4, true) // the vanished network is tried first
	b := NewBalancer(s)
	p, _ := strconv.Atoi(port)
	c, sl, _, err := b.DialTCP(t.Context(), host, p)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	b.S.ReleaseFor(sl, Fam4)
	if sl.Name != "ok" {
		t.Fatalf("flow went to %s, want ok", sl.Name)
	}
	if s.slots[0].failed.Load() != 1 {
		t.Fatal("a network that can't be bound should be sidelined")
	}
	if st := s.Snapshot()[0]; st.DialErrors != 1 || !strings.Contains(st.LastError, "network is gone") {
		t.Fatalf("the failure should be reported: %d %q", st.DialErrors, st.LastError)
	}
}

// What the emulator showed: mobile data carried only background chatter, Wi-Fi carried a
// download. Chatter is no measurement, so the weights stay equal and the next flows go to the
// NIC with fewer active flows instead of piling onto Wi-Fi.
func TestChatterIsNotAMeasurement(t *testing.T) {
	s := NewScheduler([]nics.Nic{nicH("wlan0", "0.0.0.0", 1), nicH("rmnet0", "0.0.0.0", 2)})
	s.SetAuto(true)
	wifi, mobile := s.slots[0], s.slots[1]
	s.SetState(wifi, Fam4, true)
	s.SetState(mobile, Fam4, true)
	for i := 0; i < 5; i++ {
		wifi.bytesIn[Fam4].Add(12_000_000) // ~96 Mbit/s
		mobile.bytesIn[Fam4].Add(60_000)   // ~0.5 Mbit/s of chatter
		s.Tick(time.Second)
	}
	if wifi.weight[Fam4] != mobile.weight[Fam4] {
		t.Fatalf("chatter must not down-weight mobile data: wifi %.2f, mobile %.2f", wifi.weight[Fam4], mobile.weight[Fam4])
	}
	for i := 0; i < 11; i++ { // Wi-Fi already holds 11 flows, mobile data 2
		wifi.actF[Fam4].Add(1)
	}
	mobile.actF[Fam4].Add(2)
	if got := s.AcquireFor(Fam4, nil); got != mobile {
		t.Fatalf("a new flow should go to the less busy mobile data, went to %s", got.Name)
	}
}
