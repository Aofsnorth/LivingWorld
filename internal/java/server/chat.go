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
	text := fmt.Sprintf("<%s> %s", s.Username(), ev.Message)
	s.Bridge.sessions.Broadcast(pk.Marshal(
		packetid.ClientboundGameSystemChat,
		pk.NBT(chatText{Text: text}),
		pk.Boolean(false),
	))
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
