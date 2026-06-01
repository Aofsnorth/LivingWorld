package world

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Region files group a 32×32 block of chunks (1024 chunks) into one file, the
// way Minecraft's Anvil format does. A normal play area becomes a handful of
// files instead of hundreds of per-chunk files.
//
// On-disk layout: the whole region is gzip-compressed. Decompressed payload:
//
//	"LWR1"                       magic (4 bytes)
//	uint32                       chunk count
//	count × { uint16 local, uint32 len }   index table
//	count × []byte               raw Chunk.Encode() blobs (same order as index)
//
// Writes are buffered per region and committed (whole-file rewrite, atomic
// temp+rename) on Flush. Because the whole region is held in memory once touched,
// rewriting preserves chunks that aren't currently loaded.
const (
	regionShift = 5 // 32 = 1<<5 chunks per region axis
	regionWidth = 1 << regionShift
	regionMask  = regionWidth - 1
	regionMagic = "LWR1"
)

type regionPos struct{ rx, rz int }

type regionFile struct {
	chunks map[uint16][]byte // localIndex -> raw Chunk.Encode() bytes
	dirty  bool
	loaded bool
}

// RegionStorage is a region-file Storage backend (the default for disk worlds).
type RegionStorage struct {
	dir     string
	mu      sync.Mutex
	regions map[regionPos]*regionFile
}

// NewRegionStorage creates (if needed) the world's region directory.
func NewRegionStorage(dir string) (*RegionStorage, error) {
	region := filepath.Join(dir, "region")
	if err := os.MkdirAll(region, 0o755); err != nil {
		return nil, fmt.Errorf("create world dir %s: %w", region, err)
	}
	return &RegionStorage{dir: region, regions: make(map[regionPos]*regionFile)}, nil
}

// regionOf maps a chunk coordinate to its region and local index. Arithmetic
// shift / mask give the correct floor-division behaviour for negatives.
func regionOf(cx, cz int) (regionPos, uint16) {
	rp := regionPos{cx >> regionShift, cz >> regionShift}
	li := uint16((cz&regionMask)<<regionShift | (cx & regionMask))
	return rp, li
}

func (r *RegionStorage) path(rp regionPos) string {
	return filepath.Join(r.dir, fmt.Sprintf("r.%d.%d.lwr", rp.rx, rp.rz))
}

// ensureLoaded reads a region file (if present) into memory. Caller holds r.mu.
//
// On any error reading or decoding the file (other than ENOENT, which means
// "this region hasn't been generated yet"), the file is moved to a sibling
// `quarantine/` subdir so the next call starts fresh. The in-memory rf we
// return is empty (a fresh region), not nil — LoadChunk then returns ok=false
// for any specific chunk and the world regenerates the surface instead of
// crashing (Phase 3: corrupt-chunk quarantine + recovery, Master_Plan §6).
func (r *RegionStorage) ensureLoaded(rp regionPos) (*regionFile, error) {
	if rf := r.regions[rp]; rf != nil && rf.loaded {
		return rf, nil
	}
	rf := &regionFile{chunks: make(map[uint16][]byte), loaded: true}
	r.regions[rp] = rf

	// Read the entire file into memory FIRST, then close it. Quarantining a
	// file that's still open fails on Windows (file-in-use). Reading the
	// bytes up front is cheap (region files are gzip-compressed chunks) and
	// lets us close the handle before we attempt a rename.
	raw, err := os.ReadFile(r.path(rp))
	if err != nil {
		if os.IsNotExist(err) {
			return rf, nil // brand-new region
		}
		// Some other filesystem error (permission, IO, ...). Quarantine and
		// return a fresh region so the world can keep going.
		r.quarantineRegion(rp, fmt.Errorf("read: %w", err))
		fresh := &regionFile{chunks: make(map[uint16][]byte), loaded: true}
		r.regions[rp] = fresh
		return fresh, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		r.quarantineRegion(rp, fmt.Errorf("gzip: %w", err))
		fresh := &regionFile{chunks: make(map[uint16][]byte), loaded: true}
		r.regions[rp] = fresh
		return fresh, nil
	}
	decoded, err := io.ReadAll(zr)
	zr.Close()
	if err != nil {
		r.quarantineRegion(rp, fmt.Errorf("read gz: %w", err))
		fresh := &regionFile{chunks: make(map[uint16][]byte), loaded: true}
		r.regions[rp] = fresh
		return fresh, nil
	}
	if err := decodeRegion(decoded, rf); err != nil {
		r.quarantineRegion(rp, fmt.Errorf("decode: %w", err))
		fresh := &regionFile{chunks: make(map[uint16][]byte), loaded: true}
		r.regions[rp] = fresh
		return fresh, nil
	}
	return rf, nil
}

// quarantineRegion moves the on-disk region file at rp to a sibling
// `quarantine/` directory, appending a timestamp + reason. The original
// file path is freed for the next save; the world's storage stays alive.
//
// The move is best-effort: if the move itself fails (e.g. the disk is
// read-only), we log and continue rather than escalating to a panic. The
// world always wins over a corrupt file: the in-memory state we return is
// empty, and the surface chunk will be regenerated on the next access.
func (r *RegionStorage) quarantineRegion(rp regionPos, reason error) {
	src := r.path(rp)
	if _, err := os.Stat(src); err != nil {
		// Already gone; nothing to move.
		return
	}
	quarantineDir := filepath.Join(r.dir, "quarantine")
	if err := os.MkdirAll(quarantineDir, 0o755); err != nil {
		log.Printf("[World] quarantine: mkdir %s: %v (file left in place; surface will regenerate anyway)", quarantineDir, err)
		return
	}
	stamp := time.Now().UTC().Format("20060102T150405")
	base := filepath.Base(src)
	dst := filepath.Join(quarantineDir, fmt.Sprintf("%s.%s.bad", stamp, base))
	if err := os.Rename(src, dst); err != nil {
		log.Printf("[World] quarantine: rename %s -> %s: %v", src, dst, err)
		return
	}
	log.Printf("[World] quarantined region %v: %v (moved to %s)", rp, reason, dst)
}

func decodeRegion(data []byte, rf *regionFile) error {
	if len(data) < 8 || string(data[:4]) != regionMagic {
		return fmt.Errorf("bad region magic")
	}
	count := binary.LittleEndian.Uint32(data[4:8])
	pos := 8
	type idx struct {
		li uint16
		n  uint32
	}
	entries := make([]idx, count)
	for i := uint32(0); i < count; i++ {
		if pos+6 > len(data) {
			return fmt.Errorf("region index truncated")
		}
		entries[i].li = binary.LittleEndian.Uint16(data[pos:])
		entries[i].n = binary.LittleEndian.Uint32(data[pos+2:])
		pos += 6
	}
	for _, e := range entries {
		if pos+int(e.n) > len(data) {
			return fmt.Errorf("region blob truncated")
		}
		blob := make([]byte, e.n)
		copy(blob, data[pos:pos+int(e.n)])
		rf.chunks[e.li] = blob
		pos += int(e.n)
	}
	return nil
}

func encodeRegion(rf *regionFile) []byte {
	// Capture a single, stable iteration order: the index table and the blob
	// section MUST line up, and two separate `range` passes over a map are not
	// guaranteed to agree on order.
	type entry struct {
		li   uint16
		blob []byte
	}
	entries := make([]entry, 0, len(rf.chunks))
	for li, blob := range rf.chunks {
		entries = append(entries, entry{li, blob})
	}

	var buf bytes.Buffer
	buf.WriteString(regionMagic)
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], uint32(len(entries)))
	buf.Write(u32[:])

	var u16 [2]byte
	for _, e := range entries {
		binary.LittleEndian.PutUint16(u16[:], e.li)
		buf.Write(u16[:])
		binary.LittleEndian.PutUint32(u32[:], uint32(len(e.blob)))
		buf.Write(u32[:])
	}
	for _, e := range entries {
		buf.Write(e.blob)
	}
	return buf.Bytes()
}

func (r *RegionStorage) LoadChunk(cx, cz int) (*Chunk, bool, error) {
	rp, li := regionOf(cx, cz)
	r.mu.Lock()
	defer r.mu.Unlock()
	rf, err := r.ensureLoaded(rp)
	if err != nil {
		return nil, false, err
	}
	blob, ok := rf.chunks[li]
	if !ok {
		return nil, false, nil
	}
	c, err := DecodeChunk(blob)
	if err != nil {
		// Chunk-level corruption inside an otherwise-valid region file.
		// Drop just this bad blob from the in-memory map so the next
		// SaveChunk writes a fresh version; don't kill the rest of the
		// region's good chunks.
		log.Printf("[World] chunk (%d,%d) in region %v failed to decode: %v (regenerating)", cx, cz, rp, err)
		delete(rf.chunks, li)
		rf.dirty = true
		return nil, false, nil
	}
	return c, true, nil
}

func (r *RegionStorage) SaveChunk(cx, cz int, c *Chunk) error {
	rp, li := regionOf(cx, cz)
	r.mu.Lock()
	defer r.mu.Unlock()
	rf, err := r.ensureLoaded(rp)
	if err != nil {
		return err
	}
	rf.chunks[li] = c.Encode()
	rf.dirty = true
	return nil
}

func (r *RegionStorage) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for rp, rf := range r.regions {
		if !rf.dirty {
			continue
		}
		if err := r.writeRegion(rp, rf); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		rf.dirty = false
	}
	return firstErr
}

func (r *RegionStorage) writeRegion(rp regionPos, rf *regionFile) error {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(encodeRegion(rf)); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	final := r.path(rp)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func (r *RegionStorage) Close() error { return r.Flush() }
