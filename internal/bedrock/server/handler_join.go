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

func (s *Server) sendBedrockSurvivalState(conn *minecraft.Conn, runtimeID uint64) {
	// Reassert survival gamemode.
	_ = conn.WritePacket(&packet.SetPlayerGameType{GameType: packet.GameTypeSurvival})

	// Send UpdateAbilities so the Bedrock client always uses the correct
	// survival walk/fly speeds.  Without this the client may drift into a
	// faster default speed after a gamemode/ability request cycle.
	_ = conn.WritePacket(&packet.UpdateAbilities{AbilityData: bedrockSurvivalAbilityData(runtimeID)})

	// Bedrock's actual walking speed is driven by the movement attribute. If it
	// is omitted, some clients keep a stale/non-survival value and move far too
	// fast even though the gamemode is survival.
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
