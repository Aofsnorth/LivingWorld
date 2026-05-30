# LivingWorld — Design

**Status:** draft · **Spec set:** [REQUIREMENTS](REQUIREMENTS.md) · [DESIGN](DESIGN.md) · [../../TODO.md](../../TODO.md)
**Baseline:** Java `26.1` = protocol **775** (patched `third_party/go-mc`); Bedrock `1.26.20` = protocol **975** (`gophertunnel v1.56.2`).

## 1. Overview, goals & non-goals
LivingWorld is a single Go process that speaks **both** Minecraft wire protocols natively and serves **one shared
world** to Java and Bedrock clients — not a proxy (cf. Geyser) and not single-edition (cf. dragonfly/Paper).

**Goals:** vanilla parity; one canonical model with thin per-edition adapters; two flavors over one core; an explicit
version scheme; an easy, dragonfly-compatible plugin system; a multiprotocol plugin; an official anticheat.
**Non-goals (v1):** Realms, pack-authoring tooling, web panel, mod (Forge/Fabric) loading.

**Key principle:** *implement once on the canonical model; translate at the edges.* Edition/version specifics live
only in codec + translation layers; gameplay logic never branches on edition.

## 2. Architecture
```
                         ┌───────────────────────────────────────────────┐
                         │                    Core                        │
 Plugins ───────────────▶│  World · Entities · Inventory · Combat · Cmd   │◀─── Anticheat (R13)
 (events + Host, R11)    │  Players · Scheduler · EventBus · Registries    │
                         └───────────────▲───────────────▲────────────────┘
                                         │ canonical model│
                    ┌────────────────────┴───┐        ┌───┴────────────────────┐
                    │   Java edge (go-mc)     │        │  Bedrock edge (gopher) │
                    │  codec + state mapping  │        │  codec + runtime map   │
                    └──────────▲──────────────┘        └───────────▲────────────┘
                               │ version codecs (R10/R12)           │
                  Multiprotocol translators (1.21→775)   Multi-Bedrock (1.21.x→975)
                               │                                    │
                          [Java clients]                      [Bedrock clients]
```
Layers: **Edge** (per-edition wire I/O + ID mapping) → **Version/Translation** (codec registry + up/down-graders)
→ **Core** (edition-agnostic game logic) → **Extension** (plugins, anticheat as a privileged plugin).

## 3. Package layout
One canonical core with per-edition edges and pluggable subsystems. ✅ = landed in-tree; ⏳ = in-flight under
the workstreams in §3.1.
```
internal/
  registry/        # ✅ canonical types + Java↔Bedrock id maps                (R8)
  world/           # ✅ chunks, blocks, block-state props, tick state          (R3,R4)
  worldgen/        # ✅ noise · biome · terrain (surface/caves) → chunk        (R3)
  entity/          # ✅ entity Manager + pathfind (A*); AI/metadata next       (R5)
  combat/          # ✅ damage · armor/resist · knockback · criticals          (R5)
  anticheat/       # ✅ engine + checks (movement/combat/timing)               (R13)
  player/          # ✅ shared player model + controller routing               (R5,R6)
  item/ loot/ drops/ command/ skinbridge/ auth/   # ✅ supporting subsystems
  bedrock/* java/* # ✅ per-edition edges (current wire I/O + id mapping)       (R1)
  network/         # ⏳ edge transport + protocol bridge: codec + xlate        (R1,R10,R12)
  persistence/     # ⏳ world save/load + player data; pluggable Storage       (R3.4-3.6,R6.3)
  version/         # ⏳ LWVersion registry, negotiation, capability flags       (R10)
  inventory/       # ⏳ windows, crafting, stations, recipes                    (R4)
server/            # ✅ public API; cmd/server = Vanilla flavor                 (R9)
plugin/            # ✅ event bus, Host, manifest, loader                       (R11)
plugin/dfcompat/   # ✅ dragonfly compatibility adapter                         (R11)
plugins/official/multiprotocol/  # ⏳ the multiprotocol plugin                  (R12)
cmd/server/        # ✅ Vanilla flavor entrypoint
cmd/versioncheck/  # ⏳ polls manifest/changelog vs LWVersion matrix            (R10)
```
**`internal/network`** is the new home for the version-keyed `codec` (encode/decode + packet model) and the
multiprotocol `xlate` up/down-graders (§5, §8); the existing `bedrock/*` and `java/*` edges migrate behind it as
the protocol bridge matures. **`internal/persistence`** owns the pluggable `Storage` backend (Anvil/region
default, LevelDB optional — Decision (a)) plus player-data save/load; today's save code in `internal/world`
(`persistence.go`/`region.go`) moves here.

### 3.1 Parallel development workstreams
LivingWorld is built by up to **10 concurrent workstreams**, each owning a package lane (claim-before-edit on
shared files; scoped, per-lane commits — never `git add .`). The live board is `COORDINATION.md`; lane → package map:

| # | Lane | Package(s) |
|---|------|-----------|
| 1 | Coordination | `go.mod`/`go.sum`, board, review (no feature code) |
| 2 | Gameplay / edge | `internal/world`, `internal/player`, edge `entity_sync` |
| 3 | Worldgen | `internal/worldgen/**` |
| 4 | Foundation / entity / combat | `internal/registry`, `internal/entity/**`, `internal/combat` |
| 5 | Plugins | `plugin/**`, `plugin/dfcompat/**` |
| 6 | Anticheat | `internal/anticheat/**` |
| 7 | Network | `internal/network/**` (Java + Bedrock protocol bridge) |
| 8 | Persistence | `internal/persistence/**` (world save/load, player data) |
| 9 | Server / ops | `server/**`, `config/**`, ops layer |
| 10 | Docs / CI | `docs/**`, CI/testing, `ROADMAP.md` |

The original 6 lanes (1–6) were expanded with dedicated **network (7)**, **persistence (8)**, **server/ops (9)**,
and **docs/CI (10)** lanes once the canonical foundation (lane 4) landed and `internal/**` unlocked (2026-05-31).

## 4. Canonical data model (R4, R5, R8)
Edition-agnostic types; **block state ID = vanilla Java global state ID** is the canonical key.
```go
type BlockState uint32                  // canonical = Java global state id
type Pos struct{ X, Y, Z int }
type Vec3 struct{ X, Y, Z float64 }     // store f64; downcast to f32 for Bedrock

type Block struct { State BlockState; Entity *BlockEntity }
type ItemStack struct { ID string; Count uint8; Meta int16; Components NBT }
type Entity struct { ID int32; UUID uuid.UUID; Type string; Pos Vec3; Vel Vec3; Meta MetaMap; /*…*/ }
type Player struct { Entity; Edition Edition; XUID uint64; Inv *Inventory; GameMode GameMode; /*…*/ }
```
**ID mapping (`registry`):** generated tables `JavaState ⇄ BedrockRuntime` for blocks, `name ⇄ runtime` for items,
`type ⇄ network id + metadata layout` for entities. Source: bundled go-mc/dragonfly data → `go generate`. Any
unmapped id resolves to a logged sentinel (air / unknown) rather than dropping the packet.

## 5. Protocol & version system (R10, R1, R12)
```go
type Edition uint8 // Java, Bedrock
type Capability uint64 // bitset: signed chat, item components, transfer pkt, SAMov, …

type LWVersion struct {
    Label        string            // "26 (A)"
    Year, Letter string
    Java         EditionSupport    // protocol + []clientVersion
    Bedrock      EditionSupport    // protocol + []clientVersion
    Caps         Capability
    ChangelogURL string
}
type EditionSupport struct { Protocol int; Clients []string }

type Registry interface {
    Current() LWVersion
    All() []LWVersion
    Resolve(e Edition, protocol int) (*LWVersion, bool) // negotiation
}

// Version-keyed codec: the only place that knows wire layout.
type Codec interface {
    Edition() Edition
    Protocol() int
    Decode(r PacketReader) (Packet, error)
    Encode(p Packet, w PacketWriter) error
}
```
**Negotiation (connect sequence):**
```
client → edge: handshake/login (carries protocol)
edge → version.Resolve(edition, protocol)
  ├─ exact LWVersion          → use native codec
  ├─ in multiprotocol range   → use xlate translator chain → canonical (R12)
  └─ unsupported              → disconnect w/ version message (R1.5,R10.4)
→ Core.spawn(player) using canonical model
```
**Grouping rule (R10.3):** patches that don't change the wire protocol map to one LWVersion. Seed matrix:
`26 (A)` = Java 775 {26.1, 26.1.1, 26.1.2} × Bedrock 975 {1.26.20/.21/.23}. Cells verified via Mojang manifest +
the MC release changelog; `cmd/versioncheck` flags new patches and whether a new letter is needed.

## 6. Two flavors over one core (R9)
- **Shared core**: all gameplay + protocol code. Public API (`server`, `world`, `player`, `plugin`, `command`, `event`).
- **Vanilla** = `cmd/server`: constructs the server with vanilla defaults + all official plugins (anticheat on),
  config via `config.yml`/`server.properties`-equivalents. Zero user code.
- **Custom** = library: `server.New(cfg)` + builder hooks to register custom blocks/items/entities/commands/worldgen
  and handle every event (dragonfly-style ergonomics). **Identical protocol/feature paths** — flavors differ only in
  composition root + defaults, never in capability (R9.4).
```go
srv := server.New(server.DefaultConfig())     // Vanilla defaults
srv.Registry().RegisterBlock(myBlock)          // Custom extension point
srv.Plugins().OnPlayerJoin(func(e *plugin.PlayerJoinEvent){ /*…*/ })
srv.Run()
```

## 7. Plugin system + dragonfly compatibility (R11)
```go
type Event interface { Name() string; Cancellable() bool; Cancel() }
type Host interface {
    Broadcast(string); Message(name, msg string)
    World() world.API; Entities() entity.API; Inventories() inventory.API
    Scheduler() Scheduler; Store() KV; Perms() Perms; Commands() command.Registry
    Log(string, ...any)
}
type Plugin interface { Name() string; Version() string; OnEnable(Host) error; OnDisable() error }
```
- **EventBus**: typed, cancellable hooks for every gameplay action (join/leave/move/break/place/attack/damage/death/
  chat/command/container/drop/pickup/interact/respawn). Handlers run with panic isolation; a panicking plugin is
  disabled, not fatal.
- **Manifest** (`plugin.yml`): name, version, api-version, deps/soft-deps, permissions, entrypoint. Loader resolves
  order; refuses api-version mismatched with the running LWVersion (R11.7).
- **Loading model decision:** primary = **compile-time registration** (dragonfly-style; cross-platform incl. Windows,
  where Go `-buildmode=plugin` is unavailable). Optional `-buildmode=plugin` on Linux/macOS and an optional embedded
  script runtime are secondary. Documented trade-offs in PLUGIN_API v2.
- **`plugin/dfcompat`:** implements dragonfly's `player.Handler`/`world`/`cmd`/registration surfaces against
  LivingWorld events so a dragonfly plugin runs **unmodified**; block/item defs are translated into `registry`
  (dual-edition). Bedrock-shaped APIs are bridged to Java clients via the canonical model. Compatibility scope +
  known gaps are documented; CI runs 2–3 real dragonfly sample plugins.

## 8. Multiprotocol multi-version plugin (R12)
Built on §5 + §7. Provides `xlate` translator chains so off-canonical clients still join.
```go
type Translator interface {
    Edition() Edition
    From() int                 // client protocol
    Up(p Packet) []Packet       // client → canonical
    Down(p Packet) []Packet     // canonical → client
}
type Chain []Translator // composed when client is N steps from canonical
```
Stages handled per step: **registry remap**, **chunk/section re-encode**, **item-component ⇄ legacy NBT**,
**entity-metadata remap**, **command-tree shaping**. Java path = ViaVersion-style version translators (1.21 → 775);
Bedrock path = multi-protocol across 1.21.x → 975. Per-step diffs documented under `docs/protocol/`. Negotiation
gates by configured min/max; missing capabilities degrade gracefully (R12.6) instead of disconnecting.

## 9. Official anticheat (R13)
Runs as a privileged in-tree plugin with pre-movement/pre-action hooks.
```go
type CheckResult struct { Vio float64; Reason string; Mitigate Mitigation }
type Check interface {
    Name() string
    Inspect(ctx *PlayerCtx, ev Event) CheckResult   // pure; no side effects
}
type Profile struct { Score map[string]float64; Decay float64 } // per player, per check
```
- **Server-authoritative movement**: enable Bedrock SAMov; reconcile Java positions → single source of truth.
- **Check families**: movement (Speed/Fly/NoFall/Step/NoClip/Timer), combat (Reach/Autoclicker/KillAura/aim/AntiKB/
  criticals), world (Nuker/FastBreak/Scaffold/illegal-reach/fast-use/illegal-item), packet/timing (order/BadPackets/
  idle-while-acting). Each is a pure `Check`; the engine aggregates weighted violations with decay.
- **Staged actions**: log → warn → setback → kick → ban, configurable per check (`anticheat.yml`) + `/ac` admin
  commands + exemptions. **Lag/TPS compensation** widens tolerances under latency. Emits anticheat events to plugins;
  writes structured violation logs. A false-positive regression corpus gates releases (R15).

## 10. World engine (R3, R4)
- **Tick loop** per world @20 TPS with budgeted phases (player input → block/random ticks → entity AI/physics →
  fluid/redstone → network flush). Over-budget work defers to next tick.
- **Chunk lifecycle**: ticket-based load/unload; view vs simulation distance; async generation + I/O off-tick.
- **Worldgen pipeline**: `Biomes → Noise/Surface → Carvers → Features → Structures` per dimension (Overworld/Nether/
  End), seeded deterministically so Java & Bedrock match (R3.3).
- **Lighting**: sky + block light engine with incremental propagation.
- **Persistence** (`internal/persistence`): pluggable `Storage` backend (region/Anvil-style default, LevelDB optional);
  async autosave; world lock; corrupt-chunk quarantine + recovery (R3.6); player-data save/load (R6.3); final save on
  shutdown. (Extracted from today's `internal/world/persistence.go`+`region.go`.)

## 11. Concurrency & performance (R14)
One goroutine owns each world's tick state (no shared mutable world state across worlds); cross-world/player ops use
message passing. Network read/write, chunk gen, and disk I/O run on worker pools with backpressure. AOI/viewer system
bounds per-player packet fan-out. Target ≥100 mixed players ≥19 TPS; profiling + alloc regression gates in CI.

## 12. Error handling & resilience
- Per-connection failures isolate to that session; never crash the tick loop.
- Plugin panics → plugin disabled + event logged.
- Decode/map failures → sentinel value + structured log, never silent drop.
- Corrupt world data → quarantine + continue.
- All `Start()`/`Stop()` paths are idempotent and save-safe.

## 13. Testing strategy (R15)
- **Unit**: codecs (golden round-trip + fuzz), id maps, recipe/loot tables, damage math, redstone/fluids.
- **Integration**: real Java + Bedrock client connect/join/play per supported version.
- **Parity harness**: same action via JP vs BP → assert equal canonical state (R8.3).
- **Anticheat**: cheat/exploit corpus (detection) + legit-play corpus (false-positive gate).
- **Load**: ≥100-bot mixed-edition soak + TPS/alloc profiling.
- **CI matrix**: build · vet · `govulncheck` · per-LWVersion connect tests; pin `go-raknet` to a tag; build on Go 1.26.3.

## 14. Security
Online-mode auth per edition (R2); secrets never logged; network endpoints document auth posture; dependency hygiene
via `govulncheck` (current: bump to Go 1.26.3 closes 5 reachable stdlib advisories; `go-jose`/`x/net` already bumped).

## 15. Rollout & open questions
**Phases:** §7 hygiene + version scheme → core vanilla parity (net→world→blocks/items→entities→combat→systems) →
flavors → plugins → multiprotocol → anticheat.
**Decisions (2026-05-31):**
- **(a) Persistence → pluggable `world.Storage`, default Anvil/region.** Canonical block id = Java global state id, so Anvil maps 1:1, reuses the vendored go-mc `save` code, and stays inspectable with standard NBT/region tooling; LevelDB (`df-mc/goleveldb`, already vendored) remains an optional write-heavy backend. *("the best one")*
- **(b) Worldgen → config-selected.** `world.type` picks the `WorldGenerator` at startup (`superflat` | `vanilla-parity` | `custom`/plugin-provided); each generator stays deterministic per seed. *("depends on the config")*
- **(c) Plugin DX → optimize for community ease.** Keep compile-time Go plugins (full power + dfcompat) as the primary path, AND add an embedded scripting runtime (candidate: Risor / Tengo / Starlark / Lua — TBD) plus a scaffold + hot-reload so quick/non-Go plugins are trivial. *("whatever makes it easiest for the community")*

**Still open:** (d) how far back Bedrock multiprotocol realistically reaches given gophertunnel's single-version design.
