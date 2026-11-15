package world

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// regionMagicV2 identifies the GoCraft/Anvil-inspired per-chunk zlib region
// format ("LWR2"). Unlike legacy "LWR1" (which wrapped the entire 32x32 region
// in a single outer gzip stream and kept uncompressed chunk buffers in RAM),
// "LWR2" stores each chunk as an independent zlib-compressed blob both in
// memory (rf.chunks[li]) and on disk. Saving one chunk only compresses that
// single chunk, and flushing a region writes pre-compressed blobs directly
// without re-compressing untouched chunks.
const regionMagicV2 = "LWR2"

func isZlibChunkBlob(blob []byte) bool {
	return len(blob) >= 2 && blob[0] == 0x78 && (uint16(blob[0])<<8|uint16(blob[1]))%31 == 0
}

func compressChunkBlob(raw []byte) ([]byte, error) {
	if isZlibChunkBlob(raw) {
		out := make([]byte, len(raw))
		copy(out, raw)
		return out, nil
	}
	var buf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&buf, zlib.DefaultCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(raw); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressChunkBlob(blob []byte) ([]byte, error) {
	if !isZlibChunkBlob(blob) {
		return blob, nil
	}
	zr, err := zlib.NewReader(bytes.NewReader(blob))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
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
	if len(raw) >= 4 && string(raw[:4]) == regionMagicV2 {
		if err := decodeRegionV2(raw, rf); err != nil {
			r.quarantineRegion(rp, fmt.Errorf("decode v2: %w", err))
			fresh := &regionFile{chunks: make(map[uint16][]byte), loaded: true}
			r.regions[rp] = fresh
			return fresh, nil
		}
		return rf, nil
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
	// Mark legacy LWR1 region dirty so the next Flush upgrades it to LWR2 on disk.
	rf.dirty = true
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
	if len(data) < 8 {
		return fmt.Errorf("bad region magic")
	}
	magic := string(data[:4])
	if magic == regionMagicV2 {
		return decodeRegionV2(data, rf)
	}
	if magic != regionMagic {
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
		compressed, err := compressChunkBlob(data[pos : pos+int(e.n)])
		if err != nil {
			return fmt.Errorf("compress legacy chunk %d: %w", e.li, err)
		}
		rf.chunks[e.li] = compressed
		pos += int(e.n)
	}
	return nil
}

func decodeRegionV2(data []byte, rf *regionFile) error {
	if len(data) < 8 || string(data[:4]) != regionMagicV2 {
		return fmt.Errorf("bad region v2 magic")
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
			return fmt.Errorf("region v2 index truncated")
		}
		entries[i].li = binary.LittleEndian.Uint16(data[pos:])
		entries[i].n = binary.LittleEndian.Uint32(data[pos+2:])
		pos += 6
	}
	for _, e := range entries {
		if pos+int(e.n) > len(data) {
			return fmt.Errorf("region v2 blob truncated")
		}
		compressed, err := compressChunkBlob(data[pos : pos+int(e.n)])
		if err != nil {
			return fmt.Errorf("normalize v2 chunk %d: %w", e.li, err)
		}
		rf.chunks[e.li] = compressed
		pos += int(e.n)
	}
	return nil
}

func encodeRegion(rf *regionFile) []byte {
	return encodeRegionWithMagic(rf, regionMagicV2)
}

func encodeRegionWithMagic(rf *regionFile, magic string) []byte {
	// Capture a single, deterministic iteration order sorted by local index so
	// the index table and the blob section always line up reproducibly.
	type entry struct {
		li   uint16
		blob []byte
	}
	entries := make([]entry, 0, len(rf.chunks))
	for li, blob := range rf.chunks {
		entries = append(entries, entry{li, blob})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].li < entries[j].li })

	var buf bytes.Buffer
	buf.WriteString(magic)
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
	raw, err := decompressChunkBlob(blob)
	if err != nil {
		log.Printf("[World] chunk (%d,%d) in region %v failed to decompress: %v (regenerating)", cx, cz, rp, err)
		delete(rf.chunks, li)
		rf.dirty = true
		return nil, false, nil
	}
	c, err := DecodeChunk(raw)
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
	compressed, err := compressChunkBlob(c.Encode())
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rf, err := r.ensureLoaded(rp)
	if err != nil {
		return err
	}
	rf.chunks[li] = compressed
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
	payload := encodeRegionWithMagic(rf, regionMagicV2)
	final := r.path(rp)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func (r *RegionStorage) Close() error { return r.Flush() }

const (
	anvilSectorBytes       = 4096
	anvilHeaderBytes       = anvilSectorBytes * 2
	anvilCompressionGzip   = 1
	anvilCompressionZlib   = 2
	anvilCompressionRaw    = 3
)

// ExportAnvilSectorRegion serializes region (rx, rz) into an Anvil-compatible
// 4 KiB sector-aligned region container:
//   - Sector 0 (0..4095): 1024 x 4-byte location entries (24-bit big-endian sector offset + 8-bit sector count)
//   - Sector 1 (4096..8191): 1024 x 4-byte big-endian Unix modification timestamps
//   - Sectors >= 2: 4-byte big-endian payload length + 1-byte compression type (2 = zlib) + zlib stream, padded to 4096-byte boundary.
func (r *RegionStorage) ExportAnvilSectorRegion(rx, rz int) ([]byte, error) {
	rp := regionPos{rx, rz}
	r.mu.Lock()
	defer r.mu.Unlock()
	rf, err := r.ensureLoaded(rp)
	if err != nil {
		return nil, err
	}

	type entry struct {
		li   uint16
		blob []byte
	}
	entries := make([]entry, 0, len(rf.chunks))
	for li, blob := range rf.chunks {
		if li >= 1024 || len(blob) == 0 {
			continue
		}
		zblob, err := compressChunkBlob(blob)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry{li: li, blob: zblob})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].li < entries[j].li })

	var out bytes.Buffer
	header := make([]byte, anvilHeaderBytes)
	out.Write(header)

	nowUnix := uint32(time.Now().Unix())
	curSector := uint32(2)

	for _, e := range entries {
		streamLen := uint32(1 + len(e.blob))
		totalBytes := 4 + int(streamLen)
		sectors := uint32((totalBytes + anvilSectorBytes - 1) / anvilSectorBytes)
		if sectors == 0 || sectors > 255 {
			return nil, fmt.Errorf("chunk %d requires invalid sector count %d", e.li, sectors)
		}

		locOffset := int(e.li) * 4
		header[locOffset] = byte(curSector >> 16)
		header[locOffset+1] = byte(curSector >> 8)
		header[locOffset+2] = byte(curSector)
		header[locOffset+3] = byte(sectors)

		tsOffset := anvilSectorBytes + int(e.li)*4
		binary.BigEndian.PutUint32(header[tsOffset:tsOffset+4], nowUnix)

		var lenBuf [4]byte
		binary.BigEndian.PutUint32(lenBuf[:], streamLen)
		out.Write(lenBuf[:])
		out.WriteByte(anvilCompressionZlib)
		out.Write(e.blob)

		pad := int(sectors)*anvilSectorBytes - totalBytes
		if pad > 0 {
			out.Write(make([]byte, pad))
		}
		curSector += sectors
	}

	raw := out.Bytes()
	copy(raw[:anvilHeaderBytes], header)
	return raw, nil
}

// ImportAnvilSectorRegion parses an Anvil-compatible 4 KiB sector-aligned region
// image and loads all valid chunk payloads into region (rx, rz), returning the
// number of imported chunks.
func (r *RegionStorage) ImportAnvilSectorRegion(rx, rz int, mcaData []byte) (int, error) {
	if len(mcaData) < anvilHeaderBytes || len(mcaData)%anvilSectorBytes != 0 {
		return 0, fmt.Errorf("invalid anvil region size %d (must be >= 8192 and multiple of 4096)", len(mcaData))
	}
	rp := regionPos{rx, rz}
	r.mu.Lock()
	defer r.mu.Unlock()
	rf, err := r.ensureLoaded(rp)
	if err != nil {
		return 0, err
	}

	imported := 0
	for li := uint16(0); li < 1024; li++ {
		locOffset := int(li) * 4
		secOffset := (uint32(mcaData[locOffset]) << 16) | (uint32(mcaData[locOffset+1]) << 8) | uint32(mcaData[locOffset+2])
		secCount := uint32(mcaData[locOffset+3])
		if secOffset == 0 || secCount == 0 {
			continue
		}
		byteOffset := int(secOffset) * anvilSectorBytes
		maxBytes := int(secCount) * anvilSectorBytes
		if byteOffset < anvilHeaderBytes || byteOffset+5 > len(mcaData) || byteOffset+maxBytes > len(mcaData) {
			return imported, fmt.Errorf("chunk %d sector out of bounds (offset=%d count=%d)", li, secOffset, secCount)
		}
		streamLen := int(binary.BigEndian.Uint32(mcaData[byteOffset : byteOffset+4]))
		if streamLen <= 1 || 4+streamLen > maxBytes {
			return imported, fmt.Errorf("chunk %d invalid stream length %d", li, streamLen)
		}
		compType := mcaData[byteOffset+4]
		payload := mcaData[byteOffset+5 : byteOffset+4+streamLen]

		var zblob []byte
		switch compType {
		case anvilCompressionZlib:
			raw, err := decompressChunkBlob(payload)
			if err != nil {
				return imported, fmt.Errorf("chunk %d zlib verify failed: %w", li, err)
			}
			zblob, err = compressChunkBlob(raw)
			if err != nil {
				return imported, err
			}
		case anvilCompressionGzip:
			gr, err := gzip.NewReader(bytes.NewReader(payload))
			if err != nil {
				return imported, fmt.Errorf("chunk %d gzip header failed: %w", li, err)
			}
			raw, err := io.ReadAll(gr)
			_ = gr.Close()
			if err != nil {
				return imported, fmt.Errorf("chunk %d gzip read failed: %w", li, err)
			}
			zblob, err = compressChunkBlob(raw)
			if err != nil {
				return imported, err
			}
		case anvilCompressionRaw:
			var err error
			zblob, err = compressChunkBlob(payload)
			if err != nil {
				return imported, err
			}
		default:
			return imported, fmt.Errorf("chunk %d unsupported compression type %d", li, compType)
		}

		rf.chunks[li] = zblob
		rf.dirty = true
		imported++
	}
	return imported, nil
}
