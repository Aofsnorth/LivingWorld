package world

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// Storage persists and retrieves chunks for a single world.
//
// SaveChunk may buffer writes; callers must call Flush to guarantee durability
// (region-based backends batch a whole region into one file on Flush).
type Storage interface {
	// LoadChunk returns the stored chunk and ok=true if present on disk.
	LoadChunk(cx, cz int) (c *Chunk, ok bool, err error)
	// SaveChunk records a chunk to be persisted (possibly buffered until Flush).
	SaveChunk(cx, cz int, c *Chunk) error
	// Flush writes any buffered data to disk.
	Flush() error
	Close() error
}

// NopStorage is a Storage that persists nothing (in-memory worlds).
type NopStorage struct{}

func (NopStorage) LoadChunk(cx, cz int) (*Chunk, bool, error) { return nil, false, nil }
func (NopStorage) SaveChunk(cx, cz int, c *Chunk) error       { return nil }
func (NopStorage) Flush() error                               { return nil }
func (NopStorage) Close() error                               { return nil }

// DiskStorage stores each chunk as a gzip-compressed file under
// <dir>/region/c.<cx>.<cz>.bin. Per-chunk files keep the implementation simple
// and crash-safe (atomic temp-file rename); it can be swapped for a region-file
// backend later without touching callers.
type DiskStorage struct {
	dir string
}

// NewDiskStorage creates (if needed) the world's region directory.
func NewDiskStorage(dir string) (*DiskStorage, error) {
	region := filepath.Join(dir, "region")
	if err := os.MkdirAll(region, 0o755); err != nil {
		return nil, fmt.Errorf("create world dir %s: %w", region, err)
	}
	return &DiskStorage{dir: region}, nil
}

func (d *DiskStorage) path(cx, cz int) string {
	return filepath.Join(d.dir, fmt.Sprintf("c.%d.%d.bin", cx, cz))
}

func (d *DiskStorage) LoadChunk(cx, cz int) (*Chunk, bool, error) {
	f, err := os.Open(d.path(cx, cz))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, false, fmt.Errorf("gzip open chunk (%d,%d): %w", cx, cz, err)
	}
	defer zr.Close()

	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, false, fmt.Errorf("read chunk (%d,%d): %w", cx, cz, err)
	}
	c, err := DecodeChunk(raw)
	if err != nil {
		return nil, false, err
	}
	return c, true, nil
}

func (d *DiskStorage) SaveChunk(cx, cz int, c *Chunk) error {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(c.Encode()); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	final := d.path(cx, cz)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// Flush is a no-op: DiskStorage writes each chunk immediately in SaveChunk.
func (d *DiskStorage) Flush() error { return nil }

func (d *DiskStorage) Close() error { return nil }
