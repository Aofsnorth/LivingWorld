package world

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
