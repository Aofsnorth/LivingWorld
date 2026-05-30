package network

import (
	"log"
	"sync"
	"sync/atomic"
)

// Conn is a single client connection at the edge. The Java/Bedrock servers
// implement it; the Bridge drives lifecycle + routing against it. Deliver
// receives a canonical packet and the implementation translates it down to its
// own edition via the matching Translator.
type Conn interface {
	ID() uint64
	Edition() Edition
	State() State
	Deliver(Packet) error
	Close() error
}

// Bridge routes canonical packets between connected Java and Bedrock clients
// and owns connection lifecycle plus the per-edition translators (DESIGN §2).
// It is safe for concurrent use.
type Bridge struct {
	mu          sync.RWMutex
	conns       map[uint64]Conn
	translators map[Edition]Translator
	nextID      atomic.Uint64
}

// NewBridge returns a Bridge with the default Java + Bedrock translators
// registered.
func NewBridge() *Bridge {
	return &Bridge{
		conns: map[uint64]Conn{},
		translators: map[Edition]Translator{
			Java:    javaTranslator{},
			Bedrock: bedrockTranslator{},
		},
	}
}

// NextID allocates a unique connection id (never 0).
func (b *Bridge) NextID() uint64 { return b.nextID.Add(1) }

// Translator returns the edge codec for an edition.
func (b *Bridge) Translator(e Edition) (Translator, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	t, ok := b.translators[e]
	return t, ok
}

// Register adds a connection (lifecycle: connect).
func (b *Bridge) Register(c Conn) {
	b.mu.Lock()
	b.conns[c.ID()] = c
	b.mu.Unlock()
}

// Unregister removes a connection and closes it (lifecycle: disconnect). It is
// idempotent and isolates a single session's teardown (DESIGN §12).
func (b *Bridge) Unregister(id uint64) {
	b.mu.Lock()
	c, ok := b.conns[id]
	delete(b.conns, id)
	b.mu.Unlock()
	if ok {
		_ = c.Close()
	}
}

// Count returns the number of connected clients.
func (b *Bridge) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.conns)
}

// Route fans a canonical packet out to every play-state connection except the
// sender (cross-edition broadcast). A failed Deliver isolates to that
// connection and never aborts the route (DESIGN §12). Returns the delivered
// count.
func (b *Bridge) Route(from uint64, p Packet) int {
	b.mu.RLock()
	targets := make([]Conn, 0, len(b.conns))
	for id, c := range b.conns {
		if id == from || c.State() != StatePlay {
			continue
		}
		targets = append(targets, c)
	}
	b.mu.RUnlock()

	sent := 0
	for _, c := range targets {
		if err := c.Deliver(p); err != nil {
			log.Printf("[network] deliver to conn %d (%s) failed: %v", c.ID(), c.Edition(), err)
			continue
		}
		sent++
	}
	return sent
}

// Broadcast routes a packet to every play-state connection. NextID never
// returns 0, so the zero sender excludes nobody.
func (b *Bridge) Broadcast(p Packet) int { return b.Route(0, p) }
