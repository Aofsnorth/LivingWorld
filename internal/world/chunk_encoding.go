package world

import (
	"encoding/binary"
	"fmt"
)

// chunkFormatVersion is the on-disk chunk encoding version. Bump when the layout
// changes so older files can be detected/migrated.
//
// v2: canonical Y unification (-64..319). The byte layout is unchanged, but
// section bytes are now interpreted as -64-based world sections. Pre-v2 superflat
// worlds stored their floor in section 0 (world-Y -64..-49) instead of section 4
// (world-Y 0..15), so loading them as v2 would drop the surface; the version
// mismatch makes DecodeChunk reject them and World.LoadChunk regenerates instead.
//
// v3: terrain source switched to the vanilla-gen adapter (overworld,
// nether and end). v2 chunks hold terrain from the previous hand-rolled
// pipeline; rejecting them forces regeneration with the new generator
// instead of serving stale, differently-shaped terrain next to new chunks.
//
// v4: overworld terrain switched to SteelMC Java 26.3 native generator
// (cinnabar-mc/worldgen-lab). Rejecting v3 chunks forces existing worlds to
// regenerate with the new 26.3 terrain pipeline.
//
// v5: persists 3D section biomes (4x4x4 per section) alongside blocks and
// rejects older chunks that had discarded SteelMC biomes (or were generated
// with the blank seed=0 bug).
const chunkFormatVersion byte = 5

// Encode serializes a chunk's block and biome data into a compact binary blob.
//
// Layout (little-endian):
//
//	byte    version (5)
//	uint32  section bitmask (bit i set => section i is non-empty and follows)
//	[]int32 4096 block IDs per non-empty section, in ascending section order
//	uint32  biome bitmask (bit i set => section i has 64 uint16 biome IDs following)
//	[]uint16 64 biome IDs per present biome section, in ascending section order
func (c *Chunk) Encode() []byte {
	var mask uint32
	present := 0
	var biomeMask uint32
	biomePresent := 0
	for i := range c.sections {
		if !c.sections[i].IsEmpty() {
			mask |= 1 << uint(i)
			present++
		}
		if c.sections[i].biomes != nil {
			biomeMask |= 1 << uint(i)
			biomePresent++
		}
	}

	out := make([]byte, 5+present*4096*4+4+biomePresent*64*2)
	out[0] = chunkFormatVersion
	binary.LittleEndian.PutUint32(out[1:5], mask)

	pos := 5
	for i := range c.sections {
		if c.sections[i].IsEmpty() {
			continue
		}
		for _, id := range c.sections[i].blocks {
			binary.LittleEndian.PutUint32(out[pos:], uint32(id))
			pos += 4
		}
	}

	binary.LittleEndian.PutUint32(out[pos:pos+4], biomeMask)
	pos += 4
	for i := range c.sections {
		b := c.sections[i].biomes
		if b == nil {
			continue
		}
		for _, bid := range b {
			binary.LittleEndian.PutUint16(out[pos:], uint16(bid))
			pos += 2
		}
	}
	return out
}

// DecodeChunk reconstructs a chunk from bytes produced by Encode.
func DecodeChunk(data []byte) (*Chunk, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("chunk data too short: %d bytes", len(data))
	}
	if data[0] != chunkFormatVersion {
		return nil, fmt.Errorf("unsupported chunk format version %d", data[0])
	}
	mask := binary.LittleEndian.Uint32(data[1:5])
	pos := 5

	c := NewChunk()
	for i := 0; i < SectionsPerChunk; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		end := pos + 4096*4
		if end > len(data) {
			return nil, fmt.Errorf("chunk data truncated reading section %d", i)
		}
		sec := &c.sections[i]
		blocks := make([]int32, 4096)
		var nonAir int32
		for j := 0; j < 4096; j++ {
			id := int32(binary.LittleEndian.Uint32(data[pos:]))
			pos += 4
			blocks[j] = id
			if id != AirID {
				nonAir++
			}
		}
		if nonAir > 0 {
			sec.blocks = blocks
			sec.nonAirBlocks = nonAir
		}
	}

	if pos+4 <= len(data) {
		biomeMask := binary.LittleEndian.Uint32(data[pos : pos+4])
		pos += 4
		for i := 0; i < SectionsPerChunk; i++ {
			if biomeMask&(1<<uint(i)) == 0 {
				continue
			}
			if pos+64*2 > len(data) {
				return nil, fmt.Errorf("chunk data truncated reading biomes for section %d", i)
			}
			var b [64]int32
			for j := 0; j < 64; j++ {
				b[j] = int32(binary.LittleEndian.Uint16(data[pos:]))
				pos += 2
			}
			c.sections[i].biomes = &b
		}
	}
	return c, nil
}
