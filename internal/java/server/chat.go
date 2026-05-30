package server

import (
	"fmt"
	"io"

	"livingworld/internal/command"
	"livingworld/plugin"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

type chatText struct {
	Text string `nbt:"text"`
}

func (s *PlayerSession) HandleChat(p pk.Packet) {
	var message pk.String
	if err := p.Scan(&message); err != nil {
		return
	}
	// Some clients deliver "/cmd" as plain chat; route it to the command system.
	if len(message) > 0 && message[0] == '/' {
		command.Default().Dispatch(s, string(message[1:]))
		return
	}
	ev := &plugin.PlayerChatEvent{
		BaseEvent:  plugin.BaseEvent{Type_: plugin.EventPlayerChat},
		PlayerName: s.Username(),
		Message:    string(message),
	}
	if plugin.Manager().EmitCancellable(ev) {
		return // a plugin suppressed the message
	}
	// Broadcast through the shared player manager so the message reaches BOTH
	// editions (each Controller delivers it in its protocol's chat format).
	s.Bridge.pm.Broadcast(fmt.Sprintf("<%s> %s", s.Username(), ev.Message))
}

func (s *PlayerSession) sendSystemMessage(text string) {
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameSystemChat,
		pk.NBT(chatText{Text: text}),
		pk.Boolean(false),
	))
}

// SendMessage implements player.Controller: deliver a chat line to this client.
func (s *PlayerSession) SendMessage(text string) { s.sendSystemMessage(text) }

// Kick implements player.Controller: disconnect this client with a reason.
func (s *PlayerSession) Kick(reason string) {
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameDisconnect,
		pk.NBT(chatText{Text: reason}),
	))
	_ = s.Conn_.Close()
}

// Push implements player.Controller: apply a velocity impulse (blocks/tick) to
// this player's own entity. Java encodes velocity as int16 = blocks/tick*8000,
// which the client applies as knockback — the same path vanilla uses.
func (s *PlayerSession) Push(vx, vy, vz float64) {
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameSetEntityMotion,
		pk.VarInt(s.EntityID()),
		toLpVec3(vx, vy, vz),
	))
}

type lpVec3 []byte

func (l lpVec3) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(l)
	return int64(n), err
}

func toLpVec3(x, y, z float64) lpVec3 {
	sanitize := func(v float64) float64 {
		if v < -1.7179869183E10 {
			return -1.7179869183E10
		}
		if v > 1.7179869183E10 {
			return 1.7179869183E10
		}
		return v
	}
	abs := func(v float64) float64 {
		if v < 0 {
			return -v
		}
		return v
	}
	x = sanitize(x)
	y = sanitize(y)
	z = sanitize(z)

	max := abs(x)
	if abs(y) > max {
		max = abs(y)
	}
	if abs(z) > max {
		max = abs(z)
	}

	if max < 3.051944088384301E-5 {
		return lpVec3([]byte{0})
	}

	scale := int64(max)
	if float64(scale) < max {
		scale++
	}

	isPartial := (scale & 3) != scale
	markers := scale
	if isPartial {
		markers = (scale & 3) | 4
	}

	pack := func(v float64) int64 {
		return int64((v*0.5 + 0.5) * 32766.0)
	}

	xn := pack(x/float64(scale)) << 3
	yn := pack(y/float64(scale)) << 18
	zn := pack(z/float64(scale)) << 33

	buffer := markers | xn | yn | zn

	out := make([]byte, 6)
	out[0] = byte(buffer)
	out[1] = byte(buffer >> 8)

	// writeInt writes 4 bytes in Big Endian for the remaining 32 bits (buffer >> 16)
	rem := uint32(buffer >> 16)
	out[2] = byte(rem >> 24)
	out[3] = byte(rem >> 16)
	out[4] = byte(rem >> 8)
	out[5] = byte(rem)

	if isPartial {
		// encode varint for scale >> 2
		val := uint32(scale >> 2)
		for {
			b := byte(val & 0x7F)
			val >>= 7
			if val != 0 {
				b |= 0x80
			}
			out = append(out, b)
			if val == 0 {
				break
			}
		}
	}

	return lpVec3(out)
}
