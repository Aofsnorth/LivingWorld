# Overworld Generation — Vanilla-Identical Design

## 1. Goal

Make LivingWorld's Overworld chunk generator byte-for-byte compatible with vanilla
Minecraft 1.26.2 (protocol 776) while keeping the existing deterministic, seed-driven,
chunk-independent architecture. The final output of `pipeline.Generate(cx, cz)` must
match the same (seed, cx, cz) block states produced by vanilla.

## 2. Scope

In scope:
- Density function tree extraction, parsing, and evaluation.
- Aquifer and cave-carver integration.
- Surface rules and cave-biome placement.
- Structure planning → realization (NBT/jigsaw).

Out of scope (for this design):
- Nether / End generation.
- Mod data packs.
- Biome-specific feature / decoration beyond vanilla defaults.

## 3. Background & Current State

`internal/dimension/overworld/pipeline` currently produces terrain with a hand-tuned
spline-and-noise approach:

- `shapeColumn` builds a `column` from octave noise (temperature, humidity,
  continentalness, erosion, weirdness, detail, jagged).
- `buildColumn` materialises the column with a hard-coded surface pass.
- `carveCaves` is an inline cheese/spaghetti cave carver.
- `placeOres` and `decorate` are already in place and work.
- `internal/dimension/overworld/density` has a **partial framework** (`Noise`,
  `ShiftedNoise`, `RangeChoice`, `Spline`, `Add`, `Mul`, `Subtract`, `Clamp`) and
  placeholder `BuildFinalDensity` / `BuildAquiferFields` that are **not wired**.
- `internal/dimension/overworld/aquifer` and `internal/dimension/overworld/carver`
  exist but are not used by `pipeline`.
- `internal/dimension/overworld/structure/planner.go` finds structure starts but
  does not place pieces.

Vanilla 1.26.2 generates terrain with the `final_density` function tree, then runs
`aquifer`, `carver`, `surface_rules`, `cave_biome`, `ore_features`, and `structures`
sequentially. This design follows that exact order.

## 4. Architecture

```
+------------------------------------------------+
|  Server  | dimension.NewOverworld()             |
|          |   pipeline.NewGenerator(seed)        |
+----------+--------------------------------------+
|  Pipeline.Generate(cx, cz)                     |
|    1. Build DensityContext for chunk.          |
|    2. Evaluate FinalDensity per block.         |
|    3. Build solid/air column.                  |
|    4. Apply aquifer to cave/air cells.         |
|    5. Run carvers (cave + canyon).             |
|    6. Apply surface rules per column.          |
|    7. Place ores.                              |
|    8. Place cave biomes / decoration.          |
|    9. Place structures (jigsaw).               |
|   10. Place vegetation features.               |
+------------------------------------------------+
|  Density  |  extracted from server jar data    |
|  Context  |  + noise + climate                 |
+------------------------------------------------+
```

### Key principles

- **Deterministic**: same `(seed, cx, cz)` → same chunk, always.
- **Chunk-independent**: neighbour-dependent data (structures, jigsaw trees) are
  re-anchored from the owning chunk, not stored.
- **Dependency-injected**: `DensityContext` carries `NoiseSet`, `ClimateReader`,
  `SeaLevel`, `MinY`, `MaxY`, and `WorldgenRandom` — no package globals.
- **Layered**: each stage is a pure function from `(chunk, context, stage)` to a
  modified chunk.

## 5. Components

### 5.1 `WorldgenContext` and `DensityContext`

New types in `internal/dimension/overworld/density`:

```go
type WorldgenContext struct {
    Seed          int64
    Climate       ClimateReader
    Noises        NoiseSet
    FinalDensity  Function
    AquiferFields AquiferFields
}

type DensityContext struct {
    WorldgenContext
    ChunkX, ChunkZ int
    BlockX, BlockY, BlockZ int  // updated per-cell
}
```

`WorldgenContext` is built once per `Generator` in `NewGenerator`.
`DensityContext` is a per-call scratch struct passed to `Function.Eval`.

### 5.2 Vanilla data extraction

Extend `third_party/go-mc/tools/extract.go` to read, from the 26.2 server jar:

- `data/minecraft/worldgen/density_function/*.json`
- `data/minecraft/worldgen/noise/*.json`
- `data/minecraft/worldgen/noise_settings/overworld.json`
- `data/minecraft/worldgen/world_preset/normal.json` (for structure sets)
- `data/minecraft/structures/*.nbt` (jigsaw template pieces)

Generate Go code or load JSON at runtime. For determinism and startup speed, prefer
pre-generated Go `var` in a new package `internal/dimension/overworld/vanilladata`.

### 5.3 Density function operators

The `density.Function` interface already exists. Add missing operators needed by the
vanilla tree:

| Operator | Purpose |
|----------|---------|
| `CacheOnce` | memoize expensive sub-trees per cell |
| `CacheAllInCell` | cache entire 3x3x3 neighbourhood for interpolation |
| `Interpolated` | linear interpolation of cell-corner samples |
| `BlendDensity` | blends density at biome borders |
| `ShiftNoise` | offsets coordinates by another noise field |
| `YClampedGradient` | `min(y, maxY) / range` for depth weight |
| `Slide` | vanilla slide offset for underground/overworld |
| `Beardifier` | adds structure density offsets |
| `ClampedGradient` | y-axis gradient used in aquifer |

`BuildFinalDensity` is replaced with a parser that maps extracted JSON to these
operators.

### 5.4 Pipeline rewrite

`pipeline.Generate` becomes:

```go
func (g *Generator) Generate(cx, cz int) *world.Chunk {
    c := world.NewChunk()
    ctx := g.context.ForChunk(cx, cz)

    g.shapeWithDensity(ctx, c)   // new: per-cell density
    g.applyAquifer(ctx, c)
    g.carveCaves(ctx, c)         // move inline carver to carver.Cave
    g.carveCanyons(ctx, c)
    g.applySurfaceRules(ctx, c)
    g.placeOres(ctx, c)
    g.applyCaveBiomes(ctx, c)
    g.placeStructures(ctx, c)
    g.decorate(ctx, c)

    g.writeHeightmaps(c)
    return c
}
```

### 5.5 Aquifer

`density.BuildAquiferFields` builds four `Function`s from extracted JSON:

- `barrier`
- `fluid_level_floodedness`
- `fluid_level_spread`
- `lava`

`aquifer.Apply` keeps its current logic but is called after `shapeWithDensity` and
before carvers so carver-emptied cells can be flooded. (Note: vanilla order is
`final_density` → `aquifer` → `carver`; our current aquifer docs say "after carvers".
This is adjusted to match vanilla.)

### 5.6 Carvers

Move `cave` logic from `pipeline/caves.go` into `carver.NewOverworldCave` and add it
to the carver list. Keep `canyon` from `carver/carver.go` and enable it.

Order: `cave` then `canyon`.

### 5.7 Surface rules

Replace `surface/` with a rule evaluator driven by vanilla `overworld.json` surface
rules. `surface/condition.go` is extended to implement all vanilla conditions.

### 5.8 Cave biomes

Add `internal/dimension/overworld/cavebiome` package:

- `SelectCaveBiome(ctx, x, y, z, climate)` returns `biome.ID`.
- Uses depth (`y`), noise (`cave_cheese`, `cave_entrance`), and climate
  (temperature) to choose `dripstone_caves`, `lush_caves`, or `deep_dark`.

After `surface` pass, set biome at relevant cells. The existing `biome` package
already has these IDs.

### 5.9 Structures

Extend `structure/planner.go` → `structure/realizer.go`:

1. **Extract NBT**: from server jar to `internal/dimension/overworld/structure/nbt`.
2. **Jigsaw assembler**: `JigsawRealizer` with `Pool` → `Piece` → `Placement` graph.
3. **Single-piece placement**: start with non-jigsaw structures (desert pyramid,
   igloo, swamp hut) to validate the NBT reader and block state mapping.
4. **Jigsaw structures**: villages, pillager outposts, strongholds, trial chambers.
5. **Mineshafts / ancient cities**: custom room/tunnel generators.

Block state mapping uses `world.StateID` for every block name/state present in the
NBT files.

## 6. Data Flow

1. `server.New` calls `dimension.NewOverworld()`.
2. `NewOverworld.Generator(seed)` builds `pipeline.Generator`.
3. `Generator` parses pre-generated `vanilladata` into `WorldgenContext`.
4. `Generate(cx, cz)` allocates `world.Chunk` and runs stages in order.
5. Each stage writes block IDs into `*world.Chunk`.
6. Final chunk is handed to `world.World` for storage and network encoding.

## 7. Phases

### Phase 0 — Data extraction (Foundation)

- Extend `go-mc/tools` to extract `density_function`, `noise_settings`, `noise`,
  `structures`.
- Generate `internal/dimension/overworld/vanilladata`.
- Add JSON → Go code generator for density functions and noise settings.

### Phase 1 — Density tree core

- Implement missing density operators.
- Replace `BuildFinalDensity` with parsed vanilla tree.
- Rewrite `pipeline.shapeColumn`/`buildColumn` to use `DensityContext`.
- Keep the old `column`-based code behind a `LEGACY_TERRAIN` flag until the new
  pipeline passes parity tests.

### Phase 2 — Aquifer & carvers

- Implement `BuildAquiferFields` from extracted data.
- Wire `aquifer.Apply` and `carver` package into `pipeline`.
- Tune canyon carver placement.

### Phase 3 — Surface rules & cave biomes

- Build surface rule evaluator from `overworld.json`.
- Implement missing conditions.
- Add `cavebiome` selection.

### Phase 4 — Structures

- NBT extraction and loader.
- Single-piece realizer.
- Jigsaw realizer.
- `structure/planner` integration with `pipeline`.

### Phase 5 — Parity & polish

- Compare `pipeline.Generate` output against vanilla seed for selected chunks.
- Add golden tests for density, aquifer, and surface rules.
- Remove `LEGACY_TERRAIN` flag.

## 8. Testing

- `pipeline/pipeline_test.go`: deterministic, varied, no-bedrock-on-surface.
- New `density` golden tests against extracted JSON.
- New `aquifer` unit tests for flood/lava decisions.
- New `surface` rule tests for representative biomes.
- New `structure` tests for planner and NBT loader.
- `cmd/worldview` remains the manual visual smoke test.
- Long-term: seed-pair comparison with a vanilla `server.jar`.

## 9. Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Vanilla density tree is huge and complex | Phase 1 starts with `LEGACY_TERRAIN` flag; ship in stages. |
| Performance regression from density tree | Use `CacheOnce` / `CacheAllInCell`; profile `Generate`. |
| NBT structure block state mapping mismatch | Use `go-mc` block state data and maintain a mapping table. |
| Chunk-border structures desync | Re-anchor from owner chunk; never store cross-chunk state. |
| Data extraction tool changes | Extend `go-mc/tools/extract.go` with tests for new files. |

## 10. Dependencies

- Existing `internal/dimension/overworld/*` packages.
- `third_party/go-mc/tools` for extraction.
- `third_party/go-mc/nbt` and `third_party/go-mc/level/block` for NBT.
- `internal/world` for block IDs and chunk storage.

## 11. Success Criteria

1. `go build ./...`, `go vet ./...`, `go test ./...`, `govulncheck ./...` remain green.
2. `cmd/worldview` generates a world that visually matches vanilla Overworld.
3. Selected chunk columns have identical block-state sequences compared to vanilla.
4. Existing Bedrock and Java bridge code continues to work without changes.
