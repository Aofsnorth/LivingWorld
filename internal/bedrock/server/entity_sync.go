package server

import (
	"log"

	"livingworld/internal/player"
	"livingworld/internal/bedrock/skin"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// startPlayerEventLoop bridges shared player.Manager events into Bedrock
// viewers. Bedrock->Bedrock is handled here too; clients only ignore their own
// UUID, not the whole Bedrock edition.
func (s *Server) startPlayerEventLoop() {
	if s.playerEvents != nil {
		return
	}
	s.playerEvents = s.pm.Subscribe("bedrock-server", 256)
	go func() {
		for ev := range s.playerEvents {
			s.forEachSession(func(viewer *bedrockSession) {
				if ev.Player.UUID == viewer.id {
					return
				}
				switch ev.Type {
				case player.EventJoin:
					s.spawnPlayerFor(viewer, ev.Player)
				case player.EventMove:
					s.movePlayerFor(viewer, ev.Player, ev.Teleport)
				case player.EventLeave:
					s.removePlayerFor(viewer, ev.Player)
				case player.EventSwing:
					s.swingPlayerFor(viewer, ev.Player)
				case player.EventSneak:
					s.updateSneakFor(viewer, ev.Player)
				}
			})
		}
	}()
}

func (s *Server) spawnExistingForeignPlayers(viewer *bedrockSession) {
	for _, p := range s.pm.GetAllPlayers() {
		if p.UUID == viewer.id {
			continue
		}
		s.spawnPlayerFor(viewer, p.Snapshot())
	}
}

func (s *Server) spawnPlayerFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	if p.Edition == player.EditionBedrock {
		if target, ok := s.getSession(p.UUID); ok {
			viewer.spawnBedrockPlayer(target, p)
		}
		return
	}
	viewer.spawnJavaPlayer(p)
}

func (s *Server) movePlayerFor(viewer *bedrockSession, p player.PlayerSnapshot, teleport bool) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	// MoveActorAbsolute reliably updates a remote entity's rendered body AND head
	// rotation (Rotation packs {pitch, yaw, headYaw}); this is what dragonfly and
	// Geyser use for non-self entities. MovePlayer/MoveModeTeleport only corrects
	// position and left the Java player facing the wrong way on Bedrock.
	flags := byte(0)
	if teleport {
		flags |= packet.MoveFlagTeleport
	}
	if p.OnGround {
		flags |= packet.MoveFlagOnGround
	}
	viewer.write(&packet.MoveActorAbsolute{
		EntityRuntimeID: p.EntityRuntimeID,
		Position:        bedrockPosForSnapshot(p),
		Rotation:        mgl32.Vec3{p.Rotation.Pitch, p.Rotation.Yaw, p.Rotation.Yaw},
		Flags:           flags,
	})
}

func (s *Server) updateSneakFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	viewer.write(&packet.SetActorData{
		EntityRuntimeID: p.EntityRuntimeID,
		EntityMetadata:  bedrockMetadata(p.Username, p.Sneaking),
	})
}

func (s *Server) removePlayerFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	viewer.write(&packet.PlayerList{ActionType: packet.PlayerListActionRemove, Entries: []protocol.PlayerListEntry{{UUID: p.UUID}}})
	viewer.write(&packet.RemoveActor{EntityUniqueID: int64(p.EntityRuntimeID)})
}

func (s *Server) swingPlayerFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	viewer.write(&packet.Animate{
		ActionType:      packet.AnimateActionSwingArm,
		EntityRuntimeID: p.EntityRuntimeID,
	})
}

func (viewer *bedrockSession) spawnBedrockPlayer(target *bedrockSession, p player.PlayerSnapshot) {
	sk := skin.SkinFromClientData(target.clientData)
	entry := protocol.PlayerListEntry{
		UUID:           target.id,
		EntityUniqueID: int64(target.runtimeID),
		Username:       target.username,
		XUID:           target.identity.XUID,
		PlatformChatID: "",
		BuildPlatform:  int32(target.clientData.DeviceOS),
		Skin:           sk,
	}
	viewer.write(&packet.PlayerList{ActionType: packet.PlayerListActionAdd, Entries: []protocol.PlayerListEntry{entry}})
	viewer.write(&packet.AddPlayer{
		UUID:             target.id,
		Username:         target.username,
		EntityRuntimeID:  target.runtimeID,
		PlatformChatID:   "",
		Position:         bedrockPosForSnapshot(p),
		Velocity:         mgl32.Vec3{},
		Pitch:            p.Rotation.Pitch,
		Yaw:              p.Rotation.Yaw,
		HeadYaw:          p.Rotation.Yaw,
		GameType:         0,
		EntityMetadata:   bedrockMetadata(target.username, p.Sneaking),
		EntityProperties: protocol.EntityProperties{},
		AbilityData:      bedrockSurvivalAbilityData(target.runtimeID),
		EntityLinks:      nil,
		DeviceID:         string(target.clientData.DeviceID),
		BuildPlatform:    int32(target.clientData.DeviceOS),
	})
	viewer.write(&packet.PlayerSkin{UUID: target.id, Skin: sk})
	log.Printf("[Bedrock] spawned Bedrock player %s for Bedrock viewer %s", target.username, viewer.username)
}

func (viewer *bedrockSession) spawnJavaPlayer(p player.PlayerSnapshot) {
	s := skin.JavaFallbackSkinForViewer(p)
	entry := protocol.PlayerListEntry{
		UUID:           p.UUID,
		EntityUniqueID: int64(p.EntityRuntimeID),
		Username:       p.Username,
		XUID:           "",
		PlatformChatID: "",
		Skin:           s,
	}
	viewer.write(&packet.PlayerList{ActionType: packet.PlayerListActionAdd, Entries: []protocol.PlayerListEntry{entry}})
	viewer.write(&packet.AddPlayer{
		UUID:             p.UUID,
		Username:         p.Username,
		EntityRuntimeID:  p.EntityRuntimeID,
		PlatformChatID:   "",
		Position:         bedrockPosForSnapshot(p),
		Velocity:         mgl32.Vec3{},
		Pitch:            p.Rotation.Pitch,
		Yaw:              p.Rotation.Yaw,
		HeadYaw:          p.Rotation.Yaw,
		GameType:         0,
		EntityMetadata:   bedrockMetadata(p.Username, p.Sneaking),
		EntityProperties: protocol.EntityProperties{},
		AbilityData:      bedrockSurvivalAbilityData(p.EntityRuntimeID),
		EntityLinks:      nil,
	})
	// Re-send the skin explicitly while the player is in the player list. Some
	// Bedrock clients do not reliably apply synthetic Java skins from PlayerList
	// alone when the viewer just joined.
	viewer.write(&packet.PlayerSkin{UUID: p.UUID, Skin: s})
	log.Printf("[Bedrock] spawned Java player %s for Bedrock viewer %s", p.Username, viewer.username)
}

func bedrockPosForSnapshot(p player.PlayerSnapshot) mgl32.Vec3 {
	if p.Edition == player.EditionJava {
		return bedrockPosFromJavaFeet(p.Position.X, p.Position.Y, p.Position.Z)
	}
	return bedrockPosFromFeet(p.Position.X, p.Position.Y, p.Position.Z)
}
