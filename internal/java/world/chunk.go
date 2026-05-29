package world

import (
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/biome"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

var (
	airStateID     = block.StateID(block.ToStateID[block.Air{}])
	bedrockStateID = block.StateID(block.ToStateID[block.Bedrock{}])
	dirtStateID    = block.StateID(block.ToStateID[block.Dirt{}])
	grassStateID   = block.StateID(block.ToStateID[block.GrassBlock{}])
)

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
		sec.BlockLight = make([]byte, 2048)

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
					if b.ID() == 0 {
						continue
					}

					var stateID block.StateID
					switch b.ID() {
					case 1:
						stateID = bedrockStateID
					case 2:
						stateID = dirtStateID
					case 3:
						stateID = grassStateID
					default:
						stateID = airStateID
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
	hm := level.NewBitStorage(bitsPerEntry, 16*16, nil)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			val := highestBlock[x][z] + 1 + 64
			if val < 0 {
				val = 0
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
