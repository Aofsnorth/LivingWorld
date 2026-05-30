# LivingWorld — Master TODO

**Goal:** evolve LivingWorld into a fully **playable, vanilla-parity, dual-native** Minecraft server
(Java **and** Bedrock from one Go backend, one shared world) and ship it as **two flavors**
(Vanilla & Custom/library), under its **own native version scheme**, with a
**dragonfly-compatible community plugin system**, an **official multiprotocol multi-version plugin**,
and an **official anticheat**.

> This is not a wishlist — every protocol-touching item below carries explicit **research → implement → test**
> sub-tasks. Track work top-to-bottom; do not mark a step done until its tests pass.

**Spec:** this file is the *task plan*. See [docs/specs/REQUIREMENTS.md](docs/specs/REQUIREMENTS.md) for the **what** (EARS acceptance criteria) and [docs/specs/DESIGN.md](docs/specs/DESIGN.md) for the **how** (architecture, interfaces, data models).

**Legend:** `[ ]` todo · `[~]` in progress · `[x]` done · **(R)** research · **(I)** implement · **(T)** test

---

## 0. Verified current state (2026-05-31)

- **Java**: patched `third_party/go-mc` → `ProtocolVersion = 775` (MC Java **26.1**). Upstream go-mc is at 767, so our fork is hand-maintained.
- **Bedrock**: `gophertunnel v1.56.2` → `CurrentProtocol = 975`, `CurrentVersion = "1.26.20"`.
- **World**: superflat only; per-chunk gzip persistence; no biomes/caves/structures; overworld only.
- **Gameplay**: movement + block place/break + basic block conversion; no entities/mobs/combat/inventory-sync.
- **Security**: deps bumped — `go-jose v4.1.4`, `golang.org/x/net v0.53.0` (closes GO-2026-4945 / -4918). Still TODO: build on **Go 1.26.3** to close 5 reachable stdlib vulns.
- **Plugins**: basic typed events + `Host` surface only.

---

## 1. Vanilla-parity playable feature set

The bar: a fresh player on either edition can join, survive, build, fight, progress, and persist — matching vanilla behavior.

### 1.1 Protocol & networking completeness
- [ ] **(R)** Inventory every clientbound/serverbound packet for Java 775 and Bedrock 975; build a coverage matrix (`docs/packet-coverage.md`) marking handled / stubbed / missing.
- [ ] **(I)** Java: full Configuration phase, registry sync, tags, feature flags, login→config→play transitions.
- [ ] **(I)** Java: KeepAlive, Ping, transfer, resource-pack push, cookies, server links.
- [ ] **(I)** Bedrock: full login chain (handshake → resource packs → start game → biome/creative/registry payloads → chunk radius → play status).
- [ ] **(I)** Robust packet (de)serialization with version-aware codecs (feeds §3).
- [ ] **(T)** Fuzz packet decoders; golden-file round-trip tests for both editions.

### 1.2 Authentication, encryption & sessions
- [ ] **(I)** Java online mode: Mojang/Yggdrasil auth, AES-CFB8 encryption, compression threshold, profile/properties (skins).
- [ ] **(I)** Bedrock online mode: full Xbox Live chain (PlayFab/XSAPI), encryption, chain-of-trust verification (`go-jose`).
- [ ] **(I)** Offline/LAN modes, allow/deny lists, ops, ban list, IP bans (vanilla parity files).
- [ ] **(I)** Session lifecycle: join/leave, kick, timeout, reconnect, duplicate-login handling.
- [ ] **(T)** Auth integration tests with real clients (both editions).

### 1.3 World generation
- [ ] **(R)** Decide canonical worldgen source (port vanilla noise/biome params vs. seedable custom that matches both editions).
- [ ] **(I)** Overworld: biomes, terrain noise, surface rules, caves (incl. noise caves), ravines, ore distribution.
- [ ] **(I)** Features: trees/vegetation, structures (villages, mineshafts, strongholds, temples, ocean monuments, ancient cities, fortresses, end cities).
- [ ] **(I)** Nether & End generation (incl. End islands + dragon arena).
- [ ] **(I)** Deterministic seed parity so Java & Bedrock clients see identical terrain.
- [ ] **(T)** Seed reproducibility + cross-edition chunk-equality tests.

### 1.4 World storage, chunk format & lighting
- [ ] **(I)** Canonical chunk model: 24 sections (−64..319), block/biome palettes, block entities, heightmaps.
- [ ] **(I)** Lighting engine (sky + block light) with propagation + updates.
- [ ] **(I)** Persistence at scale: region/Anvil-style or LevelDB-backed; async autosave; world locking; corruption recovery.
- [ ] **(I)** Tick-based chunk lifecycle: load/unload, view vs. simulation distance, ticket system.
- [ ] **(T)** Save/load fidelity + crash-safety tests.

### 1.5 Blocks, block entities, physics, redstone & fluids
- [ ] **(R)** Map full block-state behavior tables for 775 (place rules, supports, waterlogging, faces).
- [ ] **(I)** Block update/neighbor-notify system; random ticks (crops, fire, leaves, ice).
- [ ] **(I)** Fluids (water/lava flow, source/level, waterlogging, interactions).
- [ ] **(I)** Redstone (power, dust, repeaters, comparators, pistons, observers, rails) — full logic.
- [ ] **(I)** Block entities (chests, furnaces, hoppers, signs, spawners, beacons, brewing stands, etc.).
- [ ] **(I)** Gravity blocks, fire spread, explosions (TNT/creeper), block-break particles/sounds.
- [ ] **(T)** Redstone + fluid behavior regression suites.

### 1.6 Items, inventory, crafting & stations
- [ ] **(I)** Full inventory model + windows (player, chest, double chest, ender chest, shulker).
- [ ] **(I)** Item components/NBT, stack rules, durability, tools vs. block mining speed + correct drops.
- [ ] **(I)** Crafting (shaped/shapeless + recipe book), smelting/blasting/smoking, smithing, stonecutting.
- [ ] **(I)** Enchanting table + anvil + grindstone + brewing/potions + loom/cartography.
- [ ] **(I)** Creative inventory + item-give parity; **cross-edition inventory sync**.
- [ ] **(T)** Recipe coverage tests vs. vanilla recipe data.

### 1.7 Entities, AI & spawning
- [ ] **(R)** Entity metadata index for both editions (775 / 975); ID allocation strategy.
- [ ] **(I)** Entity base: spawn/despawn, movement/interpolation, metadata sync, collisions, physics.
- [ ] **(I)** Players-as-entities visible cross-edition (skins via skinbridge, poses, equipment).
- [ ] **(I)** Mobs: passive/neutral/hostile with AI goals + pathfinding (A*), targeting, breeding.
- [ ] **(I)** Spawning rules (light/biome/cap/spawn-eggs), despawn, persistence.
- [ ] **(I)** Villagers + trading, projectiles, dropped-item entities, XP orbs, vehicles (boats/minecarts/horses).
- [ ] **(T)** Pathfinding + spawn-rule tests.

### 1.8 Combat, health & status effects
- [ ] **(I)** Health/damage/death/respawn, fall/fire/drowning/void/cactus/lava damage.
- [ ] **(I)** Attack mechanics: cooldown, reach, crit, knockback, sweep, armor/toughness, shields.
- [ ] **(I)** Status effects + potions; hunger/saturation/regen; PvP + mob combat parity.
- [ ] **(T)** Damage-math parity tests vs. vanilla.

### 1.9 Player mechanics & progression
- [ ] **(I)** Movement validation (server-authoritative — see §6), sprint/sneak/swim/crawl/climb/elytra/levitation.
- [ ] **(I)** Gamemodes (survival/creative/adventure/spectator) + abilities (fly/instabreak/invuln).
- [ ] **(I)** XP/levels, hunger, sleep/skip-night + spawn point/anchor, inventory persistence per player.
- [ ] **(I)** Advancements/achievements, statistics, recipe unlocks.

### 1.10 Gameplay systems
- [ ] **(I)** Day/night + game-time, weather (rain/thunder), difficulty (peaceful→hard), gamerules.
- [ ] **(I)** Scoreboard/objectives, teams, bossbars, custom map/world data.
- [ ] **(I)** Mob spawning director, mob griefing rules, world border.

### 1.11 Commands & permissions
- [ ] **(I)** Brigadier command graph for Java (argument types, suggestions, error formatting) + Bedrock command sync.
- [ ] **(I)** Vanilla command set: `/give /tp /gamemode /time /weather /effect /summon /setblock /fill /clone /gamerule /op /kick /ban /whitelist /difficulty /xp /enchant /clear /kill /spawnpoint /worldborder …`.
- [ ] **(I)** Permission/op levels + per-command gating (extensible by plugins, §4).
- [ ] **(T)** Command-parse + permission tests.

### 1.12 Chat, sounds, particles & UI
- [ ] **(I)** Java signed chat (1.19+ session keys) + system/disguised messages; Bedrock text packets.
- [ ] **(I)** Cross-edition chat normalization (`§`/JSON ⇄ Bedrock rawtext) + filtering hooks.
- [ ] **(I)** Sound + particle emission (positional, per-edition IDs); titles/actionbar/tab list.
- [ ] **(I)** **Bedrock UI forms** (modal/menu/custom) abstraction for menus.

### 1.13 Dimensions & portals
- [ ] **(I)** Nether + End dimensions; Nether/End portal build+ignite+travel + coordinate scaling + portal cooldown.
- [ ] **(I)** Ender dragon fight + respawn; End gateways; bed/anchor explosions in wrong dims.

### 1.14 Cross-edition parity & conversion
- [ ] **(R)** Maintain Java-state-ID ⇄ Bedrock-runtime-ID tables for the **full** palette (block + item + entity); auto-generate from bundled data.
- [ ] **(I)** Behavior parity layer so a feature added once works for both clients (movement, combat, inventory, sounds).
- [ ] **(T)** Automated parity harness: same action on Java vs. Bedrock client → equal world state.

### 1.15 Performance & concurrency
- [ ] **(I)** Per-world tick loop @ 20 TPS; entity/chunk ticking budgets; threaded chunk gen + I/O.
- [ ] **(I)** Network batching/compression tuning; viewer/AOI system; backpressure.
- [ ] **(T)** Load test (target ≥100 mixed-edition players); TPS/alloc profiling + regression gate.

---

## 2. Two server flavors

Both flavors share **one core** (`internal/*` + public `server`, `world`, `plugin` packages). They differ only in the front-end and defaults — **protocol and feature completeness are identical**.

### 2.1 Shared core
- [ ] **(I)** Finalize public API surface (`server`, `world`, `player`, `plugin`, `command`, `event`) — nothing under `internal/` required by consumers.
- [ ] **(I)** Stable config schema + capability/`Host` interfaces both flavors build on.

### 2.2 Vanilla server (turn-key)
- [ ] **(I)** `cmd/server` ships a faithful **vanilla experience** with zero code: survival defaults, vanilla worldgen, full command set, anticheat on.
- [ ] **(I)** `server.properties`-style + `config.yml` parity (motd, gamemode, difficulty, view/sim distance, online-mode, whitelist, etc.).
- [ ] **(I)** First-run UX: EULA prompt, auto world creation, console + RCON, graceful save on stop.
- [ ] **(T)** "Boot → join (both editions) → play → restart → state persists" smoke test.

### 2.3 Custom server (dragonfly-style library)
- [ ] **(I)** `import "livingworld/server"` builds a fully programmable server while keeping **full protocol + features** (mirror dragonfly's library ergonomics).
- [ ] **(I)** Handler/hook surface for every gameplay event (join, move, break, place, attack, damage, chat, command, container, drop, death…).
- [ ] **(I)** Composable building blocks: register custom blocks/items/entities/commands/worldgen/recipes at compile time.
- [ ] **(I)** Examples in `examples/`: minigame skeleton, custom-item, custom-worldgen, protected-region.
- [ ] **(I)** Docs: "Vanilla vs Custom — when to use which" + migration guide.

---

## 3. LivingWorld version scheme + protocol R&D

Because LivingWorld is **dual-native** (not built on a single Java *or* Bedrock base), it needs its **own** version identity that maps to a *set* of compatible client patches per edition.

### 3.1 Versioning model
- **Name:** `LivingWorld <YY> (<Letter>)`, e.g. **`LivingWorld 26 (A)`**. `YY` = Minecraft year line; `Letter` increments when a wire protocol changes within the year.
- **Grouping rule:** one LivingWorld version covers **all MC patches that share the same wire protocol** on each edition. Hotfixes that don't bump the protocol fold into the same version — **no new server version needed**.
- **Pairing:** a LivingWorld version = `(Java protocol N + set of Java patches)` × `(Bedrock protocol M + set of Bedrock patches)`.
- [ ] **(I)** Implement a `version` package: a `LWVersion` registry (id, label, Java proto + client list, Bedrock proto + client list, release notes link).
- [ ] **(I)** Expose `livingworld --version` + in-game `/lwversion` showing supported clients.

### 3.2 Version matrix (seed — extend via §3.3 research)
Source of truth for patch lists: <https://feedback.minecraft.net/hc/en-us/sections/360001186971-Release-Changelogs>

| LivingWorld | Java proto | Java clients | Bedrock proto | Bedrock clients | Status |
|---|---|---|---|---|---|
| **26 (A)** | **775** ✅ | 26.1, 26.1.1, 26.1.2 | **975** ✅ | 1.26.20, 1.26.21, 1.26.23 (≈ 26.20/26.21/26.23) | primary target |
| 26 (A?) — verify | 775? | (any further 26.1.x hotfix) | 975? | 26.13 and nearby hotfixes — confirm proto | **(R)** |
| 25 (legacy) | 767 (upstream) | 1.21.x (e.g. 1.21.11 "Mounts of Mayhem") | TBD | 1.21.130–1.21.132 line | **(R)** back-compat |

- [ ] **(R)** Fill every cell: confirm exact protocol number per patch (Java via go-mc/wiki; Bedrock via gophertunnel release ↔ protocol history) and which patches truly share a protocol.
- [ ] **(I)** Encode the finalized matrix in the `version` registry + a generated `docs/VERSION_MATRIX.md`.

### 3.3 Protocol research per edition/patch
- [ ] **(R)** Java: diff packet sets/registry between consecutive 26.x patches; note which hotfixes are protocol-identical.
- [ ] **(R)** Bedrock: map each `gophertunnel` tag → `CurrentProtocol`/`CurrentVersion`; identify the patch span each protocol covers.
- [ ] **(R)** Produce `docs/protocol/<edition>-<proto>.md` per supported protocol (packet list, deltas, quirks).

### 3.4 Multi-protocol negotiation & adapters
- [ ] **(I)** Handshake/version detection per edition → resolve to a `LWVersion`; reject/redirect unsupported with a clear message.
- [ ] **(I)** Version-keyed packet codec registry (per-protocol encode/decode) so the core speaks one internal model.
- [ ] **(I)** Capability flags per version to gate features that only exist in some patches.
- [ ] **(T)** Connect each listed client version in the matrix → successful join (CI matrix where feasible).

### 3.5 Maintenance workflow
- [ ] **(I)** A `cmd/versioncheck` tool that polls the Mojang manifest + changelog and reports new patches + whether they need a new LivingWorld letter.
- [ ] **(I)** Release checklist: new patch → research proto → extend matrix → bump/confirm `LWVersion` → regression run.

---

## 4. Plugin system (dragonfly-compatible + community-friendly)

Goal: **reuse the dragonfly plugin/handler ecosystem** *and* make writing LivingWorld plugins trivial for the community.

### 4.1 Core plugin API
- [ ] **(I)** Expand typed events to cover **every** gameplay hook from §1 (join/leave/move/break/place/attack/damage/death/chat/command/container/drop/pickup/interact/respawn…), all cancellable where vanilla allows.
- [ ] **(I)** `Host` capability surface: world edit, entity spawn, inventory ops, scheduler/tasks, persistent per-plugin storage, permissions, command registration.
- [ ] **(I)** Plugin lifecycle: load order, dependencies + soft-depends, enable/disable, isolation of panics.

### 4.2 dragonfly compatibility layer
- [ ] **(R)** Map dragonfly's surfaces (`player.Handler`, `world` events, `cmd`, `item`/`block` registration, `world.Block`/`Item`) onto LivingWorld equivalents; list gaps.
- [ ] **(I)** Adapter package `plugin/dfcompat`: implement dragonfly's `Handler` interfaces so an existing dragonfly handler/plugin runs **unmodified** (Bedrock semantics) and is bridged to Java clients too.
- [ ] **(I)** Translate dragonfly block/item definitions into LivingWorld's dual-edition registry.
- [ ] **(T)** Take 2–3 real dragonfly sample plugins → run on LivingWorld unchanged.
- [ ] **(I)** Document compatibility scope + known differences.

### 4.3 Community developer experience
- [ ] **(I)** `plugin.yml`/manifest (name, version, api-version, deps, permissions, entrypoint).
- [ ] **(R)** Decide loading model: compile-time registration (dragonfly-style, cross-platform) vs. Go `-buildmode=plugin` (Linux/macOS only) vs. optional embedded scripting — pick a primary + document trade-offs.
- [ ] **(I)** Hot reload / enable-disable without full restart (where the model allows).
- [ ] **(I)** `create-livingworld-plugin` template/scaffold + `examples/` gallery + full `docs/PLUGIN_API.md` v2.
- [ ] **(I)** Optional plugin registry/index + version-compat metadata (which `LWVersion` a plugin supports).

---

## 5. Official plugin — Multiprotocol multi-version (1.21 → latest)

A first-party plugin (built on §3 + §4) that lets one server accept **MC 1.21 up to the latest** on both editions, translating older/newer clients to the core's canonical version (think native ViaVersion **+** multi-Bedrock, no external proxy).

- [ ] **(R)** Enumerate every protocol from Java **1.21 → 775** and Bedrock **1.21.x → 975**; build per-step diff docs (packets, registries, chunk format, item components, entity metadata).
- [ ] **(I)** Translation pipeline: per-version up/down-graders chained to the core model (registry remap, chunk/section re-encode, item-component ⇄ legacy NBT, entity-metadata remap, command-tree shaping).
- [ ] **(I)** Java path: ViaVersion-style version translators registered through the §4 plugin API.
- [ ] **(I)** Bedrock path: multi-protocol support across the 1.21.x → 26.x Bedrock range.
- [ ] **(I)** Version negotiation + graceful "unsupported version" messaging; feature-gating for clients missing a capability.
- [ ] **(I)** Config: min/max allowed versions, per-version toggles.
- [ ] **(T)** Connect a spread of client versions (1.21, mid, latest) on both editions → join + basic play verified in CI matrix.
- [ ] **(I)** Ship as the reference example of the plugin API's power.

---

## 6. Official anticheat

First-party, **server-authoritative** anticheat. Bedrock defaults to client-auth movement — switch to **server-authoritative movement** and validate both editions uniformly. Tunable, low false-positive, plugin-observable.

### 6.1 Foundations
- [ ] **(I)** Server-authoritative movement (enable Bedrock SAMov; reconcile Java position) — single source of truth for player state.
- [ ] **(I)** Per-player violation model: weighted checks, decay, thresholds, staged actions (log → warn → setback → kick → ban).
- [ ] **(I)** Latency/lag compensation + TPS awareness to avoid false positives.

### 6.2 Movement checks
- [ ] **(R/I)** Speed/Fly/NoFall/Step/Jesus/Spider/NoClip/Phase, Timer (packet rate vs. wall clock), invalid motion/teleport, vehicle speed.

### 6.3 Combat checks
- [ ] **(R/I)** Reach, attack-speed/Autoclicker, KillAura/multi-aura, aim/rotation (snap, GCD/angle), hitbox/AntiKB, criticals validation.

### 6.4 World-interaction checks
- [ ] **(R/I)** Nuker/FastBreak (break time vs. tool+state), Scaffold/Tower, illegal reach for place/interact, fast-use (eat/bow), container/inventory action rate + illegal slot ops, illegal item/NBT.

### 6.5 Packet & timing checks
- [ ] **(R/I)** Packet order/validity, BadPackets (duplicate/impossible flags), order of move vs. action, idle-while-acting; protocol-specific abuse per `LWVersion`.

### 6.6 Mitigation, config, events, tuning
- [ ] **(I)** Mitigations: setback, velocity correction, action cancel.
- [ ] **(I)** `anticheat.yml` (per-check enable/threshold/action) + `/ac` admin commands (alerts, verbose, exempt).
- [ ] **(I)** Emit anticheat events through the §4 plugin API; alert routing + structured violation logs.
- [ ] **(T)** Cheat-client / scripted-exploit corpus → detection rate + **false-positive regression** suite; per-edition tuning.

---

## 7. Cross-cutting (do alongside everything)

- [ ] **(I)** Build on **Go 1.26.3** to clear the 5 reachable stdlib vulns (GO-2026-4971/4947/4946/4870/4866); add `govulncheck` to CI.
- [ ] **(I)** Raise test coverage from ~5 test files toward the ROADMAP's >80% (unit + integration + parity + load).
- [ ] **(I)** CI matrix: build + vet + vulncheck + per-`LWVersion` connect tests; pin `go-raknet` to a tagged release.
- [ ] **(I)** Keep `docs/PROTOCOL.md` & `docs/VERSIONS.md` in sync with reality (currently stale at 764/1.20.2 → should be **775 / 975**).
- [ ] **(I)** Observability: metrics, profiling endpoints (auth-gated), crash diagnostics in `diag/`.
- [ ] **(I)** Deployment: Docker image, sample compose, ops runbook, backup/restore.

---

### Suggested execution order
**§7 hygiene + §3 version scheme** → **§1 vanilla parity** (networking → world → blocks/items → entities → combat → systems) → **§2 flavors** → **§4 plugins** → **§5 multiprotocol plugin** → **§6 anticheat**, with tests landing beside each feature.
