package server

import (
	javaworld "livingworld/internal/java/world"
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

func (s *PlayerSession) HandleMovePos(p pk.Packet) {
	var x, y, z pk.Double
	var onGround pk.Boolean
	if err := p.Scan(&x, &y, &z, &onGround); err != nil {
		return
	}
	s.applyMove(float64(x), float64(y), float64(z), bool(onGround))
}

func (s *PlayerSession) HandleMovePosRot(p pk.Packet) {
	var x, y, z pk.Double
	var yaw, pitch pk.Float
	var onGround pk.Boolean
	if err := p.Scan(&x, &y, &z, &yaw, &pitch, &onGround); err != nil {
		return
	}
	s.mu.Lock()
	s.Yaw, s.Pitch = float32(yaw), float32(pitch)
	s.mu.Unlock()
	s.applyMove(float64(x), float64(y), float64(z), bool(onGround))
}

// applyMove updates position from a movement packet, tracks fall damage, and
// reloads chunks when the player crosses a chunk boundary.
func (s *PlayerSession) applyMove(x, y, z float64, onGround bool) {
	s.mu.Lock()
	oldCX, oldCZ := s.ChunkX(), s.ChunkZ()
	oldY := s.Y
	s.X, s.Y, s.Z = x, y, z
	s.OnGround = onGround
	newCX, newCZ := s.ChunkX(), s.ChunkZ()
	s.mu.Unlock()

	// Publish Java movement to the shared player manager so Bedrock viewers
	// can see Java players move.
	if s.Bridge != nil && s.Bridge.pm != nil {
		s.Bridge.pm.UpdatePosition(s.UUID(), x, y, z, s.Pitch, s.Yaw, onGround)
	}

	s.trackFall(oldY, y, onGround)
	if oldCX != newCX || oldCZ != newCZ {
		s.updateChunks()
	}
}

func (s *PlayerSession) HandleMoveRot(p pk.Packet) {
	var yaw, pitch pk.Float
	var onGround pk.Boolean
	if err := p.Scan(&yaw, &pitch, &onGround); err != nil {
		return
	}
	s.mu.Lock()
	s.Yaw, s.Pitch = float32(yaw), float32(pitch)
	s.OnGround = bool(onGround)
	x, y, z := s.X, s.Y, s.Z
	s.mu.Unlock()
	if s.Bridge != nil && s.Bridge.pm != nil {
		s.Bridge.pm.UpdatePosition(s.UUID(), x, y, z, s.Pitch, s.Yaw, bool(onGround))
	}
}

func (s *PlayerSession) HandleMoveStatusOnly(p pk.Packet) {
	var onGround pk.Boolean
	if err := p.Scan(&onGround); err != nil {
		return
	}
	s.mu.Lock()
	s.OnGround = bool(onGround)
	s.mu.Unlock()
}

func (s *PlayerSession) HandlePlayerCommand(p pk.Packet) {
	var entityID pk.VarInt
	var actionID pk.VarInt
	var jumpBoost pk.VarInt
	if err := p.Scan(&entityID, &actionID, &jumpBoost); err != nil {
		return
	}

	switch actionID {
	case 0: // Start sneaking
		s.Bridge.pm.UpdateSneak(s.UUID(), true)
	case 1: // Stop sneaking
		s.Bridge.pm.UpdateSneak(s.UUID(), false)
	}
}

func (s *PlayerSession) updateChunks() {
	select {
	case s.chunkQueue <- struct{}{}:
	default:
		// An update is already pending, no need to queue another
	}
}

func (s *PlayerSession) updateChunksWithBatch(useBatch bool) {
	s.mu.Lock()
	cx := s.ChunkX()
	cz := s.ChunkZ()
	s.mu.Unlock()

	radius := int32(s.Bridge.cfg.Java.ViewDistance)
	newChunks := make(map[world.ChunkPos]bool)

	// Send chunk cache center first so client knows where to expect chunks
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameSetChunkCacheCenter,
		pk.VarInt(cx), pk.VarInt(cz),
	))

	// Unload old chunks
	s.mu.Lock()
	loadedList := make([]world.ChunkPos, 0, len(s.LoadedChunks))
	for pos := range s.LoadedChunks {
		loadedList = append(loadedList, pos)
	}
	s.mu.Unlock()

	for _, pos := range loadedList {
		dx := pos.X - int(cx)
		dz := pos.Z - int(cz)
		if dx < -int(radius) || dx > int(radius) || dz < -int(radius) || dz > int(radius) {
			_ = s.SendPacket(pk.Marshal(
				packetid.ClientboundGameForgetLevelChunk,
				pk.Int(pos.X), pk.Int(pos.Z),
			))
		}
	}

	// Start chunk batch
	newChunkCount := int32(0)
	if useBatch {
		_ = s.SendPacket(pk.Marshal(packetid.ClientboundGameChunkBatchStart))
	}

	// Send chunk data
	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			pos := world.ChunkPos{X: int(cx + dx), Z: int(cz + dz)}
			newChunks[pos] = true

			s.mu.Lock()
			isLoaded := s.LoadedChunks[pos]
			s.mu.Unlock()

			if !isLoaded {
				wChunk := s.Bridge.wm.GetDefaultWorld().LoadChunk(pos.X, pos.Z)
				lChunk := javaworld.ConvertToLevelChunk(wChunk)
				packet := javaworld.BuildLevelChunkWithLightPacket(int32(pos.X), int32(pos.Z), lChunk)
				_ = s.SendPacket(packet)
				newChunkCount++
			}
		}
	}

	// End chunk batch
	if useBatch && newChunkCount > 0 {
		_ = s.SendPacket(pk.Marshal(
			packetid.ClientboundGameChunkBatchFinished,
			pk.VarInt(newChunkCount),
		))
	}

	s.mu.Lock()
	s.LoadedChunks = newChunks
	s.mu.Unlock()
}
