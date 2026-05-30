package persistence

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SectionBlocks is the number of block states in one 16x16x16 chunk section.
const SectionBlocks = 4096

const chunkVersion byte = 1

// chunkHeaderLen = version(1) + cx(4) + cz(4) + sectionCount(2) + mask(4).
const chunkHeaderLen = 15

// ChunkData is the persistable block grid of one chunk column. Sections is
// indexed by section Y (0 = lowest); a nil entry is an empty (all-air) section
// that costs nothing on disk. Each non-nil section holds exactly SectionBlocks
// canonical block-state ids. Only blocks are persisted; light/biomes regenerate
// on load, keeping files small and gzip-friendly.
type ChunkData struct {
	CX, CZ   int32
	Sections [][]int32
}

// Encode serializes the block grid to a compact binary blob.
//
// Layout (little-endian):
//
//	byte    version
//	int32   CX
//	int32   CZ
//	uint16  section count N
//	uint32  bitmask of non-empty sections (bit i => section i follows)
//	[]int32 SectionBlocks ids per non-empty section, ascending section order
func (cd *ChunkData) Encode() []byte {
	var buf bytes.Buffer
	var b4 [4]byte
	buf.WriteByte(chunkVersion)
	binary.LittleEndian.PutUint32(b4[:], uint32(cd.CX))
	buf.Write(b4[:])
	binary.LittleEndian.PutUint32(b4[:], uint32(cd.CZ))
	buf.Write(b4[:])

	var b2 [2]byte
	binary.LittleEndian.PutUint16(b2[:], uint16(len(cd.Sections)))
	buf.Write(b2[:])

	var mask uint32
	for i, sec := range cd.Sections {
		if len(sec) == SectionBlocks {
			mask |= 1 << uint(i)
		}
	}
	binary.LittleEndian.PutUint32(b4[:], mask)
	buf.Write(b4[:])

	for i, sec := range cd.Sections {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		for _, id := range sec {
			binary.LittleEndian.PutUint32(b4[:], uint32(id))
			buf.Write(b4[:])
		}
	}
	return buf.Bytes()
}

// DecodeChunk reconstructs ChunkData from a blob produced by Encode.
func DecodeChunk(data []byte) (*ChunkData, error) {
	if len(data) < chunkHeaderLen {
		return nil, fmt.Errorf("persistence: chunk data too short: %d bytes", len(data))
	}
	if data[0] != chunkVersion {
		return nil, fmt.Errorf("persistence: unsupported chunk version %d", data[0])
	}
	cd := &ChunkData{
		CX: int32(binary.LittleEndian.Uint32(data[1:5])),
		CZ: int32(binary.LittleEndian.Uint32(data[5:9])),
	}
	n := int(binary.LittleEndian.Uint16(data[9:11]))
	mask := binary.LittleEndian.Uint32(data[11:15])
	cd.Sections = make([][]int32, n)

	pos := chunkHeaderLen
	for i := 0; i < n; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		end := pos + SectionBlocks*4
		if end > len(data) {
			return nil, fmt.Errorf("persistence: chunk truncated at section %d", i)
		}
		sec := make([]int32, SectionBlocks)
		for j := range sec {
			sec[j] = int32(binary.LittleEndian.Uint32(data[pos:]))
			pos += 4
		}
		cd.Sections[i] = sec
	}
	return cd, nil
}

func (s *Store) chunkPath(cx, cz int32) string {
	return filepath.Join(s.dir, "region", fmt.Sprintf("c.%d.%d.gz", cx, cz))
}

// SaveChunk writes a chunk's block data as a gzipped blob.
func (s *Store) SaveChunk(cd *ChunkData) error {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(cd.Encode()); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return writeAtomic(s.chunkPath(cd.CX, cd.CZ), buf.Bytes())
}

// LoadChunk reads a chunk; ok=false if it has never been saved.
func (s *Store) LoadChunk(cx, cz int32) (*ChunkData, bool, error) {
	f, err := os.Open(s.chunkPath(cx, cz))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, false, fmt.Errorf("persistence: gzip open chunk (%d,%d): %w", cx, cz, err)
	}
	defer zr.Close()

	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, false, fmt.Errorf("persistence: read chunk (%d,%d): %w", cx, cz, err)
	}
	cd, err := DecodeChunk(raw)
	if err != nil {
		return nil, false, err
	}
	return cd, true, nil
}
