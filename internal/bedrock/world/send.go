package world

import (
	"log"

	lwworld "livingworld/internal/world"

	dfworld "github.com/df-mc/dragonfly/server/world"
	dfbiome "github.com/df-mc/dragonfly/server/world/biome"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// SendChunk loads, converts, caches, and sends a single chunk to the Bedrock player.
func (c *ChunkConverter) SendChunk(
	conn *minecraft.Conn, w *lwworld.World, cx, cz int,
	chunkCache *ChunkCache,
) {
	rng := dfworld.Overworld.Range()
	plainsBiomeID := uint32(dfbiome.Plains{}.EncodeBiome())
	maxY := int(rng.Max()) // 319; world blocks live at Y >= 0

	pos := protocol.ChunkPos{int32(cx), int32(cz)}

	chunkCache.Mu.Lock()
	ch, ok := chunkCache.Cache[pos]
	chunkCache.Mu.Unlock()

	if !ok {
		wchunk := w.LoadChunk(cx, cz)
		ch = dfchunk.New(dfworld.DefaultBlockRegistry, rng)

		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				// Canonical Y: scan the full -64..319 column (dragonfly's chunk is
				// already -64-based, so world-Y passes straight to ch.SetBlock).
				for y := int(rng.Min()); y <= maxY; y++ {
					id := wchunk.GetBlock(x, y, z).ID()
					if id == 0 {
						continue
					}
					rid, found := LivingWorldBlockIDToBedrockRID(id)
					if !found {
						continue
					}
					ch.SetBlock(uint8(x), int16(y), uint8(z), 0, rid)
				}
			}
		}
		for y := int16(rng.Min()); y <= int16(rng.Max()); y += 4 {
			for x := uint8(0); x < 16; x += 4 {
				for z := uint8(0); z < 16; z += 4 {
					ch.SetBiome(x, y, z, plainsBiomeID)
				}
			}
		}

		chunkCache.Mu.Lock()
		chunkCache.Cache[pos] = ch
		chunkCache.Mu.Unlock()
	}

	// Modern Bedrock (1.18.10+) expects LevelChunk to use the sub-chunk request
	// system: the inline payload carries only biomes + the border-block count
	// (1 byte), and the client then sends a SubChunkRequest for each sub-chunk
	// it actually needs. Sending the full payload inline with a SubChunkCount
	// of (rng.Height()>>4)+1 = 24 desyncs the client's heightmap against the
	// world — the renderer then masks terrain into thin slabs around the
	// heightmap surface (the "floating islands" rendering). Dragonfly does the
	// same dance (see session.chunk.sendNetworkChunk) by sending an empty
	// sub-chunk count with a SubChunkLimit, then answering each SubChunkRequest
	// via the sub-chunk entry path.
	chunkCache.Mu.RLock()
	biomes := dfchunk.EncodeBiomes(ch, dfchunk.NetworkEncoding)
	highest := ch.HighestFilledSubChunk()
	chunkCache.Mu.RUnlock()

	// Sub-chunk request mode: an empty sub-chunk count plus an explicit
	// SubChunkLimit is how a server asks the client to pull sub-chunks on
	// demand. The inline payload then carries only biomes and the border
	// block count, and each requested sub-chunk is answered through the
	// SubChunkRequest path below.
	payload := append(biomes, 0) // trailing border block count = 0

	err := conn.WritePacket(&packet.LevelChunk{
		Position:        pos,
		Dimension:       0,
		SubChunkCount:   protocol.SubChunkRequestModeLimited,
		HighestSubChunk: highest,
		CacheEnabled:    false,
		RawPayload:      payload,
	})
	if err != nil {
		log.Printf("[Bedrock] Failed to send chunk (%d,%d): %v", cx, cz, err)
	}
}

// SendInitialChunks converts the shared world's chunks (generated terrain plus
// any persisted player edits) into Bedrock LevelChunk packets and sends them
// inline. Reading the real world is what makes block edits and persistence show
// up on Bedrock the same as on Java. chunkCache is populated so the SubChunkRequest
// path can serve the same data.
func (c *ChunkConverter) SendInitialChunks(
	conn *minecraft.Conn, w *lwworld.World, centerChunkX, centerChunkZ, radius int,
	chunkCache *ChunkCache,
) {
	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			cx, cz := centerChunkX+dx, centerChunkZ+dz
			c.SendChunk(conn, w, cx, cz, chunkCache)
		}
	}

	log.Printf("[Bedrock] Sent %d chunks (inline from world)", (2*radius+1)*(2*radius+1))
}

// HandleSubChunkRequest processes SubChunkRequest packets for the modern
// sub-chunk request path. It converts world chunks on demand if not already cached.
func (c *ChunkConverter) HandleSubChunkRequest(
	conn *minecraft.Conn, pk *packet.SubChunkRequest, w *lwworld.World,
	chunkCache *ChunkCache,
) {
	rng := dfworld.Overworld.Range()
	center := pk.Position
	plainsBiomeID := uint32(dfbiome.Plains{}.EncodeBiome())
	maxY := int(rng.Max())

	entries := make([]protocol.SubChunkEntry, 0, len(pk.Offsets))
	for _, offset := range pk.Offsets {
		absX := center.X() + int32(offset[0])
		absZ := center.Z() + int32(offset[2])
		absYInd := center.Y() + int32(offset[1])

		arrayYInd := absYInd - int32(rng.Min()>>4)
		subChunkCount := int32((rng.Height() >> 4) + 1)

		if arrayYInd < 0 || arrayYInd >= subChunkCount {
			entries = append(entries, protocol.SubChunkEntry{
				Offset: offset,
				Result: protocol.SubChunkResultIndexOutOfBounds,
			})
			continue
		}

		chunkPos := protocol.ChunkPos{absX, absZ}
		// Fast path: chunk already cached → encode under the chunk body lock.
		chunkCache.Mu.RLock()
		_, cached := chunkCache.Cache[chunkPos]
		chunkCache.Mu.RUnlock()
		if cached {
			var entry protocol.SubChunkEntry
			ok2 := false
			chunkCache.WithChunkBody(chunkPos, func(ch *dfchunk.Chunk) {
				entry, ok2 = c.buildSubChunkEntry(ch, offset, int16(arrayYInd))
			})
			if ok2 {
				entries = append(entries, entry)
				continue
			}
			// Fall through to the build path (entry not produced — chunk removed).
		}

		if !cached {
			wchunk := w.LoadChunk(int(absX), int(absZ))
			newCh := dfchunk.New(dfworld.DefaultBlockRegistry, rng)
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					// Canonical Y: scan the full -64..319 column (see SendChunk).
					for y := int(rng.Min()); y <= maxY; y++ {
						id := wchunk.GetBlock(x, y, z).ID()
						if id == 0 {
							continue
						}
						rid, found := LivingWorldBlockIDToBedrockRID(id)
						if !found {
							continue
						}
						newCh.SetBlock(uint8(x), int16(y), uint8(z), 0, rid)
					}
				}
			}
			for y := int16(rng.Min()); y <= int16(rng.Max()); y += 4 {
				for x := uint8(0); x < 16; x += 4 {
					for z := uint8(0); z < 16; z += 4 {
						newCh.SetBiome(x, y, z, plainsBiomeID)
					}
				}
			}
			chunkCache.Mu.Lock()
			chunkCache.Cache[chunkPos] = newCh
			chunkCache.Mu.Unlock()
		}

		var entry protocol.SubChunkEntry
		ok2 := false
		chunkCache.WithChunkBody(chunkPos, func(ch *dfchunk.Chunk) {
			entry, ok2 = c.buildSubChunkEntry(ch, offset, int16(arrayYInd))
		})
		if ok2 {
			entries = append(entries, entry)
		}
	}

	_ = conn.WritePacket(&packet.SubChunk{
		CacheEnabled:    false,
		Dimension:       pk.Dimension,
		Position:        center,
		SubChunkEntries: entries,
	})
}

// buildSubChunkEntry encodes one sub-chunk entry (blocks + heightmap) from a
// cached dfchunk. Caller must hold the chunk body lock (WithChunkBody).
func (c *ChunkConverter) buildSubChunkEntry(
	ch *dfchunk.Chunk, offset protocol.SubChunkOffset, arrayYInd int16,
) (protocol.SubChunkEntry, bool) {
	if arrayYInd < 0 || arrayYInd >= int16(len(ch.Sub())) {
		return protocol.SubChunkEntry{Offset: offset, Result: protocol.SubChunkResultIndexOutOfBounds}, true
	}
	sub := ch.Sub()[arrayYInd]
	if sub.Empty() {
		// Match dragonfly: an all-air entry still carries the heightmap
		// relation of this sub-chunk to the terrain surface, so the client's
		// renderer doesn't have to guess.
		hmType, hmData, _ := subChunkHeightMap(ch, arrayYInd)
		return protocol.SubChunkEntry{
			Offset:              offset,
			Result:              protocol.SubChunkResultSuccessAllAir,
			HeightMapType:       hmType,
			HeightMapData:       hmData,
			RenderHeightMapType: hmType,
			RenderHeightMapData: hmData,
		}, true
	}
	rawData := dfchunk.EncodeSubChunk(ch, dfchunk.NetworkEncoding, int(arrayYInd))
	hmType, hmData, _ := subChunkHeightMap(ch, arrayYInd)
	return protocol.SubChunkEntry{
		Offset:              offset,
		Result:              protocol.SubChunkResultSuccess,
		RawPayload:          rawData,
		HeightMapType:       hmType,
		HeightMapData:       hmData,
		RenderHeightMapType: hmType,
		RenderHeightMapData: hmData,
	}, true
}

// subChunkHeightMap mirrors dragonfly's session.subChunkEntry heightmap logic:
// for the sub-chunk at index ind it classifies every column of the chunk's
// heightmap relative to that sub-chunk (above / below / inside it) and only
// returns data when the surface actually touches or bounds this sub-chunk.
// The classifier matches dragonfly: columns whose surface sub-chunk is above
// get 16 ("all below terrain"), below get -1 ("all air"), and inside get the
// surface Y relative to this sub-chunk's base. Sub-chunk indices are computed
// relative to the world's minimum Y (dragonfly col.SubIndex(y) = (y-r[0])>>4).
func subChunkHeightMap(ch *dfchunk.Chunk, ind int16) (byte, []int8, bool) {
	hm := ch.HeightMap()
	higher, lower := true, true
	subMap := make([]int8, 256)
	minInd := int16(dfworld.Overworld.Range().Min() >> 4)
	for z := uint8(0); z < 16; z++ {
		for x := uint8(0); x < 16; x++ {
			y := hm.At(x, z)
			otherInd := (y >> 4) - minInd
			switch {
			case otherInd > ind:
				subMap[z*16+x], lower = 16, false
			case otherInd < ind:
				subMap[z*16+x], higher = -1, false
			default:
				subMap[z*16+x], lower, higher = int8(y&15), false, false
			}
		}
	}
	subMapType := byte(protocol.HeightMapDataHasData)
	switch {
	case higher:
		subMapType = protocol.HeightMapDataTooHigh
		subMap = nil
	case lower:
		subMapType = protocol.HeightMapDataTooLow
		subMap = nil
	}
	return subMapType, subMap, subMapType == protocol.HeightMapDataHasData
}
