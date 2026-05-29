package bedrock

import (
	"fmt"
	"log"

	dfbiome "github.com/df-mc/dragonfly/server/world/biome"
	dfworld "github.com/df-mc/dragonfly/server/world"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// chunkConverter handles converting internal world chunks to Bedrock format.
type chunkConverter struct{}

func newChunkConverter() *chunkConverter {
	return &chunkConverter{}
}

// blockRID resolves a block's runtime ID for Bedrock.
func blockRID(name string, properties ...map[string]any) uint32 {
	var props map[string]any
	if len(properties) > 0 {
		props = properties[0]
	}
	rid, ok := dfchunk.StateToRuntimeID(name, props)
	if !ok {
		log.Printf("[Bedrock] Could not resolve runtime ID for %s", name)
		return 0
	}
	return rid
}

// logBlockPaletteVersion logs key diagnostic info about the block palette.
func logBlockPaletteVersion() {
	airRID := blockRID("minecraft:air")
	bedrockRID := blockRID("minecraft:bedrock")
	dirtRID := blockRID("minecraft:dirt")
	grassRID := blockRID("minecraft:grass_block", map[string]any{"minecraft:snowy_bit": false})
	plainsBiome := uint32(dfbiome.Plains{}.EncodeBiome())

	_, props, found := dfchunk.RuntimeIDToState(airRID)
	ver := int32(-1)
	if found {
		if v, ok := props["version"]; ok {
			ver = v.(int32)
		}
	}

	log.Printf("[Bedrock] Block palette: air=%d bedrock=%d dirt=%d grass=%d biome_plains=%d",
		airRID, bedrockRID, dirtRID, grassRID, plainsBiome)
	log.Printf("[Bedrock] air state: found=%v props=%+v blockVersion=%d", found, props, ver)
	log.Printf("[Bedrock] gophertunnel protocol=%d version=%s",
		protocol.CurrentProtocol, protocol.CurrentVersion)

	if airRID == 0 {
		log.Printf("[Bedrock] WARNING: air RID is 0 — blocks will be invisible!")
	}
	if bedrockRID == 0 || dirtRID == 0 || grassRID == 0 {
		log.Printf("[Bedrock] WARNING: one or more terrain block RIDs are 0 — palette mismatch likely")
	}
}

// sendInitialChunks builds flat terrain chunks and sends them inline in
// LevelChunk packets. This is the classic approach that works across all
// Bedrock protocol versions.  chunkCache is populated so that
// handleSubChunkRequest can serve the modern sub-chunk request system too.
func (c *chunkConverter) sendInitialChunks(
	conn *minecraft.Conn, centerChunkX, centerChunkZ, radius int, groundY int16,
	chunkCache map[protocol.ChunkPos]*dfchunk.Chunk,
) {
	rng := dfworld.Overworld.Range()
	airRID := blockRID("minecraft:air")
	bedrockRID := blockRID("minecraft:bedrock")
	dirtRID := blockRID("minecraft:dirt")
	grassRID := blockRID("minecraft:grass_block", map[string]any{"minecraft:snowy_bit": false})
	plainsBiomeID := uint32(dfbiome.Plains{}.EncodeBiome())

	bedrockY := groundY - 3
	dirtY1 := groundY - 2
	dirtY2 := groundY - 1
	grassBlockY := groundY

	// Total sub-chunks for the overworld range (-64..319 → 24 sub-chunks).
	subChunkCount := uint32((rng.Height() >> 4) + 1)

	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			cx, cz := centerChunkX+dx, centerChunkZ+dz
			ch := dfchunk.New(airRID, rng)

			for x := uint8(0); x < 16; x++ {
				for z := uint8(0); z < 16; z++ {
					setBlock := func(y int16, rid uint32) {
						if y >= int16(rng.Min()) && y <= int16(rng.Max()) {
							ch.SetBlock(x, y, z, 0, rid)
						}
					}
					setBlock(bedrockY, bedrockRID)
					setBlock(dirtY1, dirtRID)
					setBlock(dirtY2, dirtRID)
					setBlock(grassBlockY, grassRID)
				}
			}
			for y := int16(rng.Min()); y <= int16(rng.Max()); y += 4 {
				for x := uint8(0); x < 16; x += 4 {
					for z := uint8(0); z < 16; z += 4 {
						ch.SetBiome(x, y, z, plainsBiomeID)
					}
				}
			}

			// Cache for the SubChunkRequest system (if the client uses it).
			pos := protocol.ChunkPos{int32(cx), int32(cz)}
			chunkCache[pos] = ch

			// --- Build the inline payload (all sub-chunks + biomes + border) ---
			data := dfchunk.Encode(ch, dfchunk.NetworkEncoding)
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
	}

	log.Printf("[Bedrock] Sent %d chunks (inline), groundY=%d, subChunks=%d",
		(2*radius+1)*(2*radius+1), groundY, subChunkCount)
}

// handleSubChunkRequest processes SubChunkRequest packets for the modern
// sub-chunk request path.  Currently unused when inline mode is active, but
// kept ready for future use.
func (c *chunkConverter) handleSubChunkRequest(
	conn *minecraft.Conn, pk *packet.SubChunkRequest,
	chunkCache map[protocol.ChunkPos]*dfchunk.Chunk,
) {
	rng := dfworld.Overworld.Range()
	center := pk.Position

	entries := make([]protocol.SubChunkEntry, 0, len(pk.Offsets))
	for _, offset := range pk.Offsets {
		absX := center.X() + int32(offset[0])
		absZ := center.Z() + int32(offset[2])
		absYInd := center.Y() + int32(offset[1])

		subChunkCount := int32((rng.Height() >> 4) + 1)
		if absYInd < 0 || absYInd >= subChunkCount {
			entries = append(entries, protocol.SubChunkEntry{
				Offset: offset,
				Result: protocol.SubChunkResultIndexOutOfBounds,
			})
			continue
		}

		chunkPos := protocol.ChunkPos{absX, absZ}
		ch, ok := chunkCache[chunkPos]
		if !ok {
			entries = append(entries, protocol.SubChunkEntry{
				Offset: offset,
				Result: protocol.SubChunkResultChunkNotFound,
			})
			continue
		}

		sub := ch.Sub()[absYInd]
		if sub.Empty() {
			entries = append(entries, protocol.SubChunkEntry{
				Offset:        offset,
				Result:        protocol.SubChunkResultSuccessAllAir,
				HeightMapType: protocol.HeightMapDataNone,
			})
			continue
		}

		rawData := dfchunk.EncodeSubChunk(ch, dfchunk.NetworkEncoding, int(absYInd))
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

// sendSetTime sends the SetTime packet so the Bedrock client shows the correct
// time-of-day (sun position).
func sendSetTime(conn *minecraft.Conn, ticks int32) error {
	return conn.WritePacket(&packet.SetTime{
		Time: ticks,
	})
}

// dimensionMismatchError formats a clear message when the client's block palette
// doesn't match dragonfly's embedded one.
func dimensionMismatchError(clientProtocol int32) string {
	return fmt.Sprintf(
		"Bedrock client protocol %d doesn't match dragonfly's embedded palette (protocol %d). "+
			"Blocks will render invisible. Update dragonfly or use a matching client version.",
		clientProtocol, protocol.CurrentProtocol)
}

// inlinePayloadBuffer is a small helper to build chunk payloads.
// (Separated so we can swap implementations easily.)
func newInlinePayloadBuffer() *inlinePayloadBuffer {
	return &inlinePayloadBuffer{}
}

type inlinePayloadBuffer struct {
	data []byte
}

func (b *inlinePayloadBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *inlinePayloadBuffer) WriteByte(c byte) error {
	b.data = append(b.data, c)
	return nil
}

func (b *inlinePayloadBuffer) Bytes() []byte {
	return b.data
}
