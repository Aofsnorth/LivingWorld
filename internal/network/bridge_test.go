package network

import (
	"errors"
	"testing"
)

type fakeConn struct {
	id        uint64
	edition   Edition
	state     State
	delivered []Packet
	closed    bool
	failNext  bool
}

func (c *fakeConn) ID() uint64       { return c.id }
func (c *fakeConn) Edition() Edition { return c.edition }
func (c *fakeConn) State() State     { return c.state }
func (c *fakeConn) Close() error     { c.closed = true; return nil }
func (c *fakeConn) Deliver(p Packet) error {
	if c.failNext {
		c.failNext = false
		return errors.New("boom")
	}
	c.delivered = append(c.delivered, p)
	return nil
}

func TestBridgeLifecycle(t *testing.T) {
	b := NewBridge()
	c := &fakeConn{id: b.NextID(), edition: Java, state: StatePlay}
	b.Register(c)
	if b.Count() != 1 {
		t.Fatalf("count = %d, want 1", b.Count())
	}
	b.Unregister(c.id)
	if b.Count() != 0 {
		t.Fatalf("count = %d, want 0", b.Count())
	}
	if !c.closed {
		t.Fatal("Unregister did not Close conn")
	}
	b.Unregister(c.id) // idempotent: must not panic
}

func TestBridgeRouteCrossEdition(t *testing.T) {
	b := NewBridge()
	sender := &fakeConn{id: b.NextID(), edition: Java, state: StatePlay}
	bedrock := &fakeConn{id: b.NextID(), edition: Bedrock, state: StatePlay}
	loading := &fakeConn{id: b.NextID(), edition: Java, state: StateLogin}
	for _, c := range []*fakeConn{sender, bedrock, loading} {
		b.Register(c)
	}

	n := b.Route(sender.id, Frame{PacketKind: KindChat})
	if n != 1 {
		t.Fatalf("routed to %d, want 1", n)
	}
	if len(sender.delivered) != 0 {
		t.Fatal("sender should not receive its own packet")
	}
	if len(loading.delivered) != 0 {
		t.Fatal("non-play conn should not receive packet")
	}
	if len(bedrock.delivered) != 1 || bedrock.delivered[0].Kind() != KindChat {
		t.Fatalf("bedrock got %v, want one KindChat", bedrock.delivered)
	}
}

func TestBridgeRouteIsolatesFailure(t *testing.T) {
	b := NewBridge()
	bad := &fakeConn{id: b.NextID(), edition: Java, state: StatePlay, failNext: true}
	good := &fakeConn{id: b.NextID(), edition: Bedrock, state: StatePlay}
	b.Register(bad)
	b.Register(good)
	if n := b.Broadcast(Frame{PacketKind: KindMove}); n != 1 {
		t.Fatalf("delivered %d, want 1 (bad conn isolated)", n)
	}
	if len(good.delivered) != 1 {
		t.Fatal("good conn should still receive after bad conn failed")
	}
}

func TestTranslatorRoundTrip(t *testing.T) {
	for _, e := range []Edition{Java, Bedrock} {
		b := NewBridge()
		tr, ok := b.Translator(e)
		if !ok {
			t.Fatalf("no translator for %s", e)
		}
		// A known kind round-trips: canonical -> wire id -> canonical.
		raw, err := tr.FromCanonical(Frame{PacketKind: KindChat})
		if err != nil {
			t.Fatalf("%s FromCanonical: %v", e, err)
		}
		p, err := tr.ToCanonical(raw)
		if err != nil {
			t.Fatalf("%s ToCanonical: %v", e, err)
		}
		if p.Kind() != KindChat {
			t.Fatalf("%s round-trip kind = %v, want KindChat", e, p.Kind())
		}
		// An unmapped kind is an explicit error, never a silent drop.
		if _, err := tr.FromCanonical(Frame{PacketKind: KindBlockUpdate}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s unsupported kind err = %v, want ErrUnsupported", e, err)
		}
		// An unmapped inbound wire id becomes the sentinel kind (§12).
		if got, _ := tr.ToCanonical(RawPacket{ID: 0x7FFF}); got.Kind() != KindUnknown {
			t.Fatalf("%s unmapped id kind = %v, want KindUnknown", e, got.Kind())
		}
	}
}

func TestDefaultEndpoints(t *testing.T) {
	eps := DefaultEndpoints("0.0.0.0:25565", "0.0.0.0:19132")
	if len(eps) != 2 {
		t.Fatalf("got %d endpoints, want 2", len(eps))
	}
	if eps[0].Edition != Java || eps[0].Transport != TCP {
		t.Errorf("java endpoint = %+v, want java/tcp", eps[0])
	}
	if eps[1].Edition != Bedrock || eps[1].Transport != UDP {
		t.Errorf("bedrock endpoint = %+v, want bedrock/udp", eps[1])
	}
}
