package world

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

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
