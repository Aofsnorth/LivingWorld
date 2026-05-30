package server

import (
	"fmt"

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
	toShort := func(v float64) pk.Short {
		scaled := v * 8000.0
		if scaled > 32767 {
			scaled = 32767
		} else if scaled < -32768 {
			scaled = -32768
		}
		return pk.Short(int16(scaled))
	}
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameSetEntityMotion,
		pk.VarInt(s.EntityID()),
		toShort(vx), toShort(vy), toShort(vz),
	))
}
