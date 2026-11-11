package server

import (
	javaworld "livingworld/internal/java/world"
	"livingworld/internal/world"
	"log"
	"runtime"
	"sort"
	"sync"
	"time"

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
	var flags pk.Byte
	if err := p.Scan(&x, &y, &z, &flags); err != nil {
		return
	}
	onGround := (flags & 0x01) != 0
	s.applyMove(float64(x), float64(y), float64(z), onGround)
}

func (s *PlayerSession) HandleMovePosRot(p pk.Packet) {
	var x, y, z pk.Double
	var yaw, pitch pk.Float
	var flags pk.Byte
	if err := p.Scan(&x, &y, &z, &yaw, &pitch, &flags); err != nil {
		return
	}
	onGround := (flags & 0x01) != 0
	s.mu.Lock()
	s.Yaw, s.Pitch = float32(yaw), float32(pitch)
	s.mu.Unlock()
	s.applyMove(float64(x), float64(y), float64(z), onGround)
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
	var flags pk.Byte
	if err := p.Scan(&yaw, &pitch, &flags); err != nil {
		return
	}
	onGround := (flags & 0x01) != 0
	s.mu.Lock()
	s.Yaw, s.Pitch = float32(yaw), float32(pitch)
	s.OnGround = onGround
	x, y, z := s.X, s.Y, s.Z
	s.mu.Unlock()
	if s.Bridge != nil {
		if s.Bridge.pm != nil {
			s.Bridge.pm.UpdatePosition(s.UUID(), x, y, z, s.Pitch, s.Yaw, onGround)
		}
	}
	s.trackFall(y, y, onGround)
}

func (s *PlayerSession) HandleMoveStatusOnly(p pk.Packet) {
	var flags pk.Byte
	if err := p.Scan(&flags); err != nil {
		return
	}
	onGround := (flags & 0x01) != 0
	s.mu.Lock()
	s.OnGround = onGround
	y := s.Y
	s.mu.Unlock()
	s.trackFall(y, y, onGround)
}

func (s *PlayerSession) HandlePlayerCommand(p pk.Packet) {
	var entityID pk.VarInt
	var actionID pk.VarInt
	var jumpBoost pk.VarInt
	if err := p.Scan(&entityID, &actionID, &jumpBoost); err != nil {
		return
	}

	// In Minecraft 1.21.2+ / 26.3 (protocol 777), ServerboundPlayerCommand
	// action IDs are: 0 = STOP_SLEEPING, 1 = START_SPRINTING, 2 = STOP_SPRINTING.
	// Sneaking is sent via ServerboundGamePlayerInput (bit 0x20).
	switch actionID {
	case 1: // Start sprinting
		s.Bridge.pm.UpdateSprinting(s.UUID(), true)
	case 2: // Stop sprinting
		s.Bridge.pm.UpdateSprinting(s.UUID(), false)
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

	// Record the pruned loaded set first, then progressively add chunks as each
	// micro-batch is generated and sent. Streaming in progressive micro-batches
	// (3x3 immediate spawn ground first, then 16-chunk rings outward with a brief
	// yield) prevents spiking CPU at 100% or stalling login on 441 chunks at once.
	s.mu.Lock()
	s.LoadedChunks = finalLoaded
	s.mu.Unlock()

	if len(toSend) == 0 {
		return
	}

	w := s.Bridge.wm.GetDefaultWorld()
	var sentOK, sentErr int

	for start := 0; start < len(toSend); {
		step := 16
		if start == 0 {
			step = 9 // 3x3 immediate area around player first
		}
		end := min(start+step, len(toSend))
		batchPos := toSend[start:end]
		start = end

		loadedChunks := w.LoadChunksBatch(batchPos, chunkBuildWorkers)
		packets := make([]pk.Packet, len(batchPos))
		var wg sync.WaitGroup
		sem := make(chan struct{}, chunkBuildWorkers)
		for i := range batchPos {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int) {
				defer wg.Done()
				defer func() { <-sem }()
				pos := batchPos[i]
				lChunk := javaworld.ConvertToLevelChunk(loadedChunks[i])
				packets[i] = javaworld.BuildLevelChunkWithLightPacket(int32(pos.X), int32(pos.Z), lChunk)
			}(i)
		}
		wg.Wait()

		sendChunk := func(i int) {
			if err := s.SendPacket(packets[i]); err != nil {
				sentErr++
				s.debugChunkf("send error (%d,%d): %v", batchPos[i].X, batchPos[i].Z, err)
				return
			}
			s.mu.Lock()
			s.LoadedChunks[batchPos[i]] = true
			s.mu.Unlock()
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

		if start < len(toSend) {
			// Check if player moved significantly; if so, let the next queued update take over.
			s.mu.Lock()
			curCX, curCZ := s.ChunkX(), s.ChunkZ()
			s.mu.Unlock()
			dx := curCX - cx
			dz := curCZ - cz
			if dx < -2 || dx > 2 || dz < -2 || dz > 2 {
				break
			}
			time.Sleep(15 * time.Millisecond)
		}
	}
	s.debugChunkf("send total: ok=%d err=%d (of %d)", sentOK, sentErr, len(toSend))
}

// chunkBuildWorkers bounds how many chunks are generated + serialized in
// parallel per batch (C2ME-style parallel chunk I/O). Capped to leave CPU headroom.
var chunkBuildWorkers = min(max(runtime.NumCPU()/2, 2), 4)

// chunkDistSq is the squared chunk distance from pos to the player's chunk
// (cx,cz), used to send chunks nearest-first.
func chunkDistSq(pos world.ChunkPos, cx, cz int32) int {
	dx := pos.X - int(cx)
	dz := pos.Z - int(cz)
	return dx*dx + dz*dz
}
