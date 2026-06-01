package server

import (
	javaworld "livingworld/internal/java/world"
	"livingworld/internal/world"
	"log"
	"runtime"
	"sort"
	"sync"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// debugChunkf logs a chunk-streaming diagnostic line when the --debug-chunks
// flag is set. Gated and permanent: safe to leave the call sites in place.
func (s *PlayerSession) debugChunkf(format string, args ...any) {
	if s.Bridge != nil && s.Bridge.cfg != nil && s.Bridge.cfg.Java.DebugChunks {
		log.Printf("[Java][chunk] "+format, args...)
	}
}

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
		s.debugChunkf("boundary cross: old(%d,%d) -> new(%d,%d)", oldCX, oldCZ, newCX, newCZ)
		s.updateChunks()
		// AOI: this viewer moved, so re-evaluate which foreign players are now in /
		// out of range. Enqueued so it stays ordered with the other relays.
		s.enqueue(func() { s.reconcileViewers() })
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

	s.debugChunkf("update: center(%d,%d) radius=%d", cx, cz, radius)

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

	forgotten := 0
	for _, pos := range loadedList {
		dx := pos.X - int(cx)
		dz := pos.Z - int(cz)
		if dx < -int(radius) || dx > int(radius) || dz < -int(radius) || dz > int(radius) {
			forgotten++
			s.debugChunkf("forget: (%d,%d)", pos.X, pos.Z)
			// ForgetLevelChunk uses packed-long [Z][X] encoding, NOT the [X,Z]
			// pair used by LevelChunkWithLight — see BuildForgetLevelChunkPacket.
			_ = s.SendPacket(javaworld.BuildForgetLevelChunkPacket(int32(pos.X), int32(pos.Z)))
		}
	}
	s.debugChunkf("forget total: %d", forgotten)

	// Collect the in-range chunks that still need sending. newChunks tracks the
	// full in-range set (for the LoadedChunks diff); toSend holds only the missing
	// ones so we can order them. finalLoaded starts with the in-range chunks that
	// were already loaded (not forgotten this pass); successfully-sent chunks are
	// added below. Chunks that fail to send are left out so the next
	// boundary-cross retries them instead of leaving a permanent hole.
	finalLoaded := make(map[world.ChunkPos]bool)
	var toSend []world.ChunkPos
	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			pos := world.ChunkPos{X: int(cx + dx), Z: int(cz + dz)}

			s.mu.Lock()
			isLoaded := s.LoadedChunks[pos]
			s.mu.Unlock()
			if isLoaded {
				finalLoaded[pos] = true
			} else {
				toSend = append(toSend, pos)
			}
		}
	}

	// Send nearest-first so the world builds outward from the player like vanilla.
	// The previous raster order made chunks pop in from one corner ("weird").
	sort.Slice(toSend, func(i, j int) bool {
		return chunkDistSq(toSend[i], cx, cz) < chunkDistSq(toSend[j], cx, cz)
	})

	// Build the chunk packets concurrently (C2ME-style: chunk generation +
	// serialization is the CPU-heavy part, so fan it out across cores), then send
	// them in nearest-first order. world.LoadChunk is mutex-guarded, so parallel
	// loads/generation are safe.
	packets := make([]pk.Packet, len(toSend))
	w := s.Bridge.wm.GetDefaultWorld()
	var wg sync.WaitGroup
	sem := make(chan struct{}, chunkBuildWorkers)
	for i := range toSend {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			pos := toSend[i]
			lChunk := javaworld.ConvertToLevelChunk(w.LoadChunk(pos.X, pos.Z))
			packets[i] = javaworld.BuildLevelChunkWithLightPacket(int32(pos.X), int32(pos.Z), lChunk)
		}(i)
	}
	wg.Wait()

	// sendChunk sends one chunk packet and marks it loaded only on success so a
	// failed send is retried on the next boundary-cross.
	var sentOK, sentErr int
	sendChunk := func(i int) {
		if err := s.SendPacket(packets[i]); err != nil {
			sentErr++
			s.debugChunkf("send error (%d,%d): %v", toSend[i].X, toSend[i].Z, err)
			return
		}
		finalLoaded[toSend[i]] = true
		sentOK++
	}

	if useBatch && len(packets) > 0 {
		_ = s.SendPacket(pk.Marshal(packetid.ClientboundGameChunkBatchStart))
		for i := range packets {
			sendChunk(i)
		}
		_ = s.SendPacket(pk.Marshal(packetid.ClientboundGameChunkBatchFinished, pk.VarInt(int32(len(packets)))))
	} else {
		for i := range packets {
			sendChunk(i)
		}
	}
	s.debugChunkf("send total: ok=%d err=%d (of %d)", sentOK, sentErr, len(packets))

	s.mu.Lock()
	s.LoadedChunks = finalLoaded
	s.mu.Unlock()
}

// chunkBuildWorkers bounds how many chunks are generated + serialized in
// parallel per batch (C2ME-style parallel chunk I/O). It scales with CPU cores.
var chunkBuildWorkers = max(2, runtime.NumCPU())

// chunkDistSq is the squared chunk distance from pos to the player's chunk
// (cx,cz), used to send chunks nearest-first.
func chunkDistSq(pos world.ChunkPos, cx, cz int32) int {
	dx := pos.X - int(cx)
	dz := pos.Z - int(cz)
	return dx*dx + dz*dz
}
