package world

import (
	"log"
	"runtime"
	"sync"

	"livingworld/internal/dimension/overworld"
	lwworld "livingworld/internal/world"

	dfworld "github.com/df-mc/dragonfly/server/world"
	dfbiome "github.com/df-mc/dragonfly/server/world/biome"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// convertWorldChunkToDragonfly converts a canonical *lwworld.Chunk into a
// Dragonfly *dfchunk.Chunk, skipping empty sections in O(1) and caching the
// most recent block state → Bedrock runtime ID lookup in a register.
func convertWorldChunkToDragonfly(wchunk *lwworld.Chunk, plainsBiomeID uint32) *dfchunk.Chunk {
	rng := dfworld.Overworld.Range()
	ch := dfchunk.New(dfworld.DefaultBlockRegistry, rng)

	var lastID int32 = -1
	var lastRID uint32
	var lastFound bool
	var lastWaterlogged bool
	waterRID := BedrockStillWaterRID()

	for secIdx := 0; secIdx < lwworld.SectionsPerChunk; secIdx++ {
		if wchunk.SectionIsEmpty(secIdx) {
			continue
		}
		blocks := wchunk.SectionBlocks(secIdx)
		baseY := int16(lwworld.MinWorldHeight + secIdx*16)
		for ly := 0; ly < 16; ly++ {
			y := baseY + int16(ly)
			yBits := ly << 8
			for z := 0; z < 16; z++ {
				yzBits := yBits | (z << 4)
				for x := 0; x < 16; x++ {
					id := blocks[yzBits|x]
					if id == 0 {
						continue
					}
					if id != lastID {
						lastID = id
						lastRID, lastFound = LivingWorldBlockIDToBedrockRID(id)
						lastWaterlogged = IsWaterloggedLivingWorldBlock(id)
					}
					if !lastFound {
						continue
					}
					ch.SetBlock(uint8(x), y, uint8(z), 0, lastRID)
					if lastWaterlogged {
						ch.SetBlock(uint8(x), y, uint8(z), 1, waterRID)
					}
				}
			}
		}
	}

	lastBiomeID := int32(-1)
	lastBedrockBiome := plainsBiomeID
	for secIdx := 0; secIdx < lwworld.SectionsPerChunk; secIdx++ {
		baseY := int16(lwworld.MinWorldHeight + secIdx*16)
		if secBiomes := wchunk.SectionBiomes(secIdx); secBiomes != nil {
			for i := 0; i < 64; i++ {
				bID := secBiomes[i]
				if bID != lastBiomeID {
					lastBiomeID = bID
					lastBedrockBiome = overworld.LookupBedrockBiomeID(lwworld.BiomeName(bID))
				}
				bx := uint8((i % 4) * 4)
				by := baseY + int16((i/16)*4)
				bz := uint8(((i / 4) % 4) * 4)
				for dx := uint8(0); dx < 4; dx++ {
					for dy := int16(0); dy < 4; dy++ {
						for dz := uint8(0); dz < 4; dz++ {
							ch.SetBiome(bx+dx, by+dy, bz+dz, lastBedrockBiome)
						}
					}
				}
			}
		} else {
			for dy := int16(0); dy < 16; dy += 4 {
				for x := uint8(0); x < 16; x += 4 {
					for z := uint8(0); z < 16; z += 4 {
						ch.SetBiome(x, baseY+dy, z, plainsBiomeID)
					}
				}
			}
		}
	}
	ch.Compact()
	return ch
}

// PreloadChunks loads and converts any uncached chunks in coords in parallel
// across CPU cores so subsequent SendChunk calls hit chunkCache without blocking.
func (c *ChunkConverter) PreloadChunks(
	w *lwworld.World, coords []protocol.ChunkPos, chunkCache *ChunkCache,
) {
	if len(coords) == 0 {
		return
	}
	var missingPos []protocol.ChunkPos
	var missingLW []lwworld.ChunkPos

	chunkCache.Mu.RLock()
	for _, pos := range coords {
		if _, ok := chunkCache.Cache[pos]; !ok {
			missingPos = append(missingPos, pos)
			missingLW = append(missingLW, lwworld.ChunkPos{X: int(pos.X()), Z: int(pos.Z())})
		}
	}
	chunkCache.Mu.RUnlock()

	if len(missingPos) == 0 {
		return
	}

	workers := max(2, runtime.NumCPU())
	wchunks := w.LoadChunksBatch(missingLW, workers)

	plainsBiomeID := uint32(dfbiome.Plains{}.EncodeBiome())
	dfChunks := make([]*dfchunk.Chunk, len(missingPos))

	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for i := range missingPos {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			dfChunks[i] = convertWorldChunkToDragonfly(wchunks[i], plainsBiomeID)
		}(i)
	}
	wg.Wait()

	chunkCache.Mu.Lock()
	for i, pos := range missingPos {
		if _, exists := chunkCache.Cache[pos]; !exists {
			chunkCache.Cache[pos] = dfChunks[i]
		}
	}
	chunkCache.Mu.Unlock()
}

// SendChunk loads, converts, caches, and sends a single chunk to the Bedrock player.
func (c *ChunkConverter) SendChunk(
	conn *minecraft.Conn, w *lwworld.World, cx, cz int,
	chunkCache *ChunkCache,
) {
	plainsBiomeID := uint32(dfbiome.Plains{}.EncodeBiome())

	pos := protocol.ChunkPos{int32(cx), int32(cz)}

	chunkCache.Mu.RLock()
	ch, ok := chunkCache.Cache[pos]
	chunkCache.Mu.RUnlock()

	if !ok {
		wchunk := w.LoadChunk(cx, cz)
		ch = convertWorldChunkToDragonfly(wchunk, plainsBiomeID)

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
		Position:      pos,
		Dimension:     0,
		SubChunkCount: 0,
		SubChunkLimit: protocol.Option(int32(highest)),
		CacheEnabled:  false,
		RawPayload:    payload,
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
			newCh := convertWorldChunkToDragonfly(wchunk, plainsBiomeID)
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
		RawPayload:          protocol.Option(rawData),
		HeightMapType:       hmType,
		HeightMapData:       hmData,
		RenderHeightMapType: hmType,
		RenderHeightMapData: hmData,
	}, true
}

// computeChunkHeightMap computes the Bedrock sub-chunk heightmap (Y of the Air
// block immediately above the highest light-diffusing or light-obstructing
// block in each column, or world Range().Min() if the column has no light
// blockers).
//
// Why we do NOT call ch.HeightMap() from dragonfly v0.11.5:
//  1. Operator-precedence bug in dragonfly v0.11.5 chunk.go:193:
//     `return int16(y) | chunk.SubY(index) + plus`
//     Because `+` binds tighter than `|` in Go, `int16(y) | (SubY + 1)` drops
//     the `+1` whenever local `y` is odd (1, 3, 5, 7, 9, 11, 13, 15)!
//     At local `y = 15` (top row of a sub-chunk, e.g. Y=63), this off-by-one
//     places the heightmap in the wrong sub-chunk (SubIndex 7 instead of 8),
//     marking the sub-chunk as having surface inside it instead of TooHigh(16).
//  2. FilteringBlock threshold bug:
//     dragonfly's highestLightBlocker only checks `FilteringBlock(rid) == 15`,
//     completely ignoring light-diffusing blocks with `FilteringBlock > 0`
//     such as `oak_leaves` (FilteringBlock = 1). When a tree stands in a
//     sub-chunk above ground (e.g. Y=64..79), the trunk column (oak_log,
//     filt=15) forces that sub-chunk to send `HeightMapDataHasData`, while all
//     surrounding leaf canopy columns get heightmap <= ground (`subMap = -1`,
//     "entire column in this sub-chunk is pure air"). Bedrock's client-side
//     SubChunkLighter then fails to propagate skylight into the leaf canopy,
//     trunk, ground beneath the canopy, and entities standing under the tree,
//     rendering them pitch black (light level 0).
func computeChunkHeightMap(ch *dfchunk.Chunk) [16][16]int16 {
	var hm [16][16]int16
	subs := ch.Sub()
	minY := int16(ch.Range().Min())
	airRID := AirRID()

	for z := uint8(0); z < 16; z++ {
		for x := uint8(0); x < 16; x++ {
			topY := minY
			found := false
			for subIdx := len(subs) - 1; subIdx >= 0 && !found; subIdx-- {
				sub := subs[subIdx]
				if sub.Empty() {
					continue
				}
				layers := sub.Layers()
				if len(layers) == 0 {
					continue
				}
				l0 := layers[0]
				baseY := (int16(subIdx) << 4) + minY
				for localY := int16(15); localY >= 0; localY-- {
					rid := l0.At(x, uint8(localY), z)
					if rid != airRID && IsHeightMapBlockerRID(rid) {
						topY = baseY + localY + 1
						found = true
						break
					}
				}
			}
			hm[z][x] = topY
		}
	}
	return hm
}

// SubChunkHeightMap computes the Bedrock SubChunkPacket heightmap type and
// 16x16 relative heightmap for sub-chunk index ind.
func SubChunkHeightMap(ch *dfchunk.Chunk, ind int16) (byte, protocol.Optional[protocol.HeightMap], bool) {
	hm := computeChunkHeightMap(ch)
	higher, lower := true, true
	subMap := protocol.HeightMap{}
	minInd := int16(ch.Range().Min() >> 4)
	for z := uint8(0); z < 16; z++ {
		for x := uint8(0); x < 16; x++ {
			y := hm[z][x]
			otherInd := (y >> 4) - minInd
			switch {
			case otherInd > ind:
				subMap[z][x], lower = 16, false
			case otherInd < ind:
				subMap[z][x], higher = -1, false
			default:
				subMap[z][x], lower, higher = int8(y&15), false, false
			}
		}
	}
	var subMapData protocol.Optional[protocol.HeightMap]
	subMapType := byte(protocol.HeightMapDataHasData)
	switch {
	case higher:
		subMapType = protocol.HeightMapDataTooHigh
	case lower:
		subMapType = protocol.HeightMapDataTooLow
	default:
		subMapData = protocol.Option(subMap)
	}
	return subMapType, subMapData, subMapType == protocol.HeightMapDataHasData
}

func subChunkHeightMap(ch *dfchunk.Chunk, ind int16) (byte, protocol.Optional[protocol.HeightMap], bool) {
	return SubChunkHeightMap(ch, ind)
}
