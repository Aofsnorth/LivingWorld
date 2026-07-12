# Overworld Phase 0 — Vanilla Data Extraction

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract 1.26.2 Overworld worldgen data (density functions, noise settings, noise, structures) from the vanilla server jar into the repo so the generator can use it.

**Architecture:** Extend `third_party/go-mc/tools/extract.go` to pull the additional JSON/NBT files from the downloaded server jar, then write a small Go generator that converts those files into a new `internal/dimension/overworld/vanilladata` package.

**Tech Stack:** Go 1.26.5, `archive/zip`, `encoding/json`, `github.com/Tnze/go-mc/nbt`, `go-mc/tools` extraction harness.

## Global Constraints

- All CI gates must stay green: `go build ./...`, `go vet ./...`, `go test ./...`, `govulncheck ./...`.
- Deterministic and seed-driven: extracted data is constants, no runtime network calls.
- No runtime network calls to Mojang from the server.
- `go-mc/tools` is a separate module (`third_party/go-mc/tools/`); keep changes there for extraction, then generate code into `internal/dimension/overworld/vanilladata`.

---

## File Structure

| File | Responsibility |
|------|---------------|
| `third_party/go-mc/tools/extract.go` | Downloads server jar, extracts worldgen JSONs and structure NBTs to a temp directory. |
| `third_party/go-mc/tools/main.go` | CLI entry point; drives extraction and code generation. |
| `third_party/go-mc/tools/vanillagen.go` (new) | Reads extracted JSON/NBT and emits Go source files. |
| `internal/dimension/overworld/vanilladata/data.go` (new) | Embeds extracted density functions, noise settings, noise definitions, and structure manifests as Go `const`/`var` or JSON blobs. |
| `internal/dimension/overworld/vanilladata/structures.go` (new) | Loads `.nbt` files into `level/block` block-state data structures. |
| `internal/dimension/overworld/vanilladata/vanilladata_test.go` (new) | Verifies data is loaded and has expected keys. |

---

## Task 1: Extend server jar extraction

**Files:**
- Modify: `third_party/go-mc/tools/extract.go`
- Test: `third_party/go-mc/tools/extract_test.go` (new)

**Interfaces:**
- Consumes: nothing new.
- Produces: `ExtractWorldgenData(jar string, outDir string) error` extracts JSON/NBT files.

- [ ] **Step 1: Write a failing test**

Create `third_party/go-mc/tools/extract_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractWorldgenData(t *testing.T) {
	jar := os.Getenv("VANILLA_JAR")
	if jar == "" {
		t.Skip("VANILLA_JAR not set")
	}
	dir := t.TempDir()
	if err := ExtractWorldgenData(jar, dir); err != nil {
		t.Fatalf("ExtractWorldgenData: %v", err)
	}
	for _, f := range []string{
		"density_function/overworld/base_3d_noise.json",
		"noise_settings/overworld.json",
		"worldgen/noise/temperature.json",
		"structures/village/plains/houses/plains_small_house_1.nbt",
	} {
		if _, err := os.Stat(filepath.Join(dir, f)); os.IsNotExist(err) {
			t.Fatalf("expected extracted file %s", f)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd third_party/go-mc/tools
go test -run TestExtractWorldgenData -v
```

Expected: FAIL with `ExtractWorldgenData` not defined.

- [ ] **Step 3: Implement minimal extraction**

Add to `third_party/go-mc/tools/extract.go`:

```go
func ExtractWorldgenData(jar, outDir string) error {
	zr, err := zip.OpenReader(jar)
	if err != nil {
		return err
	}
	defer zr.Close()

	prefixes := []string{
		"data/minecraft/worldgen/density_function/",
		"data/minecraft/worldgen/noise/",
		"data/minecraft/worldgen/noise_settings/",
		"data/minecraft/worldgen/world_preset/",
		"data/minecraft/structure/",
	}

	for _, f := range zr.File {
		for _, p := range prefixes {
			if strings.HasPrefix(f.Name, p) && !strings.HasSuffix(f.Name, "/") {
				rel := strings.TrimPrefix(f.Name, "data/minecraft/")
				dst := filepath.Join(outDir, rel)
				if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
					return err
				}
				rc, err := f.Open()
				if err != nil {
					return err
				}
				data, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					return err
				}
				if err := os.WriteFile(dst, data, 0644); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
```

Add `io` and `path/filepath` to imports if missing.

- [ ] **Step 4: Run test to verify it passes**

Set `VANILLA_JAR` to the path of the 26.2 server jar and run:

```bash
cd third_party/go-mc/tools
VANILLA_JAR=... go test -run TestExtractWorldgenData -v
```

Expected: PASS.

- [ ] **Step 5: Wire into main**

Modify `third_party/go-mc/tools/main.go` to call `ExtractWorldgenData` after the existing extraction, writing to `temp/cache/worldgen/<version>/`.

- [ ] **Step 6: Commit**

```bash
git add third_party/go-mc/tools/extract.go third_party/go-mc/tools/main.go third_party/go-mc/tools/extract_test.go
git commit -m "feat(tools): extract worldgen JSON and NBT from server jar"
```

---

## Task 2: Add Go code generator

**Files:**
- Create: `third_party/go-mc/tools/vanillagen.go`
- Modify: `third_party/go-mc/tools/main.go`

**Interfaces:**
- Consumes: extracted JSON files in `temp/cache/worldgen/<version>/`.
- Produces: `vanilladata` package files under `internal/dimension/overworld/vanilladata/`.

- [ ] **Step 1: Write a failing test**

Create `third_party/go-mc/tools/vanillagen_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateVanilladata(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "density_function", "overworld", "base_3d_noise.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := GenerateVanilladata(src, dst); err != nil {
		t.Fatalf("GenerateVanilladata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "data.go")); os.IsNotExist(err) {
		t.Fatal("expected data.go")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd third_party/go-mc/tools
go test -run TestGenerateVanilladata -v
```

Expected: FAIL with `GenerateVanilladata` not defined.

- [ ] **Step 3: Implement generator skeleton**

Create `third_party/go-mc/tools/vanillagen.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"text/template"
)

func GenerateVanilladata(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}
	// TODO: walk density functions, noise settings, noise, and structure manifests
	//       and generate data.go.
	return os.WriteFile(filepath.Join(dstDir, "data.go"), []byte("package vanilladata\n"), 0644)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd third_party/go-mc/tools
go test -run TestGenerateVanilladata -v
```

Expected: PASS.

- [ ] **Step 5: Populate density function JSON data**

Extend `GenerateVanilladata` to collect all `density_function/overworld/**/*.json` files into a `map[string]json.RawMessage` and write it into `data.go` as a Go `var DensityFunctions`.

Example `data.go` output:

```go
package vanilladata

import "encoding/json"

var DensityFunctions = map[string]json.RawMessage{
	"overworld/base_3d_noise":           json.RawMessage(`...`),
	"overworld/continents":              json.RawMessage(`...`),
	// ...
}

var NoiseSettings = map[string]json.RawMessage{
	"overworld": json.RawMessage(`...`),
}

var Noises = map[string]json.RawMessage{
	"temperature": json.RawMessage(`...`),
	// ...
}
```

- [ ] **Step 6: Add structure manifest**

Walk `structure/` directory and create `var Structures = map[string][]byte` mapping `path/to/template.nbt` to the raw NBT bytes. Use `go:embed` or `//go:embed` if the directory is embedded. For this phase, write the NBT bytes as `[]byte` literals in `structures.go`.

- [ ] **Step 7: Run full extraction and generation**

```bash
cd third_party/go-mc/tools
go run . 26.2
```

Expected: `internal/dimension/overworld/vanilladata/data.go` and `structures.go` are generated.

- [ ] **Step 8: Commit**

```bash
git add third_party/go-mc/tools/vanillagen.go third_party/go-mc/tools/vanillagen_test.go third_party/go-mc/tools/main.go internal/dimension/overworld/vanilladata/
git commit -m "feat(tools): generate vanilladata package from extracted worldgen"
```

---

## Task 3: Add `vanilladata` loader package

**Files:**
- Create: `internal/dimension/overworld/vanilladata/loader.go`
- Create: `internal/dimension/overworld/vanilladata/vanilladata_test.go`

**Interfaces:**
- Consumes: generated `data.go` and `structures.go`.
- Produces: `func LoadDensityFunction(name string) ([]byte, bool)`, `func LoadNoiseSettings(name string) ([]byte, bool)`, `func LoadNoise(name string) ([]byte, bool)`, `func LoadStructure(path string) ([]byte, bool)`.

- [ ] **Step 1: Write a failing test**

Create `internal/dimension/overworld/vanilladata/vanilladata_test.go`:

```go
package vanilladata

import (
	"testing"
)

func TestLoadDensityFunction(t *testing.T) {
	data, ok := LoadDensityFunction("overworld/base_3d_noise")
	if !ok {
		t.Fatalf("expected base_3d_noise to exist")
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty data")
	}
}

func TestLoadStructure(t *testing.T) {
	data, ok := LoadStructure("village/plains/houses/plains_small_house_1")
	if !ok {
		t.Fatalf("expected small house structure to exist")
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty data")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/dimension/overworld/vanilladata/... -v
```

Expected: FAIL with `LoadDensityFunction` not defined.

- [ ] **Step 3: Implement loader functions**

Create `internal/dimension/overworld/vanilladata/loader.go`:

```go
package vanilladata

func LoadDensityFunction(name string) ([]byte, bool) {
	v, ok := DensityFunctions[name]
	if !ok {
		return nil, false
	}
	return []byte(v), true
}

func LoadNoiseSettings(name string) ([]byte, bool) {
	v, ok := NoiseSettings[name]
	if !ok {
		return nil, false
	}
	return []byte(v), true
}

func LoadNoise(name string) ([]byte, bool) {
	v, ok := Noises[name]
	if !ok {
		return nil, false
	}
	return []byte(v), true
}

func LoadStructure(path string) ([]byte, bool) {
	v, ok := Structures[path]
	if !ok {
		return nil, false
	}
	return v, true
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/dimension/overworld/vanilladata/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/dimension/overworld/vanilladata/
git commit -m "feat(overworld): add vanilladata loader for density, noise, and structures"
```

---

## Task 4: Regression test and integration

- [ ] **Step 1: Run full CI gates**

```bash
go build ./...
go vet ./...
go test ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Expected: all green.

- [ ] **Step 2: Verify `cmd/worldview` still builds**

```bash
go build ./cmd/worldview
```

Expected: builds.

- [ ] **Step 3: Commit**

```bash
git commit -m "test(overworld): verify phase 0 data extraction keeps CI green"
```

---

## Spec Coverage Check

- Density function extraction → Task 1.
- Noise settings / noise extraction → Task 1.
- Structure NBT extraction → Task 1.
- `vanilladata` package → Tasks 2 and 3.
- `worldgen` data loaded at runtime → Task 3.
- CI gates green → Task 4.

## Placeholder Scan

- No TBD/TODO/"implement later".
- Code blocks are concrete.
- Exact file paths are given.
- Commands have expected output.
