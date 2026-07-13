package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GenerateVanilladata reads extracted vanilla worldgen JSON/NBT files from srcDir
// and writes generated Go source files for the vanilladata package into dstDir.
func GenerateVanilladata(srcDir, dstDir string) error {
	if _, err := os.Stat(srcDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worldgen source directory not found: %s", srcDir)
		}
		return err
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	density := map[string]json.RawMessage{}
	if err := collectJSON(filepath.Join(srcDir, "density_function"), "", density); err != nil {
		return err
	}

	noiseSettings := map[string]json.RawMessage{}
	if err := collectJSON(filepath.Join(srcDir, "noise_settings"), "", noiseSettings); err != nil {
		return err
	}

	noises := map[string]json.RawMessage{}
	if err := collectJSON(filepath.Join(srcDir, "noise"), "", noises); err != nil {
		return err
	}

	if err := writeDataGo(dstDir, density, noiseSettings, noises); err != nil {
		return err
	}

	srcStructures := filepath.Join(srcDir, "structures")
	dstStructures := filepath.Join(dstDir, "structures")
	if _, err := os.Stat(srcStructures); err == nil {
		if err := copyDir(srcStructures, dstStructures); err != nil {
			return err
		}
		if err := writeStructuresGo(dstDir); err != nil {
			return err
		}
	}

	return nil
}

func collectJSON(root, prefix string, out map[string]json.RawMessage) error {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = strings.TrimSuffix(rel, ".json")
		rel = filepath.ToSlash(rel)
		key := rel
		if prefix != "" {
			key = prefix + "/" + rel
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[key] = data
		return nil
	})
}

func writeDataGo(dstDir string, density, noiseSettings, noises map[string]json.RawMessage) error {
	path := filepath.Join(dstDir, "data.go")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := fmt.Fprint(f, generatedHeader("vanillagen.go", "worldgen")+"\npackage vanilladata\n\nimport \"encoding/json\"\n\n"); err != nil {
		return err
	}
	if err := writeMap(f, "DensityFunctions", density); err != nil {
		return err
	}
	if err := writeMap(f, "NoiseSettings", noiseSettings); err != nil {
		return err
	}
	if err := writeMap(f, "Noises", noises); err != nil {
		return err
	}
	return f.Sync()
}

func writeMap(f *os.File, name string, m map[string]json.RawMessage) error {
	if _, err := fmt.Fprintf(f, "var %s = map[string]json.RawMessage{\n", name); err != nil {
		return err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m[k]
		if _, err := fmt.Fprintf(f, "\t%q: json.RawMessage([]byte(%s)),\n", k, goStringLiteral(v)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(f, "}\n\n")
	return err
}

func goStringLiteral(data []byte) string {
	if len(data) == 0 {
		return `""`
	}
	const maxChunk = 80
	var chunks []string
	var b strings.Builder
	for _, by := range data {
		tok := stringToken(by)
		if b.Len() > 0 && b.Len()+len(tok) > maxChunk {
			chunks = append(chunks, `"`+b.String()+`"`)
			b.Reset()
		}
		b.WriteString(tok)
	}
	if b.Len() > 0 {
		chunks = append(chunks, `"`+b.String()+`"`)
	}
	if len(chunks) == 1 {
		return chunks[0]
	}
	return strings.Join(chunks, " + ")
}

func stringToken(b byte) string {
	switch b {
	case '"':
		return `\"`
	case '\\':
		return `\\`
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	}
	if b >= 0x20 && b < 0x7F {
		return string(b)
	}
	return fmt.Sprintf(`\x%02x`, b)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		return copyFile(path, dstPath)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func writeStructuresGo(dstDir string) error {
	path := filepath.Join(dstDir, "structures.go")
	content := generatedHeader("vanillagen.go", "structures") + `package vanilladata

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
`
	return os.WriteFile(path, []byte(content), 0o644)
}
