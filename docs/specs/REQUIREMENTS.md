# LivingWorld — Requirements

**Status:** draft · **Spec set:** [REQUIREMENTS](REQUIREMENTS.md) · [DESIGN](DESIGN.md) · [../../TODO.md](../../TODO.md)

## 1. Introduction

### 1.1 Purpose
Define *what* LivingWorld must do to be a fully playable, vanilla-parity, **dual-native** Minecraft
server (Java **and** Bedrock from one Go backend, one shared world), shipped in two flavors, with its own
version scheme, a dragonfly-compatible plugin system, an official multiprotocol plugin, and an official anticheat.

### 1.2 Scope
In scope: connectivity, auth, world, blocks/items/inventory, entities/combat, player progression, gameplay
systems, cross-edition parity, the two server flavors, the LivingWorld version scheme, plugins (+dragonfly
compat), the multiprotocol/multi-version plugin, anticheat, performance, and quality.
Out of scope (for v1): Realms, resource/behavior-pack authoring tools, web admin panel.

### 1.3 Actors
- **JP** — Java Edition player · **BP** — Bedrock Edition player
- **OP** — server operator/admin · **PD** — plugin developer · **SYS** — the server itself

### 1.4 Glossary
- **Edition** — Java or Bedrock. **Protocol** — the wire protocol number of one edition (e.g. Java 775, Bedrock 975).
- **Canonical model** — the edition-agnostic in-memory representation of world/blocks/items/entities/players.
- **LWVersion** — a LivingWorld server version (e.g. `26 (A)`) mapping to a set of protocol-compatible client patches per edition.
- **Flavor** — *Vanilla* (turn-key binary) or *Custom* (library, dragonfly-style).
- **State ID / Runtime ID** — Java global block-state ID / Bedrock runtime block ID.

### 1.5 Requirement conventions (EARS)
Acceptance criteria use EARS: `WHEN <event> THE SYSTEM SHALL <response>`, `WHILE <state> …`,
`IF <condition> THEN …`, `WHERE <feature included> …`. Each criterion is independently testable.

### 1.6 Verified baseline (2026-05-31)
Java target **26.1 = protocol 775** (patched `third_party/go-mc`); Bedrock target **1.26.20 = protocol 975**
(`gophertunnel v1.56.2`). These anchor §10.

---

## 2. Functional requirements

### R1 — Dual-native connectivity & sessions
**User story:** As JP/BP, I want to connect to the same server and world using my edition's normal client, so that I can play crossplay without a proxy.
1. WHEN a Java client opens a TCP connection on the Java port, THE SYSTEM SHALL complete handshake → login → configuration → play and spawn the player.
2. WHEN a Bedrock client opens a RakNet/UDP connection on the Bedrock port, THE SYSTEM SHALL complete the RakNet + login → resource-pack → start-game flow and spawn the player.
3. WHEN players of both editions are online, THE SYSTEM SHALL place them in one shared world with one shared block/entity/player state.
4. WHILE a session is active, THE SYSTEM SHALL maintain keep-alive/ping and disconnect a client that times out, releasing its resources.
5. IF a client of an unsupported protocol connects, THEN THE SYSTEM SHALL reject it with a human-readable version message (see R10).
6. WHEN a player disconnects, THE SYSTEM SHALL persist their state and broadcast a leave event.

### R2 — Authentication, encryption & access control
**User story:** As OP, I want secure, configurable authentication per edition, so that only authorized players join.
1. WHERE Java online-mode is enabled, THE SYSTEM SHALL authenticate via Mojang/Yggdrasil and enable AES-CFB8 packet encryption + compression.
2. WHERE Bedrock online-mode is enabled, THE SYSTEM SHALL verify the Xbox Live login chain (PlayFab/XSAPI, JOSE/JWT) and reject invalid chains.
3. IF a player is on the ban list or IP-ban list, THEN THE SYSTEM SHALL deny login with the ban reason.
4. WHERE a whitelist is enabled, THE SYSTEM SHALL only admit listed players.
5. THE SYSTEM SHALL expose op levels and per-command permission gating (R7.5, R11).
6. WHEN handling auth tokens or keys, THE SYSTEM SHALL NOT log their secret values.

### R3 — World generation, storage & persistence
**User story:** As a player, I want a real, persistent world that looks identical on both editions.
1. THE SYSTEM SHALL generate the Overworld with biomes, terrain noise, caves, ravines, and ore/feature/structure placement.
2. THE SYSTEM SHALL generate Nether and End dimensions (incl. End islands and the dragon arena).
3. WHEN the same seed is used, THE SYSTEM SHALL produce identical terrain for JP and BP (parity).
4. WHEN a block is changed, THE SYSTEM SHALL persist the edit and reload it after restart (no data loss on graceful stop).
5. WHILE running, THE SYSTEM SHALL autosave on a configurable interval and perform a final save on shutdown.
6. IF a chunk file is corrupt, THEN THE SYSTEM SHALL log and recover (regenerate or quarantine) without crashing the server.
7. THE SYSTEM SHALL maintain sky + block lighting consistent with vanilla propagation.

### R4 — Blocks, items, inventory & crafting
**User story:** As a player, I want full block/item behavior so I can build, mine, and craft as in vanilla.
1. THE SYSTEM SHALL support the full vanilla block-state palette for the target version and resolve states by name.
2. WHEN a block is broken with a tool, THE SYSTEM SHALL apply correct mining speed and drop the correct loot (loot tables).
3. THE SYSTEM SHALL implement block updates/neighbor notifications, random ticks, fluids (flow/waterlogging), gravity, fire, and explosions.
4. THE SYSTEM SHALL implement redstone components and block entities (chests, furnaces, hoppers, signs, spawners, brewing, beacons, …).
5. THE SYSTEM SHALL provide full inventories/containers, stack rules, durability, and item components/NBT.
6. THE SYSTEM SHALL implement crafting, smelting, smithing, stonecutting, enchanting, anvil, and brewing per vanilla recipes.
7. WHILE a JP and BP view the same container, THE SYSTEM SHALL keep inventory state synchronized across editions.

### R5 — Entities, mobs & combat
**User story:** As a player, I want living entities and working combat shared across editions.
1. THE SYSTEM SHALL spawn, tick, move (with interpolation), and despawn entities with edition-correct metadata.
2. WHEN JP and BP are in view range, THE SYSTEM SHALL render each player to the other (skins via skinbridge, pose, equipment).
3. THE SYSTEM SHALL implement mob AI (goals, pathfinding, targeting), spawning rules, breeding, and villager trading.
4. THE SYSTEM SHALL implement damage, health, death/respawn, knockback, criticals, armor, shields, and status effects matching vanilla math.
5. THE SYSTEM SHALL implement projectiles, dropped-item entities, XP orbs, and vehicles.
6. WHEN an entity-affecting action occurs, THE SYSTEM SHALL broadcast it to all in-range viewers of both editions.

### R6 — Player mechanics & progression
1. THE SYSTEM SHALL validate movement server-authoritatively (sprint/sneak/swim/climb/elytra) — see R13.
2. THE SYSTEM SHALL implement gamemodes (survival/creative/adventure/spectator) and abilities (fly/instant-break/invuln).
3. THE SYSTEM SHALL implement hunger/saturation/regen, XP/levels, sleep/night-skip, spawn point/anchor, and per-player inventory persistence.
4. THE SYSTEM SHALL implement advancements/achievements, statistics, and recipe unlocks.

### R7 — Gameplay systems, commands & chat
1. THE SYSTEM SHALL implement game-time/day-night, weather, difficulty, gamerules, world border, scoreboard/objectives, teams, and bossbars.
2. THE SYSTEM SHALL expose the vanilla command set (give, tp, gamemode, time, weather, effect, summon, setblock, fill, clone, gamerule, op, kick, ban, whitelist, difficulty, xp, enchant, clear, kill, spawnpoint, worldborder, …).
3. WHERE the client is Java, THE SYSTEM SHALL provide a Brigadier command graph with argument types, suggestions, and error formatting; WHERE Bedrock, it SHALL sync the equivalent command data.
4. WHEN a player chats, THE SYSTEM SHALL deliver the message to both editions with correct formatting (Java signed chat ⇄ Bedrock rawtext) and fire a cancellable chat event.
5. IF a command is run without sufficient permission, THEN THE SYSTEM SHALL deny it with a clear message.
6. THE SYSTEM SHALL emit positional sounds and particles with edition-correct identifiers, plus titles/actionbar/tab-list, and Bedrock UI forms.

### R8 — Cross-edition parity
1. THE SYSTEM SHALL maintain full Java-state-ID ⇄ Bedrock-runtime-ID mapping tables for blocks, items, and entities, auto-generated from bundled data.
2. WHEN a gameplay feature is implemented once on the canonical model, THE SYSTEM SHALL expose equivalent behavior to both editions.
3. WHEN identical input is given by a JP and a BP, THE SYSTEM SHALL converge to the same resulting world state (verifiable by a parity harness).

---

## 3. Product/structural requirements

### R9 — Two server flavors (one core)
**User story:** As OP I want a turn-key vanilla server; as PD I want a programmable server — both with full protocol + features.
1. THE SYSTEM SHALL expose all server capabilities through public packages (`server`, `world`, `player`, `plugin`, `command`, `event`) with nothing under `internal/` required by consumers.
2. **Vanilla flavor:** WHEN `cmd/server` is run with no code changes, THE SYSTEM SHALL provide a faithful vanilla experience (survival defaults, vanilla worldgen, full commands, anticheat on) configurable via `config.yml` / `server.properties`-equivalents.
3. **Custom flavor:** WHEN imported as a library, THE SYSTEM SHALL let PD register custom blocks/items/entities/commands/worldgen/recipes and handle every gameplay event while retaining full protocol + feature completeness.
4. THE SYSTEM SHALL guarantee both flavors share identical protocol/version/feature code paths (no feature fork).

### R10 — LivingWorld version scheme
**User story:** As OP/PD I want one clear server version that states exactly which client patches are supported.
1. THE SYSTEM SHALL identify its version as `LivingWorld <YY> (<Letter>)` (e.g. `26 (A)`).
2. THE SYSTEM SHALL map each LWVersion to a set of supported Java client versions sharing one Java protocol AND a set of Bedrock client versions sharing one Bedrock protocol.
3. WHERE multiple MC patches share a wire protocol (e.g. hotfixes 26.1 / 26.1.1 / 26.1.2), THE SYSTEM SHALL support them under ONE LWVersion without a new server version.
4. WHEN either edition changes its wire protocol, THE SYSTEM SHALL require a new LWVersion (new `Letter`, or new `YY` per MC year).
5. WHEN queried (`livingworld --version`, `/lwversion`), THE SYSTEM SHALL report the LWVersion and the full supported-client matrix.
6. THE SYSTEM SHALL source patch→protocol grouping from research backed by the MC release changelog and protocol references (see DESIGN §6).

### R11 — Plugin system + dragonfly compatibility
**User story:** As PD I want to write plugins easily and reuse existing dragonfly plugins.
1. THE SYSTEM SHALL provide typed, cancellable event handlers for every gameplay hook and a `Host` capability surface (world/entity/inventory/scheduler/storage/permissions/commands).
2. THE SYSTEM SHALL support plugin lifecycle: load order, (soft-)dependencies, enable/disable, and panic isolation so one plugin cannot crash the server.
3. WHERE a plugin targets the dragonfly API, THE SYSTEM SHALL run it through a compatibility layer (`plugin/dfcompat`) **unmodified** and bridge its effects to both editions.
4. THE SYSTEM SHALL define a plugin manifest (name, version, api-version, deps, permissions, entrypoint) and a documented loading model.
5. WHERE the loading model permits, THE SYSTEM SHALL support enable/disable/reload without a full restart.
6. THE SYSTEM SHALL ship a scaffold/template, examples, and full `PLUGIN_API` docs so a new plugin can be created in minutes.
7. IF a plugin declares an api-version incompatible with the running LWVersion, THEN THE SYSTEM SHALL refuse to load it with a clear message.

### R12 — Official plugin: Multiprotocol multi-version (1.21 → latest)
**User story:** As OP I want clients from MC 1.21 up to the latest (both editions) to join one server, natively.
1. WHERE the multiprotocol plugin is enabled, THE SYSTEM SHALL accept Java clients from **1.21 through the target protocol (775)** and Bedrock clients across the **1.21.x → 975** range.
2. WHEN a client whose protocol differs from the canonical version connects, THE SYSTEM SHALL translate packets/registries/chunks/items/entities between that protocol and the canonical model.
3. IF a connecting version is outside the configured min/max, THEN THE SYSTEM SHALL reject it with a clear message.
4. THE SYSTEM SHALL allow OP to configure the allowed version range and per-version toggles.
5. THE SYSTEM SHALL implement translation as plugins on top of R10/R11 (native, no external proxy), serving as the reference example of the plugin API.
6. WHEN a feature is unavailable on an older client, THE SYSTEM SHALL gracefully degrade rather than disconnect.

### R13 — Official anticheat
**User story:** As OP I want a built-in, server-authoritative anticheat with low false positives.
1. THE SYSTEM SHALL treat the server as the authority for player movement on both editions (enable Bedrock server-authoritative movement; reconcile Java).
2. THE SYSTEM SHALL run movement, combat, world-interaction, and packet/timing checks and maintain a per-player weighted violation model with decay.
3. WHEN violations exceed configurable thresholds, THE SYSTEM SHALL take staged action (log → warn → setback → kick → ban) and emit an anticheat event (R11).
4. THE SYSTEM SHALL apply lag/TPS compensation so that legitimate high-latency players are not falsely flagged.
5. THE SYSTEM SHALL let OP configure per-check enable/threshold/action via `anticheat.yml` and admin commands, including player exemptions.
6. THE SYSTEM SHALL produce structured violation logs and SHALL keep a false-positive regression suite green (R15).

---

## 4. Non-functional requirements

### R14 — Performance & operability
1. THE SYSTEM SHALL run each world on a 20 TPS tick loop and SHALL not let one world/entity/chunk stall others (budgeted ticking).
2. THE SYSTEM SHALL support a target of ≥100 concurrent mixed-edition players on reference hardware without sustained TPS drop below 19.
3. THE SYSTEM SHALL load/generate chunks and perform disk I/O off the tick thread (async) with backpressure.
4. THE SYSTEM SHALL expose metrics, profiling endpoints (auth-gated), and crash diagnostics under `diag/`.
5. THE SYSTEM SHALL provide graceful start/stop, RCON/console, and a documented Docker/compose deployment.

### R15 — Quality, security & maintenance
1. THE SYSTEM SHALL build on a Go toolchain free of known reachable vulnerabilities (`govulncheck` clean) and SHALL pin dependencies to released versions where possible.
2. THE SYSTEM SHALL maintain automated tests: unit, integration, cross-edition parity, and load; with coverage trending toward >80%.
3. THE SYSTEM SHALL run CI that builds, vets, vuln-scans, and connect-tests each supported client version in the LWVersion matrix.
4. THE SYSTEM SHALL keep protocol/version docs in sync with code (no stale protocol numbers).
5. WHERE network endpoints are exposed, THE SYSTEM SHALL document and enforce their authentication/authorization posture.

---

## 5. Traceability
Each requirement Rn maps to a DESIGN section and to TODO tasks:
| Req | Design | TODO |
|---|---|---|
| R1–R2 | §3, §5, §7 | §1.1–1.2 |
| R3 | §10 | §1.3–1.4 |
| R4 | §4, §10 | §1.5–1.6 |
| R5 | §4, §10 | §1.7–1.8 |
| R6–R7 | §4, §9 | §1.9–1.12 |
| R8 | §4 | §1.14 |
| R9 | §6 (flavors) | §2 |
| R10 | §6 | §3 |
| R11 | §7 | §4 |
| R12 | §8 | §5 |
| R13 | §9 | §6 |
| R14–R15 | §11–§13 | §7 |
