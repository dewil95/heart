package split

import (
	"context"
	"net"
	"sync"
)

// Balancer spreads new connections across several links, sending each to the
// link with the fewest connections in flight (ties broken round-robin). This
// combines the networks' speed for anything that opens many connections — a web
// page with many resources, or a multi-part download. A single connection still
// uses one network, so one plain download stream does not get faster.
type Balancer struct {
	links  []*Link
	mu     sync.Mutex
	active []int
	rr     int
}

func NewBalancer(links []*Link) *Balancer {
	return &Balancer{links: links, active: make([]int, len(links))}
}

// pick returns the index of the link to use and counts a flow against it.
func (b *Balancer) pick() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	min := b.active[0]
	for _, a := range b.active[1:] {
		if a < min {
			min = a
		}
	}
	// Among the links tied at the fewest flows, take the next one round-robin
	// so equal networks share evenly.
	n := len(b.links)
	chosen := 0
	for k := 0; k < n; k++ {
		i := (b.rr + k) % n
		if b.active[i] == min {
			chosen = i
			break
		}
	}
	b.rr = (chosen + 1) % n
	b.active[chosen]++
	return chosen
}

func (b *Balancer) done(i int) {
	b.mu.Lock()
	if b.active[i] > 0 {
		b.active[i]--
	}
	b.mu.Unlock()
}

// Dial picks a link and opens the connection on it, releasing the flow count
// when the connection is closed.
func (b *Balancer) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	i := b.pick()
	c, err := b.links[i].Dial(ctx, network, addr)
	if err != nil {
		b.done(i)
		return nil, err
	}
	return &countedConn{Conn: c, b: b, i: i}, nil
}

// countedConn releases its link's flow count once, when closed.
type countedConn struct {
	net.Conn
	b    *Balancer
	i    int
	once sync.Once
}

func (c *countedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.b.done(c.i) })
	return err
}

// CloseWrite forwards the half-close the relay uses, when the link supports it.
func (c *countedConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}
