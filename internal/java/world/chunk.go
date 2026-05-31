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
			highestBlock[x][z] = -64
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
		if minSecY+15 < 0 {
			continue
		}

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
func BuildLevelChunkWithLightPacket(x, z int32, lChunk *level.Chunk) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameLevelChunkWithLight,
		level.ChunkPos{x, z},
		lChunk,
	)
}
