# go-mc/tools — Code Generation & MC Data Extraction

Unified tool that extracts Minecraft data from the Mojang server jar and
generates all Go source files for go-mc.

## Quick Start

```bash
# Full pipeline: extract MC data (if needed) + generate Go code
cd tools && go run . --version 1.21.11

# Force re-extraction even if cached JSONs exist
cd tools && go run . --version 1.21.11 --extract

# Generate from a specific JSON directory
cd tools && go run . ../temp/jsons/1.21.11
```

## Requirements

- Go 1.22+
- `podman` or `docker` (for `--extract` mode)
- Internet access (first run downloads ~57 MB server jar; cached afterward)

## Usage

```
go run . [options] [json-dir]

Options:
  --version VER       MC version (extracts if cached JSONs missing)
  --extract           Force re-extraction even if cache exists
  --runtime RT        Container runtime: podman (default) or docker
  --dry-run           Print container command without running
  --gen-only          Extract only, skip generation

JSON dir can also be a positional arg: `go run . /path/to/jsons`
```

## What It Does

### Extraction (`--extract`)

Runs as a 3-phase pipeline:

**Phase 1 — Go host downloads (needs internet):**
1. Downloads the MC server jar (from `unobfuscated_versions.json` for
   pre-26.x, or from the Mojang version manifest for 26.x+)
2. Downloads all ~147 language files from the Mojang asset index CDN

**Phase 2 — Container extraction (no internet needed):**

Runs an `eclipse-temurin:21-jdk` container that:
1. Extracts the inner server jar from the bundler format
2. Runs the MC `--all` data generator → reports/*.json
3. Copies key reports: `blocks.json`, `packets.json`, `registries.json`,
   `items.json`, `commands.json`, `datapack.json`
4. Compiles and runs 6 custom Java extractors:
   - **GenEntities** — entity types with dimensions (id, name, width, height)
   - **GenComponents** — data component types with networkability flags
   - **GenComponentSchema** — component wire format schema via reflection
   - **GenBlockEntities** — block entity types with valid blocks
   - **GenBlockProperties** — block state property definitions (boolean/integer/enum)
   - **GenBiomes** — biome protocol ordering via runtime registry introspection

Output: `temp/jsons/<version>/*.json` + `lang/` (~83 MB total)

### Generation

Runs 10 Go generators that read the extracted JSON files and produce
Go source code:

| Generator | Input | Output |
|-----------|-------|--------|
| packetid | `packets.json` | `data/packetid/packetid.go` |
| soundid | `registries.json` | `data/soundid/soundid.go` |
| item | `items.json` + `registries.json` | `data/item/item.go` |
| blocks | `blocks.json` + `block_properties.json` | `level/block/blocks.go` + `block_states.nbt` + `properties_enum.go` |
| entity | `entities.json` | `data/entity/entity.go` |
| component | `components.json` + `component_schema.json` | `level/component/components.go` + `*_gen.go` |
| blockentities | `block_entities.json` | `level/block/blockentity.go` + `blockentities.go` |
| registryid | `registries.json` | `data/registryid/*.go` (95 files) |
| biome | `biomes.json` | `level/biome/list.go` |
| lang | `lang/*.json` | `data/lang/<locale>/<locale>.go` (147 languages) |

## Directory Layout

```
tools/
├── main.go              # unified entry point
├── download.go          # Go-side downloads (server jar + languages)
├── extract.go           # 3-phase extraction pipeline orchestration
├── helpers.go           # shared utilities
├── gen_packetid.go      # generator: packet IDs
├── gen_soundid.go       # generator: sound IDs
├── gen_item.go          # generator: items
├── gen_blocks.go        # generator: blocks + states + properties
├── gen_entity.go        # generator: entities
├── gen_component.go     # generator: data components
├── gen_blockentities.go # generator: block entities
├── gen_registryid.go    # generator: registry IDs (95 registries)
├── gen_biome.go         # generator: biomes
├── gen_lang.go          # generator: language translations (147 languages)
├── gen_component_types.go # generator: component type structs (*_gen.go)
├── hand-crafted/        # manually maintained config files
│   ├── component_schema.json
│   ├── naming_overrides.json
│   └── packet_phases.json
├── unobfuscated_versions.json  # hardcoded unobfuscated jar URLs (pre-26.x)
├── java/                # Java extractor sources (committed)
│   ├── ExtractAll.java  # container extraction (no internet needed)
│   ├── GenBiomes.java
│   ├── GenBlockEntities.java
│   ├── GenBlockProperties.java
│   ├── GenComponents.java
│   ├── GenComponentSchema.java
│   └── GenEntities.java
└── go.mod               # separate module (replace → parent go-mc)
```

Extracted data lives at the repo root (gitignored):

```
temp/                        # gitignored working data
├── cache/                   # cached server jars + libs
└── jsons/                   # extracted JSON files per version
    └── 1.21.11/
        ├── blocks.json
        ├── packets.json
        ├── registries.json
        ├── items.json
        ├── entities.json
        ├── components.json
        ├── block_entities.json
        ├── block_properties.json
        ├── component_schema.json
        ├── biomes.json
        ├── lang/                # 147 language JSON files
        │   ├── en_us.json
        │   ├── cs_cz.json
        │   └── ...
        └── ...
```

## Updating to a New MC Version

```bash
cd tools

# 1. Extract (if needed) + generate
go run . --version 1.XX.YY

# 2. Verify from go-mc root
cd .. && go build ./... && go test ./...

# 3. Review generated diffs
git diff --stat
```

## Module Structure

`tools/` is a separate Go module (`github.com/Tnze/go-mc/tools`) with a
`replace` directive pointing to the parent go-mc module. This keeps the
generation tooling out of the main module's dependency graph.

```go
// tools/go.mod
module github.com/Tnze/go-mc/tools
go 1.22
require github.com/Tnze/go-mc v1.22.0
replace github.com/Tnze/go-mc => ../
```

## Adding a New Generator

1. Create `gen_foo.go` with `func genFoo(jsonDir, goMCRoot string) error`
2. Add `{"foo", genFoo}` to the `generators` slice in `main.go`
3. Use the shared helpers from `helpers.go` (`readJSON`, `writeFile`,
   `snakeToCamel`, etc.)

## Unobfuscated Versions (Pre-26.x)

Versions before 26.1 ship obfuscated by default. Separate unobfuscated
builds were released for versions 25w45a through 1.21.11. These are not
listed in the standard Mojang version manifest, so their server jar URLs
are hardcoded in `tools/unobfuscated_versions.json`.

Versions 26.1+ are natively unobfuscated and use the standard manifest
automatically.

### Adding a new unobfuscated version entry

1. Open the Minecraft Wiki page for the version (e.g.,
   `https://minecraft.wiki/w/Java_Edition_1.21.11`)
2. Find the wikitext infobox via the wiki API:
   ```
   curl -s 'https://minecraft.wiki/api.php?action=parse&page=Java+Edition+VERSION&prop=wikitext&format=json&section=0' | jq -r '.parse.wikitext["*"]'
   ```
3. Look for the `serverdl` line with `{{dl|HASH|server|title=Unobfuscated}}`
4. The download URL is: `https://piston-data.mojang.com/v1/objects/HASH/server.jar`
5. Add an entry to `tools/unobfuscated_versions.json`:
   ```json
   {
     "VERSION": {
       "server_sha1": "HASH",
       "server_url": "https://piston-data.mojang.com/v1/objects/HASH/server.jar"
     }
   }
   ```

## Adding a New Java Extractor

1. Create `java/GenFoo.java` — a single-file Java 21 program that reads
   from the MC server jar classpath and writes JSON to the output directory
2. Add `"GenFoo"` to the `extractors` array in `java/ExtractAll.java`
3. Create the corresponding Go generator to consume the JSON output
