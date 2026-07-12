package vanilladata

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

//go:embed all:structures
var structuresFS embed.FS

// Structures maps structure template paths to their raw NBT bytes.
// Both the full path with the .nbt extension and the path without the
// extension are supported as keys.
var Structures = map[string][]byte{}

func init() {
	err := fs.WalkDir(structuresFS, "structures", func(filepath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if path.Ext(filepath) != ".nbt" {
			return nil
		}
		data, err := structuresFS.ReadFile(filepath)
		if err != nil {
			return err
		}
		key := strings.TrimPrefix(filepath, "structures/")
		Structures[key] = data
		Structures[strings.TrimSuffix(key, ".nbt")] = data
		return nil
	})
	if err != nil {
		panic(err)
	}
}
