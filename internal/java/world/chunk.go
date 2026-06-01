package world

import (
	"livingworld/internal/world"
	"log"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/biome"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// airStateID is the global state ID of air (0). LivingWorld's canonical world
// block IDs ARE Java global state IDs, so no per-block translation is needed.
var airStateID = block.StateID(block.ToStateID[block.Air{}])

// maxValidStateID is the highest valid block state index in the global palette.
var maxValidStateID = block.StateID(len(block.StateList) - 1)

// ExportConvertToLevelChunk is exported for testing.
func ExportConvertToLevelChunk(wChunk *world.Chunk) *level.Chunk {
	return ConvertToLevelChunk(wChunk)
}

func ConvertToLevelChunk(wChunk *world.Chunk) *level.Chunk {
	lChunk := level.EmptyChunk(24)
	lChunk.Status = level.StatusFull

	var highestBlock [16][16]int
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			// Sentinel one below the world floor (-65), so a fully-air column yields
			// heightmap 0 (val=-65+1+64) while a real block at the floor world-Y -64
			// still updates (-64 > -65) and yields 1.
			highestBlock[x][z] = -65
		}
	}

	for secIdx := 0; secIdx < 24; secIdx++ {
		sec := &lChunk.Sections[secIdx]
		sec.Biomes = level.NewBiomesPaletteContainer(4*4*4, biome.Type(0))

		sec.SkyLight = make([]byte, 2048)
		for j := range sec.SkyLight {
			sec.SkyLight[j] = 0xFF
		}
		// No block light: this world has no light-emitting blocks, so leaving
		// BlockLight nil marks every section in the empty-block-light mask (client
		// assumes zero) instead of shipping 2048 zero bytes × 24 sections (~49 KB
		// of waste per chunk, which doubled every chunk packet to ~101 KB).

		minSecY := -64 + secIdx*16
		// Sections -64..-1 are now valid (canonical Y unification): emit them so
		// deepslate/bedrock and caves below Y=0 render instead of being skipped.

		blockCount := int16(0)
		for ly := 0; ly < 16; ly++ {
			y := minSecY + ly
			for lz := 0; lz < 16; lz++ {
				for lx := 0; lx < 16; lx++ {
					b := wChunk.GetBlock(lx, y, lz)
					rawID := b.ID()
					if rawID == 0 {
						continue
					}

					// Canonical world IDs are Java global state IDs: use directly.
					stateID := block.StateID(rawID)

					// Guard against corrupt/out-of-range block IDs that would
					// panic inside PaletteContainer.Set → BitStorage.Set.
					if stateID < 0 || stateID > maxValidStateID {
						log.Printf("[Java] ConvertToLevelChunk: skipping invalid stateID %d at (%d,%d,%d)", stateID, lx, y, lz)
						continue
					}

					if stateID != airStateID {
						idx := (ly << 8) | (lz << 4) | lx
						sec.States.Set(idx, level.BlocksState(stateID))
						blockCount++
						if y > highestBlock[lx][lz] {
							highestBlock[lx][lz] = y
						}
					}
				}
			}
		}
		sec.BlockCount = blockCount
	}

	chunkHeight := 24 * 16
	bitsPerEntry := 0
	for b := chunkHeight + 1; b > 0; b >>= 1 {
		bitsPerEntry++
	}
	maxHeightVal := (1 << bitsPerEntry) - 1
	hm := level.NewBitStorage(bitsPerEntry, 16*16, nil)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			val := highestBlock[x][z] + 1 + 64
			if val < 0 {
				val = 0
			}
			if val > maxHeightVal {
				val = maxHeightVal
			}
			hm.Set(x*16+z, val)
		}
	}
	lChunk.HeightMaps.MotionBlocking = hm
	lChunk.HeightMaps.WorldSurface = hm

	return lChunk
}

// BuildLevelChunkWithLightPacket builds a LevelChunkWithLight packet using
// go-mc's native serialization for protocol 775+.
//
// NOTE on chunk-position encoding: this packet sends the position as two plain
// Ints in [X, Z] order (go-mc's level.ChunkPos). That is correct here — do NOT
// reuse level.ChunkPos for ForgetLevelChunk below, which uses a different
// encoding (see BuildForgetLevelChunkPacket).
func BuildLevelChunkWithLightPacket(x, z int32, lChunk *level.Chunk) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameLevelChunkWithLight,
		level.ChunkPos{x, z},
		lChunk,
	)
}

// BuildForgetLevelChunkPacket builds a ForgetLevelChunk (unload chunk) packet.
//
// CONTRAST with BuildLevelChunkWithLightPacket: the vanilla client decodes this
// packet's position as a single packed Long via ChunkPos.toLong(), where X is
// the low 32 bits and Z is the high 32 bits. Written big-endian, the high bits
// (Z) come out on the wire FIRST, so the byte layout is [Z][X] — the reverse of
// LevelChunkWithLight's [X][Z]. Sending two Ints in [X, Z] order (as
// level.ChunkPos would) makes the client unload the chunk mirrored across the
// X=Z line, so chunks still in view get dropped ("void chases the player").
//
// We write the packed long directly rather than two swapped Ints to keep the
// "this is one long, not a coordinate pair" intent obvious.
func BuildForgetLevelChunkPacket(x, z int32) pk.Packet {
	packed := (int64(z) << 32) | (int64(x) & 0xFFFFFFFF)
	return pk.Marshal(
		packetid.ClientboundGameForgetLevelChunk,
		pk.Long(packed),
	)
}
