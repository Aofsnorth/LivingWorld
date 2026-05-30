# LivingWorld Roadmap

**Where the project is and what's next.** Architecture: [specs/DESIGN.md](specs/DESIGN.md) · scope: [specs/REQUIREMENTS.md](specs/REQUIREMENTS.md) · live board: [../COORDINATION.md](../COORDINATION.md).

**Baseline:** `LivingWorld 26 (A)` = Java **protocol 775** (patched `third_party/go-mc`) × Bedrock **protocol 975** (`gophertunnel v1.56.2`). Patches sharing a wire protocol group under one LWVersion (R10).

---

## Current phase — Integration (greenfield lanes ✅ → wiring 🔄)

The project is built by **10 parallel workstreams** (see DESIGN §3.1). The greenfield foundation landed and `go build ./...` is green; work is now integrating those lanes into the shared `internal/world`/`internal/player` core.

### ✅ Landed
- **Crossplay core** — one shared world for Java (775) + Bedrock (975); movement, block place/break, drop physics, equipment sync.
- **Foundation** — `internal/registry`: canonical `BlockState`/`Pos`/`Vec3`/`ItemStack`/`Entity` + Java↔Bedrock id maps.
- **Block/item registries** — full vanilla 26.1 palette (~29.8k block states, ~1.5k items) via bundled go-mc data.
- **Worldgen primitives** — `worldgen/noise` (seedable Perlin/fBm), `worldgen/biome` (climate select), `worldgen/terrain` (surface + caves) — deterministic per seed.
- **Entity + combat** — `internal/entity` Manager + `entity/pathfind` (A*); `internal/combat` armor/resistance/knockback/criticals.
- **Anticheat engine** — weighted violation model + decay + staged actions; Speed/Reach/Timer/Autoclicker/KillAura checks.
- **Plugin API** — typed cancellable events, panic-isolated dispatch, `plugin.yml` manifest + dependency-ordered loader, `plugin/dfcompat` dragonfly bridge.
- **World persistence** — region/Anvil-style `r.<rx>.<rz>.lwr` files (32×32, gzip, atomic), autosave + final save on shutdown.

### 🔄 In progress (integration points)
- Worldgen → `*world.Chunk` glue (resolve names via `world.StateID`).
- Canonical entity ↔ edge `entity_sync` delta encoding.
- Rich plugin `Host.World()`/`Entities()`/`Commands()` surface.
- `world.Hardness`/`BreakTicks` → anticheat FastBreak/Nuker checks.
- Canonical `Player` → `dfcompat` player adapter.

---

## Next milestones

### M1 — Network package (R1, R10, R12)
- [ ] Carve `internal/network`: version-keyed `codec` (packet model, encode/decode) + multiprotocol `xlate`.
- [ ] `internal/version`: `LWVersion` registry + protocol negotiation + capability flags.
- [ ] Migrate `bedrock/*` / `java/*` edges behind the protocol bridge.
- [ ] `cmd/versioncheck`: poll Mojang manifest/changelog against the LWVersion matrix.

### M2 — Persistence package (R3.4–3.6, R6.3)
- [ ] Extract `internal/persistence` from `internal/world` (`persistence.go`/`region.go`).
- [ ] Pluggable `Storage` interface — Anvil/region default, optional LevelDB backend (Decision a).
- [ ] Per-player data save/load (inventory, position, gamemode, XP).
- [ ] Corrupt-chunk quarantine + recovery; world lock.

### M3 — World & gameplay parity (R3, R4)
- [ ] Worldgen pipeline → real chunks; vanilla-parity + config-selected generators (Decision b).
- [ ] Nether/End dimensions + portals; lighting propagation.
- [ ] Full inventory/container sync, crafting/smelting/enchanting, block entities, redstone, fluids.

### M4 — Entities & combat completion (R5)
- [ ] Mob AI goals/targeting on `pathfind`, spawning rules, breeding, trading.
- [ ] Projectiles, XP orbs, vehicles; entity metadata sync both editions.
- [ ] Status effects, shields; finish anticheat world/aim families (Fly/NoFall/Scaffold/AntiKB).

### M5 — Server/ops & flavors (R9, R14)
- [ ] `server/` public API hardening + ops/config layer; Vanilla vs Custom flavors share one code path.
- [ ] Metrics, profiling (auth-gated), crash diagnostics under `diag/`; graceful start/stop, RCON/console, Docker.

### M6 — Multiprotocol plugin (R12)
- [ ] `xlate` translator chains: Java 1.21 → 775, Bedrock 1.21.x → 975.
- [ ] Configurable min/max range + per-version toggles; graceful feature degradation.

### M7 — Quality gates for 1.0 (R15)
- [ ] CI matrix: build · vet · `govulncheck` · per-LWVersion connect tests.
- [ ] Cross-edition parity harness; anticheat false-positive regression corpus.
- [ ] ≥100-bot mixed-edition load soak ≥19 TPS; coverage trending >80%.

---

## Future ideas
- Embedded plugin scripting runtime + scaffold/hot-reload (Decision c).
- BungeeCord/Velocity-style cross-server; plugin repository/manager.
- Web admin panel; resource/behavior-pack support; Realms-style hosting.
