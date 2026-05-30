// Package network is LivingWorld's Java<->Bedrock protocol bridge (DESIGN §2
// edge/translation layer). It routes packets between Java (TCP/25565) and
// Bedrock (UDP/19132) clients over one canonical model: each edge translates
// its wire packets up to canonical, the Bridge routes canonical packets, and
// each edge translates them back down — gameplay never branches on edition.
//
// Canonical data types live in internal/registry and are imported, not
// redefined. Edition is defined here because registry intentionally defers it
// (and the internal/version lane is not built yet); keep the values stable.
package network

import "fmt"

// Edition identifies a client's wire-protocol family.
type Edition uint8

const (
	Java Edition = iota
	Bedrock
)

func (e Edition) String() string {
	switch e {
	case Java:
		return "java"
	case Bedrock:
		return "bedrock"
	default:
		return fmt.Sprintf("edition(%d)", uint8(e))
	}
}

// Transport is the L4 transport an edition listens on.
type Transport uint8

const (
	TCP Transport = iota // Java
	UDP                  // Bedrock (RakNet)
)

func (t Transport) String() string {
	if t == UDP {
		return "udp"
	}
	return "tcp"
}

// Endpoint describes where an edition accepts clients. By default Java =
// TCP/25565 and Bedrock = UDP/19132 (PROTOCOL.md).
type Endpoint struct {
	Edition   Edition
	Transport Transport
	Addr      string
}

// DefaultEndpoints returns the two-edition listen set for the given bind
// addresses (e.g. cfg.Address() and cfg.BedrockAddress()).
func DefaultEndpoints(javaAddr, bedrockAddr string) []Endpoint {
	return []Endpoint{
		{Edition: Java, Transport: TCP, Addr: javaAddr},
		{Edition: Bedrock, Transport: UDP, Addr: bedrockAddr},
	}
}

// PacketKind is the edition-agnostic class of a routed packet. The concrete
// canonical payload (registry types) rides in Frame.Payload; translators map
// each kind to/from per-edition wire ids.
type PacketKind uint16

const (
	KindUnknown PacketKind = iota // sentinel for unmapped wire packets (§12)
	KindKeepAlive
	KindChat
	KindMove
	KindBlockUpdate
)

// Packet is a canonical packet flowing through the Bridge.
type Packet interface{ Kind() PacketKind }

// Frame is the default canonical packet carrier. Payload holds the canonical
// model value (e.g. registry types) once codecs are implemented.
type Frame struct {
	PacketKind PacketKind
	Payload    any
}

func (f Frame) Kind() PacketKind { return f.PacketKind }

// RawPacket is an edition wire packet (id + body) at the edge boundary.
type RawPacket struct {
	ID   int
	Data []byte
}

// State is a connection's lifecycle phase (PROTOCOL.md connect flow).
type State uint8

const (
	StateHandshake State = iota
	StateLogin
	StatePlay
	StateDisconnected
)

func (s State) String() string {
	switch s {
	case StateHandshake:
		return "handshake"
	case StateLogin:
		return "login"
	case StatePlay:
		return "play"
	default:
		return "disconnected"
	}
}
