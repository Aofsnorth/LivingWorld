package worldconvert

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"livingworld/internal/world"

	dfworld "github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/df-mc/dragonfly/server/world/mcdb"
)

// ErrBedrockUnsupported is returned by the Bedrock export path, which is not
// implemented yet (writing a LevelDB world from LivingWorld blocks).
var ErrBedrockUnsupported = errors.New(
	"bedrock world export is not implemented yet: needs LivingWorld -> Bedrock LevelDB block-state translation")

// ImportBedrockWorld reads a vanilla Bedrock world — either a .mcworld archive or
// an already-extracted world directory (containing a LevelDB `db/` folder) — and
// writes its overworld blocks into a LivingWorld world directory (dstDir).
//
// Importing pivots on the block's namespaced name (Bedrock and Java share most
// vanilla block names), mapping it to the block's default Java state via
// world.StateID — exactly like the Java Anvil importer. Block-state properties
// (axis, facing, ...), biomes, block entities, entities and lighting are not
// transferred; unmapped names are skipped (left as air). The source is never
// modified (a .mcworld is extracted to a temp dir that is removed afterwards).
func ImportBedrockWorld(srcPath, dstDir string) (Stats, error) {
	var st Stats
	worldDir, cleanup, err := openBedrockWorldDir(srcPath)
	if err != nil {
		return st, err
	}
	defer cleanup()

	db, err := mcdb.Open(worldDir)
	if err != nil {
		return st, fmt.Errorf("open bedrock world (leveldb) at %s: %w", worldDir, err)
	}
	defer db.Close()

	storage, err := world.NewRegionStorage(dstDir)
	if err != nil {
		return st, err
	}
	defer storage.Close()

	it := db.NewColumnIterator(&mcdb.IteratorRange{Dimension: dfworld.Overworld})
	defer it.Release()
	for it.Next() {
		col := it.Column()
		if col == nil || col.Chunk == nil {
			continue
		}
		pos := it.Position()
		living := world.NewChunk()
		sections := bedrockChunkToLiving(col.Chunk, living)
		if sections == 0 {
			continue
		}
		if err := storage.SaveChunk(int(pos[0]), int(pos[1]), living); err != nil {
			return st, err
		}
		st.Chunks++
		st.Sections += sections
	}
	if err := it.Error(); err != nil {
		return st, fmt.Errorf("iterate bedrock chunks: %w", err)
	}
	if err := storage.Flush(); err != nil {
		return st, err
	}
	return st, nil
}

// bedrockChunkToLiving copies blocks from a decoded Bedrock chunk into a
// LivingWorld chunk, returning the number of 16-block layers that received at
// least one non-air block.
func bedrockChunkToLiving(c *chunk.Chunk, living *world.Chunk) int {
	r := c.Range()
	touched := map[int]bool{}
	for y := r.Min(); y <= r.Max(); y++ {
		// Bedrock and LivingWorld now share canonical world-Y (-64..319), so write
		// world-Y straight into the chunk.
		if y < world.MinWorldHeight || y >= world.MinWorldHeight+world.SectionsPerChunk*16 {
			continue
		}
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				name, _, ok := chunk.RuntimeIDToState(c.Block(uint8(x), int16(y), uint8(z), 0))
				if !ok {
					continue
				}
				id := world.StateID(name)
				if id == world.AirID {
					continue // air or a Bedrock-only name we can't map
				}
				living.SetBlock(x, y, z, world.BlockByID(id))
				touched[(y-world.MinWorldHeight)>>4] = true
			}
		}
	}
	return len(touched)
}

// openBedrockWorldDir resolves srcPath to a Bedrock world directory containing a
// LevelDB `db/` folder. A directory is used in place; a .mcworld/.zip file is
// extracted to a temporary directory (returned via cleanup for removal).
func openBedrockWorldDir(srcPath string) (dir string, cleanup func(), err error) {
	cleanup = func() {}
	info, err := os.Stat(srcPath)
	if err != nil {
		return "", cleanup, err
	}
	if info.IsDir() {
		root, err := findWorldRoot(srcPath)
		if err != nil {
			return "", cleanup, err
		}
		return root, cleanup, nil
	}
	tmp, err := os.MkdirTemp("", "lw-bedrock-*")
	if err != nil {
		return "", cleanup, err
	}
	cleanup = func() { _ = os.RemoveAll(tmp) }
	if err := unzip(srcPath, tmp); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("extract %s: %w", filepath.Base(srcPath), err)
	}
	root, err := findWorldRoot(tmp)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return root, cleanup, nil
}

// findWorldRoot locates the directory holding the LevelDB `db/` folder, either at
// base or one level below it (the common .mcworld layouts).
func findWorldRoot(base string) (string, error) {
	if _, err := os.Stat(filepath.Join(base, "db")); err == nil {
		return base, nil
	}
	entries, _ := os.ReadDir(base)
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(base, e.Name(), "db")); err == nil {
				return filepath.Join(base, e.Name()), nil
			}
		}
	}
	return "", errors.New("no LevelDB 'db' folder found (not a vanilla Bedrock world)")
}

// unzip extracts a .mcworld/.zip archive into dst, guarding against zip-slip.
func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	cleanDst := filepath.Clean(dst) + string(os.PathSeparator)
	for _, f := range r.File {
		fp := filepath.Join(dst, f.Name)
		if !strings.HasPrefix(fp, cleanDst) {
			return fmt.Errorf("illegal path in archive: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fp, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			return err
		}
		if err := extractZipFile(f, fp); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// ExportBedrockWorld would write a vanilla Bedrock world (LevelDB) from a
// LivingWorld directory. Not implemented yet.
func ExportBedrockWorld(srcDir, dstDir string) (Stats, error) {
	return Stats{}, ErrBedrockUnsupported
}
