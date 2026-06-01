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
	airRID := BlockRID("minecraft:air")
	plainsBiomeID := uint32(dfbiome.Plains{}.EncodeBiome())
	maxY := int(rng.Max()) // 319; world blocks live at Y >= 0
	subChunkCount := uint32((rng.Height() >> 4) + 1)

	pos := protocol.ChunkPos{int32(cx), int32(cz)}

	chunkCache.Mu.Lock()
	ch, ok := chunkCache.Cache[pos]
	chunkCache.Mu.Unlock()

	if !ok {
		wchunk := w.LoadChunk(cx, cz)
		ch = dfchunk.New(airRID, rng)

		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				// Canonical Y: scan the full -64..319 column (dragonfly's chunk is
				// already -64-based, so world-Y passes straight to ch.SetBlock).
				for y := int(rng.Min()); y <= maxY; y++ {
					id := wchunk.GetBlock(x, y, z).ID()
					if id == 0 {
						continue
					}
					ch.SetBlock(uint8(x), int16(y), uint8(z), 0, LivingWorldBlockIDToBedrockRID(id))
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

	chunkCache.Mu.RLock()
	data := dfchunk.Encode(ch, dfchunk.NetworkEncoding)
	chunkCache.Mu.RUnlock()

	buf := newInlinePayloadBuffer()
	for _, sub := range data.SubChunks {
		_, _ = buf.Write(sub)
	}
	_, _ = buf.Write(data.Biomes)
	buf.WriteByte(0) // border block count = 0

	err := conn.WritePacket(&packet.LevelChunk{
		Position:      pos,
		Dimension:     0,
		SubChunkCount: subChunkCount,
		CacheEnabled:  false,
		RawPayload:    buf.Bytes(),
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
	airRID := BlockRID("minecraft:air")
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
		chunkCache.Mu.RLock()
		ch, ok := chunkCache.Cache[chunkPos]
		chunkCache.Mu.RUnlock()
		if !ok {
			wchunk := w.LoadChunk(int(absX), int(absZ))
			ch = dfchunk.New(airRID, rng)
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					// Canonical Y: scan the full -64..319 column (see SendChunk).
					for y := int(rng.Min()); y <= maxY; y++ {
						id := wchunk.GetBlock(x, y, z).ID()
						if id == 0 {
							continue
						}
						ch.SetBlock(uint8(x), int16(y), uint8(z), 0, LivingWorldBlockIDToBedrockRID(id))
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
			chunkCache.Cache[chunkPos] = ch
			chunkCache.Mu.Unlock()
		}

		chunkCache.Mu.RLock()
		sub := ch.Sub()[arrayYInd]
		if sub.Empty() {
			chunkCache.Mu.RUnlock()
			entries = append(entries, protocol.SubChunkEntry{
				Offset:        offset,
				Result:        protocol.SubChunkResultSuccessAllAir,
				HeightMapType: protocol.HeightMapDataNone,
			})
			continue
		}

		rawData := dfchunk.EncodeSubChunk(ch, dfchunk.NetworkEncoding, int(arrayYInd))
		chunkCache.Mu.RUnlock()
		entries = append(entries, protocol.SubChunkEntry{
			Offset:              offset,
			Result:              protocol.SubChunkResultSuccess,
			RawPayload:          rawData,
			HeightMapType:       protocol.HeightMapDataNone,
			RenderHeightMapType: protocol.HeightMapDataNone,
		})
	}

	_ = conn.WritePacket(&packet.SubChunk{
		CacheEnabled:    false,
		Dimension:       pk.Dimension,
		Position:        center,
		SubChunkEntries: entries,
	})
}
