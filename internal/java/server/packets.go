package server

import (
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/nbt"
	pk "github.com/Tnze/go-mc/net/packet"
)

func (s *PlayerSession) sendInitialPlayPackets() error {
	return s.version.SendInitialPlayPackets(s)
}

func (s *PlayerSession) getChunkCount() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int32(len(s.LoadedChunks))
}

func (s *PlayerSession) sendPlayerInfo() {}

func buildSystemChatPacket(text string, actionBar bool) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameSystemChat,
		pk.NBT(nbt.RawMessage{Type: nbt.TagCompound, Data: func() []byte {
			b, _ := nbt.Marshal(chatText{Text: text})
			return b[3:]
		}()}),
		pk.Boolean(actionBar),
	)
}
