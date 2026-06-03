package server

import (
	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/nbt"
	pk "github.com/Tnze/go-mc/net/packet"
)

func (s *PlayerSession) sendInitialPlayPackets() error {
	return s.version.SendInitialPlayPackets(s)
}

// NOTE: the ClientboundGameCommands packet is intentionally NOT sent.
//
// History:
//   - Configuration phase (id 16 = ServerLinks in 26.1.2): crashes the
//     client with "Expected non-null compound tag" because the client
//     tries to decode the command graph as a ServerLinks NBT compound.
//   - Play phase (id 16 = Commands): no longer crashes on id, but the
//     vendored go-mc command.Graph encoder targets an older protocol; the
//     26.1.2 ClientboundCommandsPacket.readNode decoder runs short of
//     bytes mid-read ("expected 112, but got 104") on every command name
//     because node field shapes (argumentType is now a registry holder,
//     etc.) changed between the vendored go-mc's target version and 26.1.
//
// Server-side command execution still works — command.Default().Dispatch()
// is wired through HandleChatCommand in the chat pipeline and resolves
// every registered command by name. The only loss is client-side tab
// completion and the parsed syntax tree; the client shows "Unknown
// command" the instant a player types '/'. Re-enable when vendored go-mc
// is updated to 26.1 wire format for the command graph.

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
