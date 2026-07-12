package world

import (
	"bytes"
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
const chunkFormatVersion byte = 2

// Encode serializes a chunk's block data into a compact binary blob.
//
// Layout (little-endian):
//
//	byte    version
//	uint32  section bitmask (bit i set => section i is non-empty and follows)
//	[]int32 4096 block IDs per non-empty section, in ascending section order
//
// Only blocks are persisted; light and biomes are regenerated on load. This keeps
// files small (empty sections cost nothing) and gzip-friendly.
func (c *Chunk) Encode() []byte {
	var buf bytes.Buffer
	buf.WriteByte(chunkFormatVersion)

	var mask uint32
	for i := range c.sections {
		if !c.sections[i].IsEmpty() {
			mask |= 1 << uint(i)
		}
	}
	var maskBytes [4]byte
	binary.LittleEndian.PutUint32(maskBytes[:], mask)
	buf.Write(maskBytes[:])

	var word [4]byte
	for i := range c.sections {
		if c.sections[i].IsEmpty() {
			continue
		}
		for _, id := range c.sections[i].blocks {
			binary.LittleEndian.PutUint32(word[:], uint32(id))
			buf.Write(word[:])
		}
	}
	return buf.Bytes()
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
		var nonAir int32
		for j := 0; j < 4096; j++ {
			id := int32(binary.LittleEndian.Uint32(data[pos:]))
			pos += 4
			sec.blocks[j] = id
			if id != AirID {
				nonAir++
			}
		}
		sec.nonAirBlocks = nonAir
	}
	return c, nil
}
