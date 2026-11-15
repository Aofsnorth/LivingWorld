package world

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"os"
	"testing"
)

func TestRegionStorageLWR2PerChunkZlibAndLegacyLWR1Upgrade(t *testing.T) {
	dir := t.TempDir()
	rs, err := NewRegionStorage(dir)
	if err != nil {
		t.Fatalf("NewRegionStorage: %v", err)
	}

	c1 := NewChunk()
	stone := BlockByID(StateID("minecraft:stone"))
	c1.SetBlock(4, 64, 4, stone)

	if err := rs.SaveChunk(2, 3, c1); err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}

	// Verify in-memory blob is per-chunk zlib compressed (much smaller than raw encode).
	rp, li := regionOf(2, 3)
	inMemBlob := rs.regions[rp].chunks[li]
	if !isZlibChunkBlob(inMemBlob) {
		t.Fatalf("expected in-memory chunk blob to be zlib-compressed")
	}
	rawEncoded := c1.Encode()
	if len(inMemBlob) >= len(rawEncoded) {
		t.Fatalf("expected zlib blob (%d bytes) to be smaller than raw encoded chunk (%d bytes)", len(inMemBlob), len(rawEncoded))
	}

	if err := rs.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Verify on-disk file uses LWR2 header directly without outer gzip wrapper.
	diskBytes, err := os.ReadFile(rs.path(rp))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(diskBytes) < 8 || string(diskBytes[:4]) != regionMagicV2 {
		t.Fatalf("expected on-disk magic %q, got %q", regionMagicV2, string(diskBytes[:4]))
	}

	// Reload from disk in a fresh RegionStorage and verify block integrity.
	rs2, err := NewRegionStorage(dir)
	if err != nil {
		t.Fatalf("NewRegionStorage 2: %v", err)
	}
	loaded, ok, err := rs2.LoadChunk(2, 3)
	if err != nil || !ok || loaded == nil {
		t.Fatalf("LoadChunk failed: ok=%v err=%v", ok, err)
	}
	if loaded.GetBlock(4, 64, 4).ID() != stone.ID() {
		t.Fatalf("expected stone at (4,64,4), got %d", loaded.GetBlock(4, 64, 4).ID())
	}

	// Now synthesize a legacy LWR1 outer-gzipped region file at region (1, 1) (chunk 32, 32)
	// and verify transparent load + automatic upgrade to LWR2 on Flush.
	legacyChunk := NewChunk()
	dirt := BlockByID(StateID("minecraft:dirt"))
	legacyChunk.SetBlock(1, 70, 1, dirt)
	legacyRaw := legacyChunk.Encode()

	legacyRP, legacyLI := regionOf(32, 32)
	var lwr1Buf bytes.Buffer
	lwr1Buf.WriteString(regionMagic)
	var u32 [4]byte
	var u16 [2]byte
	binary.LittleEndian.PutUint32(u32[:], 1)
	lwr1Buf.Write(u32[:])
	binary.LittleEndian.PutUint16(u16[:], legacyLI)
	lwr1Buf.Write(u16[:])
	binary.LittleEndian.PutUint32(u32[:], uint32(len(legacyRaw)))
	lwr1Buf.Write(u32[:])
	lwr1Buf.Write(legacyRaw)

	var gzBuf bytes.Buffer
	zw := gzip.NewWriter(&gzBuf)
	if _, err := zw.Write(lwr1Buf.Bytes()); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	if err := os.WriteFile(rs2.path(legacyRP), gzBuf.Bytes(), 0o644); err != nil {
		t.Fatalf("write legacy LWR1 file: %v", err)
	}

	rs3, err := NewRegionStorage(dir)
	if err != nil {
		t.Fatalf("NewRegionStorage 3: %v", err)
	}
	upgradedChunk, ok, err := rs3.LoadChunk(32, 32)
	if err != nil || !ok || upgradedChunk == nil {
		t.Fatalf("LoadChunk legacy LWR1 failed: ok=%v err=%v", ok, err)
	}
	if upgradedChunk.GetBlock(1, 70, 1).ID() != dirt.ID() {
		t.Fatalf("expected dirt at (1,70,1), got %d", upgradedChunk.GetBlock(1, 70, 1).ID())
	}
	if err := rs3.Flush(); err != nil {
		t.Fatalf("Flush legacy upgrade: %v", err)
	}
	upgradedBytes, err := os.ReadFile(rs3.path(legacyRP))
	if err != nil {
		t.Fatalf("ReadFile upgraded: %v", err)
	}
	if len(upgradedBytes) < 8 || string(upgradedBytes[:4]) != regionMagicV2 {
		t.Fatalf("expected legacy LWR1 file to upgrade to %q on Flush, got %q", regionMagicV2, string(upgradedBytes[:4]))
	}
}

func TestAnvilSectorRegionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	rs, err := NewRegionStorage(dir)
	if err != nil {
		t.Fatalf("NewRegionStorage: %v", err)
	}

	c := NewChunk()
	stone := BlockByID(StateID("minecraft:stone"))
	c.SetBlock(3, 80, 9, stone)
	if err := rs.SaveChunk(5, 7, c); err != nil {
		t.Fatalf("SaveChunk: %v", err)
	}

	mcaData, err := rs.ExportAnvilSectorRegion(0, 0)
	if err != nil {
		t.Fatalf("ExportAnvilSectorRegion: %v", err)
	}
	if len(mcaData) < 12288 || len(mcaData)%4096 != 0 {
		t.Fatalf("expected 4KiB sector-aligned Anvil region data (>=12288 bytes), got %d", len(mcaData))
	}

	dir2 := t.TempDir()
	rs2, err := NewRegionStorage(dir2)
	if err != nil {
		t.Fatalf("NewRegionStorage 2: %v", err)
	}
	imported, err := rs2.ImportAnvilSectorRegion(0, 0, mcaData)
	if err != nil {
		t.Fatalf("ImportAnvilSectorRegion: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected 1 imported chunk, got %d", imported)
	}
	loaded, ok, err := rs2.LoadChunk(5, 7)
	if err != nil || !ok || loaded == nil {
		t.Fatalf("LoadChunk after Anvil import failed: ok=%v err=%v", ok, err)
	}
	if loaded.GetBlock(3, 80, 9).ID() != stone.ID() {
		t.Fatalf("expected stone at (3,80,9) after Anvil import, got %d", loaded.GetBlock(3, 80, 9).ID())
	}
}

func TestChunkSectionAutoAirReclamation(t *testing.T) {
	c := NewChunk()
	stone := BlockByID(StateID("minecraft:stone"))
	air := BlockByID(StateID("minecraft:air"))

	c.SetBlock(0, 64, 0, stone)
	secIdx := (64 - MinWorldHeight) >> 4
	sec := &c.sections[secIdx]
	if sec.IsEmpty() || sec.blocks == nil {
		t.Fatal("expected section for Y=64 to allocate backing slice after placing stone")
	}

	c.SetBlock(0, 64, 0, air)
	if !sec.IsEmpty() || sec.blocks != nil {
		t.Fatal("expected section for Y=64 to automatically reclaim backing slice when last non-air block is removed")
	}
}

