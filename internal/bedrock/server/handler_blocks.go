package server

import (
	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/shared/constants/gameplay"
	"livingworld/internal/world"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func adjacentBlockPos(pos protocol.BlockPos, face int32) protocol.BlockPos {
	x, y, z := pos[0], pos[1], pos[2]
	switch face {
	case 0:
		y--
	case 1:
		y++
	case 2:
		z--
	case 3:
		z++
	case 4:
		x--
	case 5:
		x++
	}
	return protocol.BlockPos{x, y, z}
}

func (s *Server) resyncBedrockBlock(conn *minecraft.Conn, pos protocol.BlockPos) {
	blockID := s.wm.GetDefaultWorld().GetBlock(int(pos[0]), int(pos[1]), int(pos[2])).ID()
	_ = conn.WritePacket(&packet.UpdateBlock{
		Position:          pos,
		NewBlockRuntimeID: bedrockworld.LivingWorldBlockIDToBedrockRID(blockID),
		Flags:             packet.BlockUpdateNetwork | packet.BlockUpdateNeighbours,
		Layer:             0,
	})
}

func (s *Server) breakBedrockBlock(bs *bedrockSession, pos protocol.BlockPos) {
	// Do not allow breaking bedrock or air. This is still a minimal survival
	// placeholder; real hardness/drop logic belongs in a block service.
	current := s.wm.GetDefaultWorld().GetBlock(int(pos[0]), int(pos[1]), int(pos[2]))
	if current.ID() == 0 || current.ID() == 1 {
		return
	}

	// Send the break animation + sound to every Bedrock viewer BEFORE the block
	// becomes air. LevelEventParticlesDestroyBlock (2001) makes the client play
	// the block's own break particles AND its break sound; EventData is the
	// Bedrock runtime ID of the block being destroyed. Without this the block
	// just blinked out of existence on Bedrock (no crack, no particles, no sound).
	s.broadcastBlockBreakEffect(int32(pos[0]), int32(pos[1]), int32(pos[2]), current.ID())
	// Clear any crack overlay this break opened on Java viewers (the overlay is
	// keyed by entity id and isn't cleared by the block turning to air), then
	// render the break particles+sound on Java (the effect bus subscriber skips
	// Bedrock-source events, so no double on Bedrock).
	s.publishCrack(bs, pos, -1)
	s.wm.PublishBlockDestroy(world.BlockUpdateSourceBedrock, bs.id, int(pos[0]), int(pos[1]), int(pos[2]), current.ID())

	// Roll vanilla loot and spawn item entities before the block becomes air.
	s.wm.DropBlockLoot(current.ID(), int(pos[0]), int(pos[1]), int(pos[2]))

	s.wm.SetBlockAndPublish(world.BlockUpdateSourceBedrock, int(pos[0]), int(pos[1]), int(pos[2]), world.BlockAir{})
}

// publishCrack mirrors a Bedrock crack-overlay change to Java via the effect bus.
// stage>=0 starts/updates the overlay, stage<0 clears it.
func (s *Server) publishCrack(bs *bedrockSession, pos protocol.BlockPos, stage int32) {
	s.wm.PublishCrack(world.BlockUpdateSourceBedrock, bs.id, int(pos[0]), int(pos[1]), int(pos[2]), stage)
}

// broadcastBlockBreakEffect plays the destroy-block particles + sound on every
// Bedrock client at the given block position, for the block with world ID
// brokenBlockID.
func (s *Server) broadcastBlockBreakEffect(x, y, z int32, brokenBlockID int32) {
	rid := bedrockworld.LivingWorldBlockIDToBedrockRID(brokenBlockID)
	center := mgl32.Vec3{float32(x) + 0.5, float32(y) + 0.5, float32(z) + 0.5}
	s.forEachSession(func(bs *bedrockSession) {
		bs.write(&packet.LevelEvent{
			EventType: packet.LevelEventParticlesDestroyBlock,
			Position:  center,
			EventData: int32(rid),
		})
	})
}

// bedrockCrackBreakSeconds is the assumed mining duration used to compute the
// crack-progress increment. LivingWorld has no per-block hardness service yet,
// so the actual break is client-timed; this only paces the visual crack overlay.
// The crack is cleared naturally when the block turns to air (on finish) or by
// an explicit StopBlockCracking (on abort), so a slightly-off rate is harmless.
const bedrockCrackBreakSeconds = gameplay.BedrockCrackBreakSeconds

// crackSwitch updates the tracked breaking block for bs and, when the player
// switched from a different block, clears the crack overlay on the old one.
// Returns whether a switch happened (the caller then starts the new block's
// crack). This is what stops a held-break crack from sticking on the previous
// block when the crosshair slides to a new one.
func (s *Server) crackSwitch(bs *bedrockSession, pos protocol.BlockPos) bool {
	hadPrev, px, py, pz := s.wm.CrackManager().StartBreaking(bs.id, int(pos[0]), int(pos[1]), int(pos[2]))
	if hadPrev {
		s.broadcastBlockCracking(protocol.BlockPos{int32(px), int32(py), int32(pz)}, packet.LevelEventStopBlockCracking)
		s.wm.PublishCrack(world.BlockUpdateSourceBedrock, bs.id, px, py, pz, -1) // clear overlay on Java too
	}
	return hadPrev
}

// broadcastBlockCracking drives the progressive crack overlay on every Bedrock
// viewer. eventType is LevelEventStartBlockCracking (begin animating) or
// LevelEventStopBlockCracking (clear it). EventData on start is the per-tick
// crack increment (65535 = fully cracked); on stop it is 0. The crack LevelEvent
// addresses the block by its corner position, not its center.
func (s *Server) broadcastBlockCracking(pos protocol.BlockPos, eventType int32) {
	corner := mgl32.Vec3{float32(pos[0]), float32(pos[1]), float32(pos[2])}
	var data int32
	if eventType == packet.LevelEventStartBlockCracking {
		data = int32(65535 / (bedrockCrackBreakSeconds * 20))
	}
	s.forEachSession(func(bs *bedrockSession) {
		bs.write(&packet.LevelEvent{
			EventType: eventType,
			Position:  corner,
			EventData: data,
		})
	})
}

func isBedrockBreakAction(action int32) bool {
	switch action {
	case protocol.PlayerActionStopBreak, protocol.PlayerActionPredictDestroyBlock, protocol.PlayerActionCreativePlayerDestroyBlock:
		return true
	default:
		return false
	}
}
