// Package resourcepack converts a vanilla resource pack between the Java and
// Bedrock editions so a LivingWorld server can dress both client types from one
// source pack.
//
// Scope (v1): pack structure, the edition manifest (pack.mcmeta <-> manifest.json),
// the pack icon, and TEXTURES (.png) remapped between the editions' folder
// layouts (Java assets/minecraft/textures/block <-> Bedrock textures/blocks, …).
//
// NOT converted yet (these are large, dedicated efforts): per-file texture NAME
// differences and the Bedrock terrain_texture.json/item_texture.json indirection,
// 3D models (Java JSON models <-> Bedrock geometry), sounds, languages, fonts —
// and Bedrock ADDONS (behaviour packs), which are a content/runtime feature, not
// a file conversion. Unconverted files are reported in Report.Skipped.
package resourcepack

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Edition identifies a resource pack's edition.
type Edition int

const (
	Unknown Edition = iota
	Java
	Bedrock
)

func (e Edition) String() string {
	switch e {
	case Java:
		return "Java"
	case Bedrock:
		return "Bedrock"
	default:
		return "Unknown"
	}
}

// Report summarises a conversion.
type Report struct {
	Source  Edition
	Target  Edition
	Copied  int // textures + icon written
	Skipped int // files not handled by v1 (models, sounds, lang, …)
	OutDir  string
}

// javaPackFormat is the pack.mcmeta format stamped on Java output. Java validates
// this; adjust it to match the target client if the pack is rejected.
const javaPackFormat = 34

// Convert auto-detects the edition of the pack at srcPath (a directory, .zip,
// .mcpack or .jar) and writes the opposite-edition pack into dstDir.
func Convert(srcPath, dstDir string) (Report, error) {
	ed, err := Detect(srcPath)
	if err != nil {
		return Report{}, err
	}
	switch ed {
	case Java:
		return convert(srcPath, dstDir, Java, Bedrock)
	case Bedrock:
		return convert(srcPath, dstDir, Bedrock, Java)
	default:
		return Report{}, fmt.Errorf("unrecognised resource pack at %s (no pack.mcmeta or manifest.json)", srcPath)
	}
}

// Detect reports whether the pack is a Java or Bedrock pack by looking for its
// signature file (pack.mcmeta for Java, manifest.json for Bedrock).
func Detect(srcPath string) (Edition, error) {
	ed := Unknown
	err := walkPack(srcPath, func(rel string, _ func() (io.ReadCloser, error)) error {
		switch strings.ToLower(path.Base(rel)) {
		case "pack.mcmeta":
			ed = Java
		case "manifest.json":
			if ed == Unknown {
				ed = Bedrock
			}
		}
		return nil
	})
	return ed, err
}

func convert(srcPath, dstDir string, from, to Edition) (Report, error) {
	rep := Report{Source: from, Target: to, OutDir: dstDir}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return rep, err
	}
	err := walkPack(srcPath, func(rel string, open func() (io.ReadCloser, error)) error {
		dst, ok := mapPath(rel, from)
		if !ok {
			rep.Skipped++
			return nil
		}
		if err := copyInto(open, filepath.Join(dstDir, filepath.FromSlash(dst))); err != nil {
			return err
		}
		rep.Copied++
		return nil
	})
	if err != nil {
		return rep, err
	}
	if err := writeManifest(dstDir, to); err != nil {
		return rep, err
	}
	return rep, nil
}

// mapPath returns the destination (target-edition) path for a source file, or
// ok=false if v1 does not convert it. from is the SOURCE edition.
func mapPath(rel string, from Edition) (string, bool) {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	if from == Java {
		// Icon: pack.png -> pack_icon.png
		if rel == "pack.png" {
			return "pack_icon.png", true
		}
		// assets/<ns>/textures/<sub>/<rest> -> textures/<remap>/<rest>
		parts := strings.Split(rel, "/")
		if len(parts) >= 5 && parts[0] == "assets" && parts[2] == "textures" && strings.HasSuffix(rel, ".png") {
			sub := remapTexDir(parts[3], Java)
			return "textures/" + sub + "/" + strings.Join(parts[4:], "/"), true
		}
		return "", false
	}
	// Bedrock source.
	if rel == "pack_icon.png" {
		return "pack.png", true
	}
	parts := strings.Split(rel, "/")
	if len(parts) >= 3 && parts[0] == "textures" && strings.HasSuffix(rel, ".png") {
		sub := remapTexDir(parts[1], Bedrock)
		return "assets/minecraft/textures/" + sub + "/" + strings.Join(parts[2:], "/"), true
	}
	return "", false
}

// remapTexDir maps a texture subfolder name between editions. from is the SOURCE
// edition. Only the names that actually differ are remapped; the rest pass
// through (entity, environment, particle, gui, painting, …).
func remapTexDir(sub string, from Edition) string {
	if from == Java {
		switch sub {
		case "block":
			return "blocks"
		case "item":
			return "items"
		}
		return sub
	}
	switch sub {
	case "blocks":
		return "block"
	case "items":
		return "item"
	}
	return sub
}

func writeManifest(dstDir string, to Edition) error {
	if to == Bedrock {
		m := map[string]any{
			"format_version": 2,
			"header": map[string]any{
				"name":               "LivingWorld Converted Pack",
				"description":        "Converted from a Java pack by LivingWorld",
				"uuid":               uuid.NewString(),
				"version":            []int{1, 0, 0},
				"min_engine_version": []int{1, 21, 0},
			},
			"modules": []map[string]any{{
				"type":    "resources",
				"uuid":    uuid.NewString(),
				"version": []int{1, 0, 0},
			}},
		}
		return writeJSON(filepath.Join(dstDir, "manifest.json"), m)
	}
	m := map[string]any{
		"pack": map[string]any{
			"pack_format": javaPackFormat,
			"description": "Converted from a Bedrock pack by LivingWorld",
		},
	}
	return writeJSON(filepath.Join(dstDir, "pack.mcmeta"), m)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// copyInto streams the source file (via open) into dst, creating parent dirs.
func copyInto(open func() (io.ReadCloser, error), dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	rc, err := open()
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

// walkPack invokes fn for every file in a pack, whether it is a directory or a
// .zip/.mcpack/.jar archive. rel is the slash-separated path within the pack.
func walkPack(srcPath string, fn func(rel string, open func() (io.ReadCloser, error)) error) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(srcPath, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(srcPath, p)
			return fn(filepath.ToSlash(rel), func() (io.ReadCloser, error) { return os.Open(p) })
		})
	}
	r, err := zip.OpenReader(srcPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		f := f
		if err := fn(f.Name, func() (io.ReadCloser, error) { return f.Open() }); err != nil {
			return err
		}
	}
	return nil
}
