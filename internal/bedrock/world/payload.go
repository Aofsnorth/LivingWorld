package world

import (
	"fmt"

	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// SendSetTime sends the SetTime packet so the Bedrock client shows the correct
// time-of-day (sun position).
func SendSetTime(conn *minecraft.Conn, ticks int32) error {
	return conn.WritePacket(&packet.SetTime{
		Time: ticks,
	})
}

// DimensionMismatchError formats a clear message when the client's block palette
// doesn't match dragonfly's embedded one.
func DimensionMismatchError(clientProtocol int32) string {
	return fmt.Sprintf(
		"Bedrock client protocol %d doesn't match dragonfly's embedded palette (protocol %d). "+
			"Blocks will render invisible. Update dragonfly or use a matching client version.",
		clientProtocol, protocol.CurrentProtocol)
}

// inlinePayloadBuffer is a small helper to build chunk payloads.
// (Separated so we can swap implementations easily.)
func newInlinePayloadBuffer() *inlinePayloadBuffer {
	return &inlinePayloadBuffer{}
}

type inlinePayloadBuffer struct {
	data []byte
}

func (b *inlinePayloadBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *inlinePayloadBuffer) WriteByte(c byte) error {
	b.data = append(b.data, c)
	return nil
}

func (b *inlinePayloadBuffer) Bytes() []byte {
	return b.data
}
