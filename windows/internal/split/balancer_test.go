package split

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestBalancerPicksLeastBusyRoundRobin(t *testing.T) {
	b := NewBalancer([]*Link{{Name: "a"}, {Name: "b"}})
	// With no flows released, four picks should alternate and end balanced.
	got := []int{b.pick(), b.pick(), b.pick(), b.pick()}
	want := []int{0, 1, 0, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pick sequence = %v, want %v", got, want)
		}
	}
	if b.active[0] != 2 || b.active[1] != 2 {
		t.Fatalf("active = %v, want [2 2]", b.active)
	}
}

func TestBalancerReleasesOnClose(t *testing.T) {
	target := echoServer(t)
	defer target.Close()
	_, portStr, _ := net.SplitHostPort(target.Addr().String())
	port, _ := strconv.Atoi(portStr)

	// Two links, both index 0 (no-op pinning on Linux): the balancer spreads
	// connections and relays real data through the proxy.
	b := NewBalancer([]*Link{{Name: "a"}, {Name: "b"}})
	p, err := Listen("127.0.0.1:0", "merge", b.Dial)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := socksConnect(t, p.Addr(), "localhost", port)
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			msg := []byte("merge-test")
			conn.Write(msg)
			got := make([]byte, len(msg))
			if _, err := io.ReadFull(conn, got); err != nil {
				t.Errorf("read: %v", err)
			}
		}()
	}
	wg.Wait()

	// Give Close() callbacks a moment, then every flow must be released.
	time.Sleep(100 * time.Millisecond)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active[0] != 0 || b.active[1] != 0 {
		t.Fatalf("active after close = %v, want [0 0]", b.active)
	}
}

func TestBalancerDialErrorReleases(t *testing.T) {
	// Grab then free a port so dialing it is refused.
	tmp, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := tmp.Addr().String()
	tmp.Close()

	b := NewBalancer([]*Link{{Name: "a"}})
	if _, err := b.Dial(context.Background(), "tcp", addr); err == nil {
		t.Fatal("expected dial error")
	}
	if b.active[0] != 0 {
		t.Fatalf("active after failed dial = %d, want 0", b.active[0])
	}
}
