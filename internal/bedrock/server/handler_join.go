package server

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// sendInitialInventories initializes the player's inventory windows so the
// Bedrock client will actually render the inventory UI when opened. With the
// server-authoritative inventory system the client keeps the screen closed
// (player just freezes) until these windows have been given content, even if
// empty. Sizes match dragonfly: main 36, armour 4, off-hand 1, UI 54.
func sendInitialInventories(conn *minecraft.Conn) {
	send := func(windowID uint32, size int) {
		_ = conn.WritePacket(&packet.InventoryContent{
			WindowID: windowID,
			Content:  make([]protocol.ItemInstance, size),
		})
	}
	send(protocol.WindowIDInventory, 36)
	send(protocol.WindowIDArmour, 4)
	send(protocol.WindowIDOffHand, 1)
	send(protocol.WindowIDUI, 54)
}

func teleportPlayer(conn *minecraft.Conn, pos mgl32.Vec3, pitch, yaw float32) {
	_ = conn.WritePacket(&packet.MovePlayer{
		EntityRuntimeID: bedrockLocalRuntime,
		Position:        pos,
		Pitch:           pitch,
		Yaw:             yaw,
		HeadYaw:         yaw,
		Mode:            packet.MoveModeTeleport,
		OnGround:        true,
		TeleportCause:   packet.TeleportCauseCommand,
	})
}

// sendBedrockGameMode re-asserts the given gamemode on the client, plus the
// ability data and movement attribute that the client resets whenever
// SetPlayerGameType fires. mode is the Java-style int (0 survival, 1 creative,
// 2 adventure, 3 spectator); use javaModeToBedrock to translate from
// config.DefaultGamemode / pl.Gamemode.
func (s *Server) sendBedrockGameMode(conn *minecraft.Conn, runtimeID uint64, mode int) {
	_ = conn.WritePacket(&packet.SetPlayerGameType{GameType: javaModeToBedrock(mode)})
	_ = conn.WritePacket(&packet.UpdateAbilities{AbilityData: bedrockSurvivalAbilityData(runtimeID)})
	_ = conn.WritePacket(&packet.UpdateAttributes{
		EntityRuntimeID: runtimeID,
		Attributes:      []protocol.Attribute{bedrockMovementAttribute()},
	})
}

// sendBedrockSurvivalState is the legacy hardcoded-survival entry point.
// Kept for the one call site that needs to forcibly snap a non-op client
// back from a self-gamemode change — there we WANT survival regardless of
// the configured default, since the player tried to leave survival.
func (s *Server) sendBedrockSurvivalState(conn *minecraft.Conn, runtimeID uint64) {
	_ = conn.WritePacket(&packet.SetPlayerGameType{GameType: packet.GameTypeSurvival})
	_ = conn.WritePacket(&packet.UpdateAbilities{AbilityData: bedrockSurvivalAbilityData(runtimeID)})
	_ = conn.WritePacket(&packet.UpdateAttributes{
		EntityRuntimeID: runtimeID,
		Attributes:      []protocol.Attribute{bedrockMovementAttribute()},
	})
}

// sendLocalPlayerActorData initializes the local player's actor data so the
// client renders a correct HUD. Without it the air-supply component defaults to
// 0 and the client shows the drowning (air-bubble) bar on dry land. 300 ticks
// (15s) = full air, plus the breathing flag, matching dragonfly's reference.
func (s *Server) sendLocalPlayerActorData(conn *minecraft.Conn) {
	meta := protocol.NewEntityMetadata()
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasGravity)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasCollision)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagBreathing)
	meta[protocol.EntityDataKeyAirSupply] = int16(300)
	meta[protocol.EntityDataKeyAirSupplyMax] = int16(300)
	_ = conn.WritePacket(&packet.SetActorData{
		EntityRuntimeID: bedrockLocalRuntime,
		EntityMetadata:  meta,
	})
}
