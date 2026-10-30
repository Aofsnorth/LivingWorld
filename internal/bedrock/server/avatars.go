package server

import (
	"log"

	"livingworld/internal/bedrock/skin"
	"livingworld/internal/entity"
	"livingworld/internal/player"
	"livingworld/internal/registry"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

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

// canonicalPlayer maps the edge player.PlayerSnapshot onto the canonical,
// edition-agnostic entity.Player (registry.Entity base). This is the A4<->A2
// entity_sync seam: both lanes converge on this one shared type so AI,
// anticheat and the dfcompat adapter consume the canonical model instead of a
// per-edge snapshot. Agent-2 migrates the broadcast path onto this at its own
// pace; edition wire downcast (f64->f32 pos/rotation) stays edge-side here.
func canonicalPlayer(p player.PlayerSnapshot) entity.Player {
	return entity.Player{
		Entity: registry.Entity{
			UUID: p.UUID,
			Type: entity.PlayerType,
			Pos:  registry.Vec3{X: p.Position.X, Y: p.Position.Y, Z: p.Position.Z},
		},
		Name:     p.Username,
		Yaw:      float64(p.Rotation.Yaw),
		Pitch:    float64(p.Rotation.Pitch),
		OnGround: p.OnGround,
		Sneaking: p.Sneaking,
	}
}
