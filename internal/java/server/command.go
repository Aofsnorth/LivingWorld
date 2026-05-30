package server

import (
	"fmt"

	"livingworld/internal/command"
	"livingworld/internal/item"
	"livingworld/internal/player"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// HandleChatCommand handles ServerboundChatCommand (a client typing "/cmd"). In
// 26.1 the unsigned command packet body is a single String (the command without
// the leading '/'). We read it and dispatch through the shared command registry.
func (s *PlayerSession) HandleChatCommand(p pk.Packet) {
	var cmd pk.String
	if err := p.Scan(&cmd); err != nil {
		return
	}
	command.Default().Dispatch(s, string(cmd))
}

// --- command.Sender implementation on the Java session ---

func (s *PlayerSession) Name() string { return s.UsernameVal }

func (s *PlayerSession) IsOp() bool {
	if pl := s.Bridge.pm.GetPlayer(s.UUIDVal); pl != nil {
		return pl.Op
	}
	return false
}

func (s *PlayerSession) Edition() player.Edition { return player.EditionJava }

func (s *PlayerSession) Reply(msg string) { s.sendSystemMessage(msg) }

// SetGameMode updates the server-side gamemode and tells the client via the
// GameEvent packet (event 3 = change gamemode; the byte+float structure is the
// same one already used for the chunk-wait event).
func (s *PlayerSession) SetGameMode(mode int) error {
	if mode < 0 || mode > 3 {
		return fmt.Errorf("invalid gamemode %d", mode)
	}
	s.mu.Lock()
	s.GameModeVal = int32(mode)
	s.mu.Unlock()
	return s.SendPacket(pk.Marshal(
		packetid.ClientboundGameGameEvent,
		pk.UnsignedByte(3), // event 3: change gamemode
		pk.Float(float32(mode)),
	))
}

// Teleport moves the player. Reuses the spawn-position packet path (absolute
// position + same dimension).
func (s *PlayerSession) Teleport(x, y, z float64) error {
	s.mu.Lock()
	s.X, s.Y, s.Z = x, y, z
	yaw, pitch := s.Yaw, s.Pitch
	s.mu.Unlock()
	s.Bridge.pm.UpdatePosition(s.UUIDVal, x, y, z, pitch, yaw, false)
	return s.SendPacket(pk.Marshal(
		packetid.ClientboundGamePlayerPosition,
		pk.VarInt(0), // teleport id
		pk.Double(x), pk.Double(y), pk.Double(z),
		pk.Double(0), pk.Double(0), pk.Double(0), // velocity
		pk.Float(yaw), pk.Float(pitch),
		pk.Int(0), // relative flags = none (all absolute)
	))
}

// GiveItem is not yet wired on Java: the 26.1 SetContainerSlot item-stack format
// uses data components that we have not verified, and sending a wrong shape would
// crash the client (the project's core lesson). We validate the item name so the
// command still gives correct feedback, and defer the actual packet until the
// item-stack encoding is verified.
func (s *PlayerSession) GiveItem(itemName string, count int) error {
	if _, ok := item.ByName(itemName); !ok {
		return fmt.Errorf("unknown item %q", itemName)
	}
	return fmt.Errorf("/give is not yet supported on Java (item-stack encoding pending verification)")
}
