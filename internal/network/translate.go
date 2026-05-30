package network

import "errors"

// ErrUnsupported is returned by a Translator when a canonical packet kind has
// no wire mapping yet. Callers log and skip; they must never silently drop
// (DESIGN §12).
var ErrUnsupported = errors.New("network: unsupported packet")

// Translator is an edition's edge codec: it lifts wire packets up to the
// canonical model and lowers canonical packets back down (DESIGN §5 Codec, §2
// edge layer). Implementations become version-aware once the codec tables land.
type Translator interface {
	Edition() Edition
	ToCanonical(RawPacket) (Packet, error)   // up:   wire -> canonical
	FromCanonical(Packet) (RawPacket, error) // down: canonical -> wire
}

// Stub wire-id tables (canonical kind <-> wire id) standing in for the real
// per-version packet maps. Populated from go-mc/gophertunnel data later.
var (
	javaWireIDs = map[PacketKind]int{
		KindKeepAlive: 0x26,
		KindChat:      0x38,
	}
	bedrockWireIDs = map[PacketKind]int{
		KindKeepAlive: 0x00,
		KindChat:      0x09,
	}
)

// fromCanonical resolves a canonical kind to a wire id via the given table.
func fromCanonical(table map[PacketKind]int, p Packet) (RawPacket, error) {
	id, ok := table[p.Kind()]
	if !ok {
		return RawPacket{}, ErrUnsupported
	}
	return RawPacket{ID: id}, nil
}

// toCanonical resolves a wire id to a canonical kind, falling back to the
// KindUnknown sentinel so unmapped inbound packets are never dropped (§12).
func toCanonical(table map[PacketKind]int, r RawPacket) (Packet, error) {
	for kind, id := range table {
		if id == r.ID {
			return Frame{PacketKind: kind, Payload: r.Data}, nil
		}
	}
	return Frame{PacketKind: KindUnknown, Payload: r.Data}, nil
}

type javaTranslator struct{}

func (javaTranslator) Edition() Edition                        { return Java }
func (javaTranslator) ToCanonical(r RawPacket) (Packet, error) { return toCanonical(javaWireIDs, r) }
func (javaTranslator) FromCanonical(p Packet) (RawPacket, error) {
	return fromCanonical(javaWireIDs, p)
}

type bedrockTranslator struct{}

func (bedrockTranslator) Edition() Edition { return Bedrock }
func (bedrockTranslator) ToCanonical(r RawPacket) (Packet, error) {
	return toCanonical(bedrockWireIDs, r)
}
func (bedrockTranslator) FromCanonical(p Packet) (RawPacket, error) {
	return fromCanonical(bedrockWireIDs, p)
}
