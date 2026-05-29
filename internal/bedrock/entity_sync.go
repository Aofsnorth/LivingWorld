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
	viewer.spawnJavaAvatar(p)
}

func (s *Server) movePlayerFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	viewer.write(&packet.MoveActorAbsolute{
		EntityRuntimeID: p.EntityRuntimeID,
		Flags:           packet.MoveFlagOnGround,
		Position:        bedrockPosFromFeet(p.Position.X, p.Position.Y, p.Position.Z),
		Rotation:        mgl32.Vec3{p.Rotation.Pitch, p.Rotation.Yaw, p.Rotation.Yaw},
	})
}

func (s *Server) removePlayerFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	if p.Edition == player.EditionBedrock {
		viewer.write(&packet.PlayerList{ActionType: packet.PlayerListActionRemove, Entries: []protocol.PlayerListEntry{{UUID: p.UUID}}})
	}
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
		Position:         bedrockPosFromFeet(p.Position.X, p.Position.Y, p.Position.Z),
		Velocity:         mgl32.Vec3{},
		Pitch:            p.Rotation.Pitch,
		Yaw:              p.Rotation.Yaw,
		HeadYaw:          p.Rotation.Yaw,
		GameType:         0, // survival
		EntityMetadata:   bedrockMetadata(target.username),
		EntityProperties: protocol.EntityProperties{},
		AbilityData:      bedrockSurvivalAbilityData(target.runtimeID),
		EntityLinks:      nil,
		DeviceID:         string(target.clientData.DeviceID),
		BuildPlatform:    int32(target.clientData.DeviceOS),
	})
	log.Printf("[Bedrock] spawned Bedrock player %s for Bedrock viewer %s", target.username, viewer.username)
}

func (viewer *bedrockSession) spawnJavaAvatar(p player.PlayerSnapshot) {
	// Java skins/profiles require a separate Java->Bedrock skin translation
	// pipeline. Until that exists, Java players remain an explicit avatar actor
	// for Bedrock viewers, while Bedrock<->Bedrock uses real player models.
	viewer.write(&packet.AddActor{
		EntityUniqueID:   int64(p.EntityRuntimeID),
		EntityRuntimeID:  p.EntityRuntimeID,
		EntityType:       "minecraft:zombie",
		Position:         bedrockPosFromFeet(p.Position.X, p.Position.Y, p.Position.Z),
		Velocity:         mgl32.Vec3{},
		Pitch:            p.Rotation.Pitch,
		Yaw:              p.Rotation.Yaw,
		HeadYaw:          p.Rotation.Yaw,
		BodyYaw:          p.Rotation.Yaw,
		EntityMetadata:   bedrockMetadata(p.Username + " [Java]"),
		EntityProperties: protocol.EntityProperties{},
		Attributes:       nil,
		EntityLinks:      nil,
	})
	log.Printf("[Bedrock] spawned Java avatar %s for Bedrock viewer %s", p.Username, viewer.username)
}
