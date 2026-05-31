# LivingWorld Package Reference

Doc-coverage index for every package, mapped to its workstream lane (DESIGN [§3](specs/DESIGN.md) / §3.1) and requirements.
**✅** = landed, carries a `// Package` doc comment. **⏳** = planned; documented here until its code (and `doc.go`) lands.

> Convention: every landed package opens with a `// Package <name> …` comment (verified across the tree). New
> packages add that comment with their first file; this index is the human-facing summary.

## Core / canonical (lanes 2, 4)
| Package | St | Summary | Req |
|---|----|---------|-----|
| `internal/registry` | ✅ | Canonical edition-agnostic types (`BlockState`/`Pos`/`Vec3`/`ItemStack`/`Entity`) + Java↔Bedrock id maps. | R8 |
| `internal/world` | ✅ | Chunks, blocks, block-state properties, tick state, crack tracking. | R3,R4 |
| `internal/player` | ✅ | Shared player model + cross-edition controller routing. | R5,R6 |
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
| `internal/network` | ⏳ | **Java + Bedrock protocol bridge.** Version-keyed `codec` (packet model, encode/decode) + multiprotocol `xlate` up/down-graders; the `java/*`/`bedrock/*` edges migrate behind it. | R1,R10,R12 |
| `internal/version` | ⏳ | `LWVersion` registry, protocol negotiation, capability flags. | R10 |

## Persistence (lane 8)
| Package | St | Summary | Req |
|---|----|---------|-----|
| `internal/persistence` | ⏳ | **World save/load + player data.** Pluggable `Storage` (Anvil/region default, LevelDB optional — Decision a); autosave, world lock, corrupt-chunk quarantine; per-player data. Extracted from `internal/world/{persistence,region}.go`. | R3.4-3.6,R6.3 |
| `internal/worldconvert` | ✅ | Convert vanilla **Java Anvil** (`region/*.mca`) ⇄ LivingWorld region format (`region/r.*.lwr`), pivoting on block name (LivingWorld id == Java state id). Driven by `cmd/worldconvert`. Bedrock LevelDB path stubbed. | R3.4 |

## Public API & extensions (lanes 5, 9)
| Package | St | Summary | Req |
|---|----|---------|-----|
| `server` | ✅ | Public embeddable API (`server.New`/`Run`/`Host`); `cmd/server` = Vanilla flavor. | R9 |
| `plugin` | ✅ | Event bus, `Host`, manifest, dependency-ordered loader. | R11 |
| `plugin/dfcompat` | ✅ | Run unmodified dragonfly plugins via a `Handler` bridge. | R11 |
| `internal/inventory` | ⏳ | Windows, crafting, stations, recipes. | R4 |
| `plugins/official/multiprotocol` | ⏳ | Reference multiprotocol plugin (1.21 → 775 / 1.21.x → 975). | R12 |
| `cmd/versioncheck` | ⏳ | Polls Mojang manifest/changelog against the LWVersion matrix. | R10 |
