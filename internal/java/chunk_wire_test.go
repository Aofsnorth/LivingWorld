package java

import (
	"encoding/binary"
	"testing"

	"livingworld/internal/world"
	worldgen "livingworld/internal/world/generator"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/level"
	"github.com/Tnze/go-mc/level/biome"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

// clientReader re-parses the level_chunk_with_light payload EXACTLY as a
// protocol-775 (MC 1.21.5 / 26.1) Java client does. The long count for every
// paletted container is derived with the client's own formula
// (size + valuesPerLong - 1) / valuesPerLong from the bits byte on the wire —
// this mirrors ViaVersion's PaletteType1_21_5.readValues and Mojang's
// SimpleBitStorage, NOT go-mc's helper, so a disagreement between go-mc's
// packing and the client's expectation would surface as an overrun or leftover.
type clientReader struct {
	t    *testing.T
	data []byte
	off  int
}

func (r *clientReader) need(n int, what string) {
	if r.off+n > len(r.data) {
		r.t.Fatalf("OVERRUN reading %s: readerIndex(%d)+length(%d) exceeds writerIndex(%d) "+
			"— reproduces the client IndexOutOfBoundsException", what, r.off, n, len(r.data))
	}
}

func (r *clientReader) varInt(what string) int {
	var v, pos int
	for {
		r.need(1, what)
		b := r.data[r.off]
		r.off++
		v |= int(b&0x7F) << (7 * pos)
		if b&0x80 == 0 {
			break
		}
		if pos++; pos >= 5 {
			r.t.Fatalf("varint %s too long", what)
		}
	}
	return v
}

func (r *clientReader) u8(what string) int { r.need(1, what); b := r.data[r.off]; r.off++; return int(b) }
func (r *clientReader) i16(what string) int {
	r.need(2, what)
	v := int16(binary.BigEndian.Uint16(r.data[r.off:]))
	r.off += 2
	return int(v)
}
func (r *clientReader) skip(n int, what string) { r.need(n, what); r.off += n }

func clientLongCount(bits, size int) int {
	if bits == 0 {
		return 0
	}
	vpl := 64 / bits
	return (size + vpl - 1) / vpl
}

// TestSuperflatChunkParsesAsClient builds the real chunk the server sends
// (superflat via convertToLevelChunk), marshals the full LevelChunkWithLight
// packet, then parses the section-data sub-buffer in isolation — the same way
// Mojang's LevelChunk.replaceWithPacketData parses the byte[] it received. The
// section sub-buffer MUST be consumed exactly: a pending long read at its end is
// precisely the crash "readerIndex(N)+length(8) exceeds writerIndex(N)".
func TestSuperflatChunkParsesAsClient(t *testing.T) {
	w := world.NewWorld("test")
	w.SetGenerator(worldgen.NewSuperflat())
	wChunk := w.LoadChunk(0, 0)
	lChunk := convertToLevelChunk(wChunk)

	pkt := pk.Marshal(packetid.ClientboundGameLevelChunkWithLight, level.ChunkPos{0, 0}, lChunk)
	r := &clientReader{t: t, data: pkt.Data}

	r.skip(8, "ChunkPos")

	// Heightmaps: VarInt count, per entry { VarInt type, VarInt longCount, longs }.
	hmCount := r.varInt("heightmap count")
	if hmCount < 0 || hmCount > 6 {
		t.Fatalf("implausible heightmap count %d (heightmaps likely not the prefixed-array format)", hmCount)
	}
	for i := 0; i < hmCount; i++ {
		r.varInt("heightmap type")
		r.skip(r.varInt("heightmap long count")*8, "heightmap data")
	}

	// Section data: VarInt size, then EXACTLY that many bytes. The client wraps
	// these bytes in their own buffer (cap == size) and parses sections from it,
	// so we slice the sub-buffer and require it to be fully consumed.
	dataSize := r.varInt("data size")
	r.need(dataSize, "section data")
	sub := &clientReader{t: t, data: r.data[r.off : r.off+dataSize]}
	r.off += dataSize

	const sections = 24
	blockLongsTotal := 0
	for s := 0; s < sections; s++ {
		sub.i16("blockCount")
		sub.i16("fluidCount") // new in protocol 775 (MC 26.1); see Section.FluidCount
		// Block states container.
		bBits := sub.u8("block bits")
		switch {
		case bBits == 0:
			sub.varInt("block single value")
		case bBits <= 8:
			n := sub.varInt("block palette len")
			for j := 0; j < n; j++ {
				sub.varInt("block palette id")
			}
		default: // direct: no palette
		}
		bLongs := clientLongCount(blockCfgBits(bBits), 16*16*16)
		blockLongsTotal += bLongs
		sub.skip(bLongs*8, "block data")
		// Biomes container.
		mBits := sub.u8("biome bits")
		switch {
		case mBits == 0:
			sub.varInt("biome single value")
		case mBits <= 3:
			n := sub.varInt("biome palette len")
			for j := 0; j < n; j++ {
				sub.varInt("biome palette id")
			}
		default:
		}
		sub.skip(clientLongCount(biomeCfgBits(mBits), 4*4*4)*8, "biome data")
	}

	if sub.off != len(sub.data) {
		t.Fatalf("section sub-buffer not fully consumed: consumed %d of %d (leftover %d) — "+
			"a desync between go-mc packing and the client", sub.off, len(sub.data), len(sub.data)-sub.off)
	}

	// Superflat: only section 4 has blocks (bedrock/dirt/grass + air = 4 states,
	// 4-bit linear palette -> 256 longs); every other section is single-value
	// air (0 longs). Lock that in so a packing regression is caught loudly.
	if blockLongsTotal != 256 {
		t.Fatalf("expected exactly 256 block-state longs across all sections (superflat), got %d", blockLongsTotal)
	}

	// Block entities, then light — the rest of the packet must also fully drain.
	if be := r.varInt("block entity count"); be != 0 {
		t.Fatalf("expected 0 block entities, got %d", be)
	}
	for _, what := range []string{"sky mask", "block mask", "empty sky mask", "empty block mask"} {
		r.skip(r.varInt(what+" len")*8, what)
	}
	for _, what := range []string{"sky light", "block light"} {
		n := r.varInt(what + " count")
		for i := 0; i < n; i++ {
			r.skip(r.varInt(what+" entry len"), what+" entry")
		}
	}
	if r.off != len(r.data) {
		t.Fatalf("packet not fully consumed: %d bytes left — a client would reject (\"found %d bytes extra\")",
			len(r.data)-r.off, len(r.data)-r.off)
	}
}

// blockCfgBits / biomeCfgBits map the wire bits byte to the bits used for the
// data-array long count, per the client's strategy (blocks: 1-4 -> linear 4,
// 5-8 -> hashmap, else direct; biomes: 1-3 -> linear, else direct).
func blockCfgBits(b int) int {
	switch {
	case b == 0:
		return 0
	case b <= 4:
		return 4
	case b <= 8:
		return b
	default:
		return block.BitsPerBlock
	}
}

func biomeCfgBits(b int) int {
	switch {
	case b == 0:
		return 0
	case b <= 3:
		return b
	default:
		return biome.BitsPerBiome
	}
}
