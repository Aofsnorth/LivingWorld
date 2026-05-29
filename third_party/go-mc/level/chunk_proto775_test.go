package level

import (
	"bytes"
	"io"
	"math/bits"
	"testing"

	"github.com/Tnze/go-mc/level/biome"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// buildProto775TestChunk builds a chunk resembling what the server sends to a
// real client: 24 sections, mostly air (single-value palettes), one section
// with a couple of block states (indirect 4-bit palette), heightmaps set, and
// sky light present on every section.
func buildProto775TestChunk() *Chunk {
	c := EmptyChunk(24)
	c.Status = StatusFull

	// Section 4 gets two distinct block states -> indirect (4-bit) palette,
	// which exercises a non-empty packed data array.
	sec := &c.Sections[4]
	for i := 0; i < 32; i++ {
		sec.SetBlock(i, BlocksState(1))
	}

	// Sky light on every section -> exercises the light array encoding.
	for i := range c.Sections {
		c.Sections[i].SkyLight = make([]byte, 2048)
	}

	// Heightmaps: 9 bits per entry for a 384-tall world (height+1 = 385).
	bpe := bits.Len(uint(24*16) + 1)
	hm := NewBitStorage(bpe, 16*16, nil)
	for i := 0; i < 256; i++ {
		hm.Set(i, 64)
	}
	c.HeightMaps.MotionBlocking = hm
	c.HeightMaps.WorldSurface = hm
	return c
}

// TestChunkWireFormatProto775 marshals a Chunk and then re-reads the bytes
// exactly as a protocol-775 (MC 1.21.5+/26.1) client would. The client's
// PacketDecoder rejects a chunk packet if any bytes remain after its read
// method completes ("...was larger than I expected, found N bytes extra").
// This test reproduces that check: the spec-compliant reader must consume the
// entire payload with nothing left over.
func TestChunkWireFormatProto775(t *testing.T) {
	c := buildProto775TestChunk()

	var buf bytes.Buffer
	if _, err := c.WriteTo(&buf); err != nil {
		t.Fatalf("Chunk.WriteTo: %v", err)
	}
	data := buf.Bytes()
	r := bytes.NewReader(data)
	total := len(data)

	readVarInt := func(what string) int {
		var v pk.VarInt
		if _, err := v.ReadFrom(r); err != nil {
			t.Fatalf("read VarInt (%s): %v", what, err)
		}
		return int(v)
	}
	readUByte := func() int {
		var v pk.UnsignedByte
		if _, err := v.ReadFrom(r); err != nil {
			t.Fatalf("read UnsignedByte: %v", err)
		}
		return int(v)
	}
	skipLongs := func(n int, what string) {
		var l pk.Long
		for i := 0; i < n; i++ {
			if _, err := l.ReadFrom(r); err != nil {
				t.Fatalf("read Long (%s, idx %d/%d): %v", what, i, n, err)
			}
		}
	}

	// (1) Heightmaps: prefixed array of { VarInt type, prefixed Long array }.
	// If heightmaps were still NBT-encoded the first byte would be 0x0A
	// (TAG_Compound = 10), so the count would read as 10 and the types/longs
	// would be garbage. Guard against that.
	hmCount := readVarInt("heightmap count")
	if hmCount < 0 || hmCount > 6 {
		t.Fatalf("heightmap count = %d (implausible) — heightmaps are likely still NBT-encoded instead of the protocol-775 prefixed array", hmCount)
	}
	for i := 0; i < hmCount; i++ {
		typ := readVarInt("heightmap type")
		if typ < 0 || typ > 5 {
			t.Fatalf("heightmap type = %d out of [0,5] — heightmap section is misaligned", typ)
		}
		skipLongs(readVarInt("heightmap long count"), "heightmap data")
	}

	// (2) Data: VarInt size, then exactly that many bytes of section data.
	size := readVarInt("data size")
	sectionConsumedStart := total - r.Len()

	readContainer := func(entries, directBits int, kind string) {
		b := readUByte()
		switch {
		case b == 0: // single-value palette: one VarInt, empty data array
			readVarInt(kind + " single value")
		case b < directBits: // indirect palette: count + ids
			n := readVarInt(kind + " palette len")
			for i := 0; i < n; i++ {
				readVarInt(kind + " palette id")
			}
		default: // direct palette: no palette fields
		}
		// Protocol 775: NO data-array-length VarInt. Long count is computed.
		skipLongs(calcBitStorageSize(b, entries), kind+" data")
	}

	for s := 0; s < len(c.Sections); s++ {
		var bc, fc pk.Short
		if _, err := bc.ReadFrom(r); err != nil {
			t.Fatalf("read section %d block count: %v", s, err)
		}
		// Protocol 775 added a fluidCount Short after blockCount.
		if _, err := fc.ReadFrom(r); err != nil {
			t.Fatalf("read section %d fluid count: %v", s, err)
		}
		readContainer(16*16*16, block.BitsPerBlock, "block states")
		readContainer(4*4*4, biome.BitsPerBiome, "biomes")
	}

	consumed := (total - r.Len()) - sectionConsumedStart
	if consumed != size {
		t.Fatalf("section data desync: data size header = %d but spec-775 parse consumed %d bytes — palette container likely still writes a Data Array Length VarInt", size, consumed)
	}

	// (3) Block entities: prefixed array (empty for this test chunk).
	if be := readVarInt("block entity count"); be != 0 {
		t.Fatalf("expected 0 block entities, got %d (misaligned)", be)
	}

	// (4) Light data: 4 BitSets, then sky/block light prefixed arrays.
	readBitSet := func(what string) { skipLongs(readVarInt(what+" len"), what) }
	readBitSet("sky light mask")
	readBitSet("block light mask")
	readBitSet("empty sky light mask")
	readBitSet("empty block light mask")
	readLightArrays := func(what string) {
		n := readVarInt(what + " count")
		for i := 0; i < n; i++ {
			length := readVarInt(what + " entry len")
			b := make([]byte, length)
			if _, err := io.ReadFull(r, b); err != nil {
				t.Fatalf("read %s entry %d (len %d): %v", what, i, length, err)
			}
		}
	}
	readLightArrays("sky light")
	readLightArrays("block light")

	if r.Len() != 0 {
		t.Fatalf("a protocol-775 client would reject this chunk: found %d bytes extra after a complete read (reproduces the level_chunk_with_light disconnect)", r.Len())
	}
}
