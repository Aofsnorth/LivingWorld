// Package worldconvert converts worlds between vanilla Minecraft formats and the
// LivingWorld on-disk format.
//
// The conversion pivots on the fact that LivingWorld's canonical block ID *is*
// the vanilla Java global block-state ID (see internal/world/registry.go), so
// Java Anvil <-> LivingWorld is effectively identity at the block-state level —
// we only translate the container layout (Anvil bit-packed palette sections vs.
// LivingWorld's flat int32 sections), pivoting on the block's namespaced name.
//
// Fidelity (v1): block *types* are preserved for every vanilla block. Block-state
// *properties* (e.g. log axis, stair facing) fall back to the block's default
// state, because LivingWorld stores only the base state name. Biomes, block
// entities, entities, and lighting are not transferred (vanilla relights on load).
package worldconvert

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"livingworld/internal/world"

	"github.com/Tnze/go-mc/nbt"
	"github.com/Tnze/go-mc/save"
	"github.com/Tnze/go-mc/save/region"
)

// encodeChunkZlib serializes an Anvil chunk as region payload: a compression-type
// byte (2 = zlib) followed by the zlib-compressed root NBT. We do this here
// instead of using go-mc's Chunk.Data, which builds a zlib writer but never
// closes it — so its compressed output is never flushed (truncated to a few
// bytes). Closing the writer is mandatory to flush the deflate stream.
func encodeChunkZlib(ch save.Chunk) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(2)
	zw := zlib.NewWriter(&buf)
	if err := nbt.NewEncoder(zw).Encode(ch, ""); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// emptyCompound returns a RawMessage holding an empty NBT compound ({}). Its Data
// is a single TagEnd byte (the compound payload after the type byte/name).
func emptyCompound() nbt.RawMessage {
	return nbt.RawMessage{Type: nbt.TagCompound, Data: []byte{0x00}}
}

// javaDataVersion is the DataVersion stamped on exported Anvil chunks.
// 4790 = Minecraft Java 26.1.2 (source: 26.1.2.jar/version.json world_version).
const javaDataVersion int32 = 4790

// minSectionY is the lowest Anvil section index for a -64..319 world
// (minY −64 / 16 = −4); LivingWorld section index i maps to Anvil Y = i + minSectionY.
const minSectionY = -4

// regionChunks is the number of chunks in one LivingWorld region (.lwr) file: 32×32.
const regionChunks = 32 * 32

// Stats reports how much was converted.
type Stats struct {
	Chunks   int
	Sections int
}

// bitsFor returns the bits-per-entry for an Anvil palette of paletteLen entries
// (smallest b with 2^b >= paletteLen, minimum 4) — the 1.16+ section encoding.
func bitsFor(paletteLen int) int {
	bits := 4
	for (1 << bits) < paletteLen {
		bits++
	}
	return bits
}

// ImportJavaWorld reads a vanilla Java world (srcDir containing region/*.mca) and
// writes its blocks into a LivingWorld world directory (dstDir). The source is
// opened read-only and never modified.
func ImportJavaWorld(srcDir, dstDir string) (Stats, error) {
	var st Stats
	files, _ := filepath.Glob(filepath.Join(srcDir, "region", "r.*.*.mca"))
	if len(files) == 0 {
		return st, fmt.Errorf("no Anvil region files (region/r.*.*.mca) under %s", srcDir)
	}
	storage, err := world.NewRegionStorage(dstDir)
	if err != nil {
		return st, err
	}
	defer storage.Close()

	for _, path := range files {
		f, err := os.Open(path) // read-only: non-destructive to the source world
		if err != nil {
			return st, err
		}
		reg, err := region.Load(f)
		if err != nil {
			f.Close()
			return st, fmt.Errorf("load region %s: %w", filepath.Base(path), err)
		}
		for x := 0; x < 32; x++ {
			for z := 0; z < 32; z++ {
				if !reg.ExistSector(x, z) {
					continue
				}
				data, err := reg.ReadSector(x, z)
				if err != nil {
					continue // skip unreadable chunk, keep going
				}
				var ch save.Chunk
				if err := ch.Load(data); err != nil {
					continue
				}
				living := world.NewChunk()
				sections := anvilToLiving(&ch, living)
				if sections == 0 {
					continue
				}
				if err := storage.SaveChunk(int(ch.XPos), int(ch.ZPos), living); err != nil {
					f.Close()
					return st, err
				}
				st.Chunks++
				st.Sections += sections
			}
		}
		f.Close()
	}
	if err := storage.Flush(); err != nil {
		return st, err
	}
	return st, nil
}

// anvilToLiving copies block data from a parsed Anvil chunk into a LivingWorld
// chunk, returning the number of non-empty sections written.
func anvilToLiving(ch *save.Chunk, living *world.Chunk) int {
	written := 0
	for si := range ch.Sections {
		sec := &ch.Sections[si]
		pal := sec.BlockStates.Palette
		if len(pal) == 0 {
			continue
		}
		base := int(sec.Y) - minSectionY // LivingWorld section index
		if base < 0 || base >= world.SectionsPerChunk {
			continue // padding/edge sections outside the -64..319 range
		}
		ids := make([]int32, len(pal))
		for i, bs := range pal {
			ids[i] = world.StateID(bs.Name) // name -> global state ID (default props)
		}
		nonAir := false
		if len(pal) == 1 {
			if ids[0] == world.AirID {
				continue
			}
			blk := world.BlockByID(ids[0])
			for p := 0; p < 4096; p++ {
				living.SetBlock(p&15, base*16+(p>>8)&15, (p>>4)&15, blk)
			}
			nonAir = true
		} else {
			bits := bitsFor(len(pal))
			epl := 64 / bits
			mask := uint64(1<<bits) - 1
			data := sec.BlockStates.Data
			for p := 0; p < 4096; p++ {
				li := p / epl
				if li >= len(data) {
					break
				}
				idx := int((data[li] >> uint((p%epl)*bits)) & mask)
				if idx >= len(ids) || ids[idx] == world.AirID {
					continue
				}
				living.SetBlock(p&15, base*16+(p>>8)&15, (p>>4)&15, world.BlockByID(ids[idx]))
				nonAir = true
			}
		}
		if nonAir {
			written++
		}
	}
	return written
}

// ExportJavaWorld reads a LivingWorld world (srcDir containing region/r.*.*.lwr)
// and writes a vanilla Java world (dstDir/region/*.mca) that vanilla 26.1.x can
// open. Heightmaps and lighting are omitted; vanilla recomputes them on load.
func ExportJavaWorld(srcDir, dstDir string) (Stats, error) {
	var st Stats
	files, _ := filepath.Glob(filepath.Join(srcDir, "region", "r.*.*.lwr"))
	if len(files) == 0 {
		return st, fmt.Errorf("no LivingWorld region files (region/r.*.*.lwr) under %s", srcDir)
	}
	src, err := world.NewRegionStorage(srcDir)
	if err != nil {
		return st, err
	}
	defer src.Close()
	regionDir := filepath.Join(dstDir, "region")
	if err := os.MkdirAll(regionDir, 0o755); err != nil {
		return st, err
	}

	regions := map[[2]int]*region.Region{}
	defer func() {
		for _, r := range regions {
			_ = r.PadToFullSector()
			_ = r.Close()
		}
	}()
	regionFor := func(cx, cz int) (*region.Region, error) {
		rx, rz := region.At(cx, cz)
		if r, ok := regions[[2]int{rx, rz}]; ok {
			return r, nil
		}
		p := filepath.Join(regionDir, fmt.Sprintf("r.%d.%d.mca", rx, rz))
		var r *region.Region
		var err error
		if _, e := os.Stat(p); e == nil {
			r, err = region.Open(p)
		} else {
			r, err = region.Create(p)
		}
		if err != nil {
			return nil, err
		}
		regions[[2]int{rx, rz}] = r
		return r, nil
	}

	for _, path := range files {
		rx, rz, ok := parseRegionCoord(filepath.Base(path))
		if !ok {
			continue
		}
		// A region holds 32×32 chunks; localIndex li = (cz&31)<<5 | (cx&31).
		for li := 0; li < regionChunks; li++ {
			cx := rx*32 + (li & 31)
			cz := rz*32 + ((li >> 5) & 31)
			living, ok, err := src.LoadChunk(cx, cz)
			if err != nil || !ok {
				continue
			}
			ch, sections := livingToAnvil(living, cx, cz)
			if sections == 0 {
				continue
			}
			data, err := encodeChunkZlib(ch)
			if err != nil {
				return st, err
			}
			reg, err := regionFor(cx, cz)
			if err != nil {
				return st, err
			}
			lx, lz := region.In(cx, cz)
			if err := reg.WriteSector(lx, lz, data); err != nil {
				return st, err
			}
			st.Chunks++
			st.Sections += sections
		}
	}
	return st, nil
}

// livingToAnvil builds an Anvil chunk from a LivingWorld chunk, returning the
// number of non-empty sections.
func livingToAnvil(living *world.Chunk, cx, cz int) (save.Chunk, int) {
	ch := save.Chunk{
		DataVersion: javaDataVersion,
		XPos:        int32(cx),
		ZPos:        int32(cz),
		YPos:        minSectionY,
		Status:      "minecraft:full",
		// go-mc's encoder rejects a zero-value (Type 0) RawMessage, and these
		// fields are always written (no omitempty). Empty compounds encode fine
		// and load leniently in vanilla (CompoundTag.getList/getCompound return
		// empty defaults on a type mismatch, so "no ticks/structures" is honoured).
		BlockTicks:     emptyCompound(),
		FluidTicks:     emptyCompound(),
		PostProcessing: emptyCompound(),
		Structures:     emptyCompound(),
	}
	written := 0
	for i := 0; i < world.SectionsPerChunk; i++ {
		sec, nonEmpty := livingSectionToAnvil(living, i)
		ch.Sections = append(ch.Sections, sec)
		if nonEmpty {
			written++
		}
	}
	return ch, written
}

// livingSectionToAnvil encodes one LivingWorld section as an Anvil section with a
// bit-packed block-state palette and a single-entry plains biome palette.
func livingSectionToAnvil(living *world.Chunk, i int) (save.Section, bool) {
	idxByName := map[string]int{}
	var palette []save.BlockState
	indices := make([]int, 4096)
	nonAir := false
	for p := 0; p < 4096; p++ {
		id := living.GetBlock(p&15, i*16+(p>>8)&15, (p>>4)&15).ID()
		if id != world.AirID {
			nonAir = true
		}
		name := world.StateName(id)
		j, ok := idxByName[name]
		if !ok {
			j = len(palette)
			idxByName[name] = j
			palette = append(palette, save.BlockState{Name: name, Properties: emptyCompound()})
		}
		indices[p] = j
	}
	sec := save.Section{Y: int8(i + minSectionY)}
	sec.BlockStates.Palette = palette
	if len(palette) > 1 {
		bits := bitsFor(len(palette))
		epl := 64 / bits
		data := make([]uint64, (4096+epl-1)/epl)
		for p := 0; p < 4096; p++ {
			data[p/epl] |= uint64(indices[p]) << uint((p%epl)*bits)
		}
		sec.BlockStates.Data = data
	}
	sec.Biomes.Palette = []save.BiomeState{"minecraft:plains"}
	return sec, nonAir
}

// parseRegionCoord extracts (rx, rz) from a LivingWorld region filename
// "r.<rx>.<rz>.lwr".
func parseRegionCoord(name string) (int, int, bool) {
	if !strings.HasPrefix(name, "r.") || !strings.HasSuffix(name, ".lwr") {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(name, "r."), ".lwr"), ".")
	if len(parts) != 2 {
		return 0, 0, false
	}
	rx, err1 := strconv.Atoi(parts[0])
	rz, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return rx, rz, true
}
