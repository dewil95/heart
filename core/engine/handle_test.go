package engine

import (
	"errors"
	"net"
	"strconv"
	"sync"
	"testing"

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
}
