package server

import (
	"fmt"

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
	text := fmt.Sprintf("<%s> %s", s.Username(), string(message))
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
