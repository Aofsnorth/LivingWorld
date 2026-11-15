# LivingWorld

<p align="center">
  <strong>High-Performance Native Cross-Play Minecraft Server in Go & Rust</strong><br>
  Java Edition 26.1–26.3 (Protocols 775–777) · Bedrock Edition 26.32 (Protocol 1001) · Native Cross-Play · Zero JVM
</p>

---

## Overview

**LivingWorld** is a native, server-authoritative Minecraft server written from scratch in **Go (`1.27.2`)** with a high-performance **Rust native world-generation bridge (`native/steel-bridge`)**. It is not a Paper/Spigot fork, requires no JVM, and runs both **Minecraft: Java Edition** and **Minecraft: Bedrock Edition** players in the exact same world and entity simulation without any external translation proxy (no Geyser/Floodgate required).

Every tick, movement validation, block interaction, combat calculation, hunger/exhaustion cycle, weather transition, and mob AI decision is resolved once in an edition-agnostic core and broadcast natively to both protocol bridges.

---

## Protocol Compatibility

| Edition | Supported Versions | Protocol Version | Default Port | Status |
| :--- | :--- | :--- | :--- | :--- |
| **Minecraft: Java Edition** | `26.1`, `26.1.1`, `26.1.2`, `26.2`, `26.3` | `775`, `776`, `777` | `25565/TCP` | Active Target |
| **Minecraft: Bedrock Edition** | `26.32` | `1001` | `19132/UDP` | Full Cross-Play |

---

## Key Engineering Highlights

- **Zero-JVM Native Binary**: Compiled directly to machine code using Go `1.27.2` and Rust static/shared FFI (`steel-bridge`), keeping idle memory consumption minimal and eliminating JVM garbage-collection pauses.
- **Native Java + Bedrock Cross-Play**: Both editions connect directly to the server, sharing chunks, block updates, item drops, mob AI, combat, and chat in real time.
- **Bidirectional Cross-Edition Skin Synchronization**:
  - Java skins (Mojang official & Ely.by / LegacyLauncher) are automatically converted to raw RGBA buffers and broadcast to Bedrock clients via `PlayerSkin` packets.
  - Bedrock client skins are normalized, encoded to PNG, signed asynchronously via MineSkin, and injected into Java `PlayerInfoUpdate` profiles.
- **Vanilla-Faithful Overworld Generation**: Powered by extracted vanilla density functions, noise parameters, spline evaluators, and a native Rust C-ABI bridge (`native/steel-bridge`).
- **20 Hz Deterministic Simulation & Optimized AI**:
  - Unified 20 Hz world tick engine (`internal/world/tick.go`) driving daylight, weather state machines, fluid/gravity scheduled ticks, status effects, and hunger/saturation regeneration.
  - **Binary Min-Heap A\* Pathfinding** (`O(N log N)`) with dynamic terrain malus weights and **Spatial Hash Grid Entity Collisions** (`O(N)`) for smooth player-to-mob and mob-to-mob pushing.
- **Complete Survival & Combat Mechanics**:
  - Server-authoritative tool harvest rules, hardness/efficiency crack stage synchronization, critical hits (`1.5x` damage + critical particles), sprint-knockback, 10-tick invulnerability frames, armor reduction, and vanilla fall damage.
- **Persistent World & Player State**: Anvil-style region storage, atomic `level.json` metadata persistence (seed, daytime, weather phase), and per-player inventory/health/hunger persistence.
- **Native Go Plugin API**: Event-driven extensibility (`plugin/`) with cancellable player, block, chat, and world lifecycle hooks.

---

## Architecture

```text
LivingWorld/
├── cmd/livingworld/        # Main server binary entrypoint
├── config/                 # YAML configuration loader & environment overrides
├── internal/
│   ├── bedrock/            # Bedrock Edition RakNet listener, packet handlers & chunk encoders
│   ├── java/               # Java Edition TCP listener, protocol 775-777 codecs & session state
│   ├── combat/             # Edition-neutral weapon damage, knockback & sweep math
│   ├── command/            # Built-in server commands (/gamemode, /time, /weather, /tp, etc.)
│   ├── drops/              # Item drop entities, pickup radius & despawn lifecycle
│   ├── item/               # Vanilla item registry & block-item mappings
│   ├── mobs/               # Mob store, binary-heap A* pathfinder, goal/brain AI & spatial grid
│   ├── player/             # Authoritative player state, inventory, effects, armor & push loop
│   ├── registry/           # Canonical block state & runtime ID registries
│   └── world/              # 20 Hz tick scheduler, chunks, lighting, hunger, weather & persistence
├── native/steel-bridge/    # Rust FFI crate for high-throughput native terrain generation
├── plugin/                 # Public Go plugin interfaces and event definitions
├── server/                 # Cross-edition bridge wiring & lifecycle orchestration
└── tests/                  # Integration, cross-play mechanics & benchmark test suites
```

---

## Quick Start

### Prerequisites
- **Go** `1.27.2` or newer
- **Rust** toolchain (`cargo`) for building `native/steel-bridge`
- **CGO** enabled (`CGO_ENABLED=1`) with a C compiler (`gcc` / `clang` / MinGW-w64 on Windows)

### 1. Build the Native Rust Bridge
```bash
cd native/steel-bridge
cargo build --release
cd ../..
```

### 2. Run the Test Suite & Build the Server
```bash
go test ./...
go build -o livingworld ./cmd/livingworld
```
> **Windows (PowerShell):**
> ```powershell
> go build -o livingworld.exe ./cmd/livingworld
> .\livingworld.exe
> ```

### 3. Connect Clients
- **Java Edition (`26.1`–`26.3`)**: Connect to `localhost:25565`
- **Bedrock Edition (`26.32`)**: Connect to `127.0.0.1` on UDP port `19132`

---

## Configuration

Server settings live in `config/config.yml` and can be overridden via environment variables:

| Environment Variable | YAML Path | Default | Description |
| :--- | :--- | :--- | :--- |
| `LW_DEFAULT_GAMEMODE` | `defaultGamemode` | `0` | Default gamemode on first join (`0`=Survival, `1`=Creative, `2`=Adventure, `3`=Spectator) |
| `LW_DIFFICULTY` | `world.difficulty` | `normal` | World difficulty (`peaceful`, `easy`, `normal`, `hard`) |
| `LW_ONLINE_MODE` | `java.onlineMode` | `false` | Mojang session server authentication for Java clients |
| `LW_SKIN_MODE` | `java.skinMode` | `auto` | Skin provider lookup (`auto`, `mojang`, `ely`, `off`) |
| `LW_MINESKIN_API_KEY` | `bedrock.mineSkinApiKey` | `""` | Optional MineSkin API key for Bedrock-to-Java skin signing |

---

## Contributing & Engineering Standards

All contributors and AI coding agents must strictly follow the rules in [`AGENTS.md`](./AGENTS.md):

1. **Edition-Neutral Core**: Never import `internal/java` or `internal/bedrock` inside `internal/world`, `internal/player`, `internal/mobs`, or `internal/combat`.
2. **Conventional Commits**: All commit messages must follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/) (`feat(scope): ...`, `fix(scope): ...`, `perf(scope): ...`, `refactor(scope): ...`, `test(scope): ...`, `docs(scope): ...`, `ci: ...`). Vague commit messages (`Add`, `Idk`, `wip`, `update`) are strictly forbidden.
3. **Pre-Commit Verification Gate**: Every change must pass `go fmt ./...`, `go vet ./...`, `go test ./...`, and `go build ./...` with zero warnings or regressions before committing.
