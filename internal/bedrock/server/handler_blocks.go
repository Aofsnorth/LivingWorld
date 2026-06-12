package server

import (
	"strings"

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

// bedrockCrackBreakSeconds is the legacy fixed mining duration; new code
// should compute a per-block duration with bedrockCrackSecondsFor and pass it
// through to the crack/broadcast helpers. It is kept as the fallback used by
// the (very narrow) code paths that don't have a block id handy, and as the
// default in the effect-bus publishing helper.
const bedrockCrackBreakSeconds = gameplay.BedrockCrackBreakSeconds

// bedrockCrackSecondsFor returns the per-block crack overlay duration in
// seconds for the given block state. The Bedrock client computes its own
// break time locally (using its knowledge of the held tool, mining fatigue,
// haste, in-water, etc.), so the server's only job is to pace the *Java*
// crack overlay in roughly the right ballpark. Using vanilla's hand-mining
// time (hardness * 1.5) is a serviceable tool-agnostic approximation; the
// crack is cleared on actual break-finish (block turns to air) or on abort,
// so a slightly off rate is harmless. -1 hardness (unbreakable) returns 0
// which disables the overlay entirely.
func bedrockCrackSecondsFor(blockID int32) float64 {
	h := world.Hardness(blockID)
	if h <= 0 {
		return 0
	}
	return h * 1.5
}

// crackSwitch updates the tracked breaking block for bs and, when the player
// switched from a different block, clears the crack overlay on the old one.
// totalSeconds is the per-block expected break duration (see
// bedrockCrackSecondsFor) captured into CrackState so the periodic Java
// overlay update reads the right rate for this specific block+tool combo.
// Returns whether a switch happened (the caller then starts the new block's
// crack). This is what stops a held-break crack from sticking on the previous
// block when the crosshair slides to a new one.
func (s *Server) crackSwitch(bs *bedrockSession, pos protocol.BlockPos, totalSeconds float64) bool {
	hadPrev, px, py, pz := s.wm.CrackManager().StartBreaking(bs.id, int(pos[0]), int(pos[1]), int(pos[2]), totalSeconds)
	if hadPrev {
		s.broadcastBlockCracking(protocol.BlockPos{int32(px), int32(py), int32(pz)}, packet.LevelEventStopBlockCracking, 0)
		s.wm.PublishCrack(world.BlockUpdateSourceBedrock, bs.id, px, py, pz, -1) // clear overlay on Java too
	}
	return hadPrev
}

// broadcastBlockCracking drives the progressive crack overlay on every Bedrock
// viewer. eventType is LevelEventStartBlockCracking (begin animating) or
// LevelEventStopBlockCracking (clear it). EventData on start is the per-tick
// crack increment (65535 = fully cracked); on stop it is 0. totalSeconds is
// only consulted for the Start event — the per-tick increment is
// 65535 / (totalSeconds * 20). The crack LevelEvent addresses the block by
// its corner position, not its center.
func (s *Server) broadcastBlockCracking(pos protocol.BlockPos, eventType int32, totalSeconds float64) {
	corner := mgl32.Vec3{float32(pos[0]), float32(pos[1]), float32(pos[2])}
	var data int32
	if eventType == packet.LevelEventStartBlockCracking {
		secs := totalSeconds
		if secs <= 0 {
			secs = bedrockCrackBreakSeconds
		}
		data = int32(65535 / (secs * 20))
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

// placeSoundForBlockName returns the vanilla Bedrock step-SFX for the placed
// block. The mapping is intentionally coarse — it's keyed off the block's
// material family so grass/dirt sound like grass, stone-family sounds like
// stone, wood sounds like wood, etc. Anything we don't recognise falls back
// to `step.stone`, which is the same default vanilla uses for un-categorised
// place sounds.
func placeSoundForBlockName(name string) string {
	// Strip the namespace; the lookup is by base name only.
	if i := strings.Index(name, ":"); i >= 0 {
		name = name[i+1:]
	}
	switch {
	case strings.HasSuffix(name, "_log"), strings.HasSuffix(name, "_wood"),
		strings.HasSuffix(name, "_planks"), strings.HasSuffix(name, "_slab") && (strings.Contains(name, "wood") || strings.Contains(name, "plank")),
		strings.Contains(name, "fence"),
		strings.HasSuffix(name, "_stairs") && (strings.Contains(name, "oak") || strings.Contains(name, "spruce") || strings.Contains(name, "birch") || strings.Contains(name, "jungle") || strings.Contains(name, "acacia") || strings.Contains(name, "dark_oak") || strings.Contains(name, "mangrove") || strings.Contains(name, "cherry") || strings.Contains(name, "bamboo")),
		name == "crafting_table" || name == "bookshelf" || name == "chest" || name == "barrel" || name == "lectern" || name == "loom" || name == "smoker" || name == "furnace" || name == "blast_furnace" || name == "composter":
		return "step.wood"
	case name == "grass_block" || name == "dirt" || name == "coarse_dirt" || name == "podzol" || name == "mycelium" || name == "rooted_dirt" || name == "farmland" || name == "mud" || name == "muddy_mangrove_roots" || name == "moss_block" || name == "moss_carpet":
		return "step.grass"
	case name == "sand" || name == "red_sand" || name == "gravel" || name == "soul_sand" || name == "soul_soil":
		return "step.sand"
	case name == "snow" || name == "snow_block" || name == "powder_snow":
		return "step.snow"
	case strings.HasSuffix(name, "_wool"):
		return "step.cloth"
	case name == "glass" || strings.HasSuffix(name, "_glass") || strings.HasSuffix(name, "_glass_pane"):
		return "random.glass"
	case name == "ice" || name == "packed_ice" || name == "blue_ice" || name == "frosted_ice":
		return "step.ice"
	default:
		return "step.stone"
	}
}

// playBlockPlaceSound sends a PlaySound packet at the placement position
// with the vanilla per-material step SFX. We send it on the placer's
// connection only — the placer's own client is the one that "should have"
// played the sound via its own prediction; foreign viewers don't need a
// second place sound on top of whatever they already heard client-side.
func (s *Server) playBlockPlaceSound(conn *minecraft.Conn, itemName string, pos protocol.BlockPos) {
	sound := placeSoundForBlockName(itemName)
	center := mgl32.Vec3{float32(pos[0]) + 0.5, float32(pos[1]) + 0.5, float32(pos[2]) + 0.5}
	_ = conn.WritePacket(&packet.PlaySound{
		SoundName: sound,
		Position:  center,
		Volume:    1.0,
		Pitch:     0.79,
	})
}
