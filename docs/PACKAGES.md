# LivingWorld Package Reference

Doc-coverage index for every package, mapped to its workstream lane (DESIGN [§3](specs/DESIGN.md) / §3.1) and requirements.
**✅** = landed, carries a `// Package` doc comment. **⏳** = planned; documented here until its code (and `doc.go`) lands.

> Convention: every landed package opens with a `// Package <name> …` comment (verified across the tree). New
> packages add that comment with their first file; this index is the human-facing summary.

## Core / canonical (lanes 2, 4)
| Package | St | Summary | Req |
|---|----|---------|-----|
| `internal/registry` | ✅ | Canonical edition-agnostic types (`BlockState`/`Pos`/`Vec3`/`ItemStack`/`Entity`) + Java↔Bedrock id maps. | R8 |
| `internal/world` | ✅ | Chunks, blocks, block-state properties, tick state, crack tracking. **Phase 4a:** unified per-world tick loop (`internal/world/tick.go`) owns the 7-phase cadence (player inputs → scheduled ticks → random ticks → mob AI → drop physics → outbound stage → autosave) at 20 Hz; the legacy `StartTimeLoop`/`StartMobAI`/`StartDropPhysics`/`StartAutosave` are now thin wrappers. | R3,R4 |
| `internal/player` | ✅ | Shared player model + cross-edition controller routing. **Phase 3:** explicit XP + Gamemode fields, atomic-rename player save writes, quarantine of malformed JSON on load. | R5,R6 |
| `internal/entity` | ✅ | Edition-agnostic entity `Manager` (spawn/despawn/id alloc). | R5 |
| `internal/entity/pathfind` | ✅ | A* pathfinding over an abstract `Nav` grid. | R5.1 |
| `internal/combat` | ✅ | Damage math: armor/resistance, knockback, criticals. | R5.4 |
| `internal/item` | ✅ | Item registry wrapping vanilla 26.1 item data. | R4 |
| `internal/loot` | ✅ | Block→drop loot-table resolution. | R4.2 |
| `internal/drops` | ✅ | Dropped-item entity tracking + physics. | R5.5 |
| `internal/command` | ✅ | Protocol-free command/cheat system. | R7.2 |

## Worldgen (lane 3)
| Package | St | Summary | Req |
|---|----|---------|-----|
| `internal/worldgen/noise` | ✅ | Deterministic seedable RNG + Perlin/octaves (fBm). | R3 |
| `internal/worldgen/biome` | ✅ | Climate-classified biomes + nearest-climate select. | R3 |
| `internal/worldgen/terrain` | ✅ | Buffer + height shaping, surface rules, cave carving. | R3 |
| `internal/worldgen` (top-level) | ✅ | **Phase 4c:** ore distribution pipeline (`ore.go`) over the terrain buffer — coal, iron, gold, redstone, diamond, lapis, emerald with vanilla-ish height bands; deterministic from (seed, x, y, z). Ravines and trees remain as future work (this wave ships ore + seed determinism). | R3 |

## Anticheat (lane 6)
| Package | St | Summary | Req |
|---|----|---------|-----|
| `internal/anticheat` | ✅ | Server-authoritative engine: weighted violations + decay + staged actions; movement/combat/timing checks. | R13 |

## Edges & networking (lanes 2, 7)
| Package | St | Summary | Req |
|---|----|---------|-----|
| `internal/java/*` | ✅ | Java edge today: protocol 775 wire I/O + state mapping (go-mc). | R1 |
| `internal/bedrock/*` | ✅ | Bedrock edge today: protocol 975 wire I/O + runtime mapping (gophertunnel). | R1 |
| `internal/skinbridge` | ✅ | Skin resolution/forwarding across editions. | R5.2 |
| `internal/auth` | ✅ | Mojang/Yggdrasil + Xbox Live auth chains. | R2 |
| `internal/version` | ✅ | `LWVersion` registry (`26 (A)` = Java `775` / Bedrock `975`), `Resolve(edition, protocol)` negotiation, capability bitset (`CapCustomPlayerModels`, `CapNetherUpdateNotes`). Surfaces via `/lwversion`, `livingworld --version`, and `cmd/versioncheck`. | R10 |
| `internal/network` | — | **Removed in Phase 1.** Was a dead scaffold (`Translator` used stub wire-ids; `bridge.go` never imported by `server.New()`). The Edition enum was salvaged into `internal/version`; the bridge idea itself is not part of the canonical architecture — the real bridge is the shared `world.Manager` + `player.Manager` (Master_Plan §3 audit row). | R1,R10,R12 |

## Persistence (lane 8)
| Package | St | Summary | Req |
|---|----|---------|-----|
| `internal/world` (persistence subpackage) | ✅ | Single live persistence path: `Storage` interface (`NopStorage` / `DiskStorage` / `RegionStorage`), region files at `region/r.<rx>.<rz>.lwr`, atomic temp+rename, autosave, world lock, level.json restore. **Phase 3:** corrupt-chunk quarantine moves bad region files to `quarantine/<timestamp>.<name>.bad` and returns a fresh empty region so the surface regenerates; chunk-level corruption inside a valid region drops the bad blob in-memory and forces a fresh write. The standalone `internal/persistence` package that wrote a divergent `c.<cx>.<cz>.gz` format was removed in Phase 1. | R3.4-3.6,R6.3 |
| `internal/worldconvert` | ✅ | Convert vanilla **Java Anvil** (`region/*.mca`) ⇄ LivingWorld region format (`region/r.*.lwr`), pivoting on block name (LivingWorld id == Java state id). Driven by `cmd/worldconvert`. Bedrock LevelDB path stubbed. | R3.4 |

## Public API & extensions (lanes 5, 9)
| Package | St | Summary | Req |
|---|----|---------|-----|
| `server` | ✅ | Public embeddable API (`server.New`/`Run`/`Host`); `cmd/server` = Vanilla flavor. | R9 |
| `plugin` | ✅ | Event bus, `Host`, manifest, dependency-ordered loader. | R11 |
| `plugin/dfcompat` | ✅ | Run unmodified dragonfly plugins via a `Handler` bridge. | R11 |
| `internal/inventory` | ⏳ | Windows, crafting, stations, recipes. | R4 |
| `plugins/official/multiprotocol` | ⏳ | Reference multiprotocol plugin (1.21 → 775 / 1.21.x → 975). | R12 |
| `cmd/versioncheck` | ✅ | Polls the Mojang version manifest against the `LWVersion` matrix; exits 0 on no-drift, 1 when upstream Java protocol is ahead. `--json` for CI gating. | R10 |
