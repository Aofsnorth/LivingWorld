package bedrock

import (
	"log"

	"livingworld/internal/player"

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
					s.movePlayerFor(viewer, ev.Player)
				case player.EventLeave:
					s.removePlayerFor(viewer, ev.Player)
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

func (s *Server) movePlayerFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	mode := byte(packet.MoveModeNormal)
	if p.Edition == player.EditionJava {
		// Java positions arrive less frequently and are already server-absolute.
		// Teleporting remote Java players avoids Bedrock desync where the player
		// body/skin disappears after the first movement update.
		mode = packet.MoveModeTeleport
	}
	viewer.write(&packet.MovePlayer{
		EntityRuntimeID: p.EntityRuntimeID,
		Position:        bedrockPosForSnapshot(p),
		Pitch:           p.Rotation.Pitch,
		Yaw:             p.Rotation.Yaw,
		HeadYaw:         p.Rotation.Yaw,
		Mode:            mode,
		OnGround:        p.OnGround,
		TeleportCause:   packet.TeleportCauseCommand,
	})
}

func (s *Server) removePlayerFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	viewer.write(&packet.PlayerList{ActionType: packet.PlayerListActionRemove, Entries: []protocol.PlayerListEntry{{UUID: p.UUID}}})
	viewer.write(&packet.RemoveActor{EntityUniqueID: int64(p.EntityRuntimeID)})
}

func (viewer *bedrockSession) spawnBedrockPlayer(target *bedrockSession, p player.PlayerSnapshot) {
	entry := protocol.PlayerListEntry{
		UUID:           target.id,
		EntityUniqueID: int64(target.runtimeID),
		Username:       target.username,
		XUID:           target.identity.XUID,
		PlatformChatID: "",
		BuildPlatform:  int32(target.clientData.DeviceOS),
		Skin:           skinFromClientData(target.clientData),
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
		EntityMetadata:   bedrockMetadata(target.username),
		EntityProperties: protocol.EntityProperties{},
		AbilityData:      bedrockSurvivalAbilityData(target.runtimeID),
		EntityLinks:      nil,
		DeviceID:         string(target.clientData.DeviceID),
		BuildPlatform:    int32(target.clientData.DeviceOS),
	})
	log.Printf("[Bedrock] spawned Bedrock player %s for Bedrock viewer %s", target.username, viewer.username)
}

func (viewer *bedrockSession) spawnJavaPlayer(p player.PlayerSnapshot) {
	entry := protocol.PlayerListEntry{
		UUID:           p.UUID,
		EntityUniqueID: int64(p.EntityRuntimeID),
		Username:       p.Username,
		XUID:           "",
		PlatformChatID: "",
		BuildPlatform:  7,
		Skin:           javaFallbackSkinForViewer(viewer),
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
		EntityMetadata:   bedrockMetadata(p.Username),
		EntityProperties: protocol.EntityProperties{},
		AbilityData:      bedrockSurvivalAbilityData(p.EntityRuntimeID),
		EntityLinks:      nil,
		DeviceID:         "java",
		BuildPlatform:    7,
	})
	log.Printf("[Bedrock] spawned Java player %s for Bedrock viewer %s", p.Username, viewer.username)
}

func bedrockPosForSnapshot(p player.PlayerSnapshot) mgl32.Vec3 {
	if p.Edition == player.EditionJava {
		return bedrockPosFromJavaFeet(p.Position.X, p.Position.Y, p.Position.Z)
	}
	return bedrockPosFromFeet(p.Position.X, p.Position.Y, p.Position.Z)
}
