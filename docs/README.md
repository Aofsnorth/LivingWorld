# LivingWorld

**A native Minecraft hybrid server — Bedrock *and* Java editions from one unified Go backend, usable as a library.**

[![Go Version](https://img.shields.io/badge/Go-1.26.1-blue)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## 🎯 Overview

LivingWorld is a Minecraft server written in Go that natively supports **both Bedrock and Java editions at the same time**. Unlike proxies (BungeeCord, Velocity) or compatibility layers (Geyser), LivingWorld is **not a proxy**: a single backend serves both protocols and all players share one world state.

It is also designed to be **used as a library** — embed it in your own Go program the way you would [dragonfly](https://github.com/df-mc/dragonfly).

### Key Features

- **🔄 Cross-Play Native** — Bedrock and Java players share the same world, blocks, and entities
- **🧱 Complete block & item registries** — the full vanilla 26.1 palette (~29,800 block states, ~1,500 items) via the bundled go-mc data; no hand-maintained tables
- **🎮 Cross-edition gameplay** — day/night, **weather** (`/weather`), **mobs + basic AI** (`/summon`), and **melee combat** (knockback + damage + hurt flash) all synced to both editions
- **💾 World & player persistence** — chunk edits (gzip region files), weather/time, and per-player position/inventory/health/gamemode are saved to disk, autosaved, and restored on rejoin
- **🗺️ World import** — bring vanilla **Java** (Anvil) *and* **Bedrock** (`.mcworld`/LevelDB) worlds in via the `worldconvert` tool
- **🎨 Resource-pack converter** — convert a vanilla pack between Java and Bedrock (`packconvert`)
- **🔌 Plugins, two ways** — typed **Go** plugins (compile-time, full API) and drop-in **JavaScript** scripts (runtime, no rebuild)
- **🖥️ Terminal UI** — a live console TUI (opt out with `--no-tui`)
- **📦 Library-shaped** — `import "livingworld/server"` and run a full server in a few lines
- **🔧 Protocol Adaptors** — automatic translation between Java state IDs and Bedrock runtime IDs

## 📋 Requirements

- **Go 1.26.1+**
- **Minecraft Java Edition** — 26.1 (protocol **775**)
- **Minecraft Bedrock Edition** — 26.x (gophertunnel's bundled protocol)

## 🚀 Quick Start (run the bundled server)

```bash
git clone https://github.com/yourusername/livingworld.git
cd livingworld
go mod download
go build -o livingworld ./cmd/server
./livingworld
```

The server starts with a live **terminal UI** (status + logs) when attached to a TTY. Pass `--no-tui` for plain log output (e.g. when redirecting to a file or running as a service).

## 📦 Use as a library

The entire server is exposed through the public `livingworld/server` package:

```go
package main

import (
    "log"

    "livingworld/plugin"
    "livingworld/server"
)

func main() {
    srv := server.New(server.DefaultConfig())

    // React to events with typed handlers.
    srv.Plugins().OnPlayerJoin(func(e *plugin.PlayerJoinEvent) {
        srv.Broadcast("§e" + e.PlayerName + " joined!")
    })

    // Cancel events to veto actions (here: protect bedrock).
    bedrock := srv.StateID("minecraft:bedrock")
    srv.Plugins().OnBlockBreak(func(e *plugin.BlockBreakEvent) {
        if e.BlockID == bedrock {
            srv.Message(e.PlayerName, "§cYou can't break bedrock.")
            e.Cancel()
        }
    })

    if err := srv.Run(); err != nil { // blocks until Ctrl-C, then saves + stops
        log.Fatal(err)
    }
}
```

A complete, runnable example lives in [`examples/exampleplugin`](../examples/exampleplugin/main.go):

```bash
go run ./examples/exampleplugin
```

See **[PLUGIN_API.md](PLUGIN_API.md)** for the full plugin/library guide.

## ⚙️ Configuration

Edit `config/config.yml`:

```yaml
serverName: "LivingWorld Server"
motd: "A Minecraft Server — Cross-play enabled!"

world:
  type: superflat        # superflat (nether/end coming soon)
  seed: 12345
  spawn: { x: 0, y: 4, z: 0, yaw: 0, pitch: 0 }
  persistence: true      # save the world to disk
  directory: "worlds"    # base folder; each world gets a subfolder
  autosaveSeconds: 300   # 0 disables autosave (a final save still runs on shutdown)
  dayNightCycle: true    # advance the sun/moon (false = fixed time)
  difficulty: normal     # peaceful | easy | normal | hard

java:
  port: 25565
  onlineMode: false
  maxPlayers: 100
  viewDistance: 10
  simulationDistance: 10
  bind: "0.0.0.0"
  skinSource: auto       # auto | mojang | ely | none (offline-mode skin lookup)
  mineSkinAPIKey: ""     # upload Bedrock skins so Java clients can see them
  bedrockHDSkins: false  # serve full-res Bedrock skins (see Skins section)

bedrock:
  port: 19132
  maxPlayers: 100
  viewDistance: 8
  bind: "0.0.0.0"
  authDisabled: true
```

### Environment overrides

| Variable | Description |
|----------|-------------|
| `LIVINGWORLD_SERVER_NAME` | Server display name |
| `LIVINGWORLD_JAVA_PORT` | Java edition port |
| `LIVINGWORLD_BEDROCK_PORT` | Bedrock edition port |
| `LIVINGWORLD_PLUGINS_DIR` | Plugin directory path |

## 🎮 Commands

Built-in commands (operator-only except `help`), available from both editions:

| Command | Usage | Description |
|---------|-------|-------------|
| `/help` | `help` | List available commands |
| `/gamemode` (`/gm`) | `gamemode <survival\|creative\|adventure\|spectator>` | Change your gamemode |
| `/tp` (`/teleport`) | `tp <x> <y> <z>` or `tp <player>` | Teleport |
| `/give` | `give <item> [count]` | Give yourself an item |
| `/time` | `time set <day\|night\|noon\|midnight\|ticks>` | Set time of day |
| `/weather` | `weather <clear\|rain\|thunder>` | Set the weather (persisted, both editions) |
| `/summon` | `summon <pig\|cow\|chicken\|sheep\|creeper\|zombie\|skeleton>` | Spawn a mob at your position |
| `/lwversion` | `lwversion` | Show the supported LivingWorld + client version matrix (anyone can run) |

Plugins register their own commands via the `player.command` event.

## 📜 JavaScript plugins (drop-in)

Besides Go plugins, drop `.js` files into the `plugins/` directory — they load at
startup with no rebuild. A `server` global exposes broadcast/message/block APIs and
`on*` hooks mirror the Go events:

```js
// plugins/welcome.js
server.onPlayerJoin(function (e) {
  server.broadcast("§e" + e.playerName + " joined!");
});
```

Go plugins are the most powerful (typed, full `Host` API, compile-time); JavaScript is
the fastest to iterate (runtime, no rebuild). See **[PLUGIN_API.md](PLUGIN_API.md)**.

## 🧱 Blocks & Items

LivingWorld's **canonical block ID is the vanilla Java global block-state ID**. This single decision unlocks the whole vanilla palette:

- **Java** uses these IDs directly on the wire (identity mapping — no translation).
- **Bedrock** maps a state ID → namespaced name → Bedrock runtime ID via dragonfly's palette.

```go
stone := world.StateID("minecraft:stone")   // resolve by name
name  := world.StateName(stone)              // "minecraft:stone"
count := world.StateCount()                  // full palette size

it, ok := item.ByName("diamond_sword")       // item registry
blockState, placeable := item.BlockStateID("minecraft:oak_planks")
```

## 💾 World Persistence

- Edits set a per-chunk dirty flag; only edited chunks are written.
- Chunks are grouped **32×32 into region files** at `worlds/<world>/region/r.<rx>.<rz>.lwr`
  (gzip-compressed, atomic temp-file + rename) — like Minecraft's Anvil format, so a
  normal play area is a **handful of files instead of hundreds**.
- Autosave runs on an interval and a final save runs on graceful shutdown.
- On access, a chunk is loaded from disk if present, otherwise generated.
- **Level state** (weather + time of day) is stored in `worlds/<world>/level.json`.
- **Per-player data** (position, inventory, health, gamemode) is saved to
  `worlds/<world>/playerdata/<uuid>.json` on disconnect/shutdown and restored on rejoin.

> Upgrading from an older build that wrote per-chunk `c.<x>.<z>.bin` files? Those are
> no longer read — delete the old `worlds/` folder once (a superflat world regenerates).

## 🗺️ World Import / Conversion

Vanilla worlds aren't loaded directly — convert them with the bundled `worldconvert` tool:

```bash
go build -o worldconvert ./cmd/worldconvert
./worldconvert import-java    <vanillaJavaWorldDir>    worlds/world   # vanilla Java → server
./worldconvert export-java    worlds/world             <vanillaJavaWorldDir>
./worldconvert import-bedrock <vanillaBedrockWorld>    worlds/world   # .mcworld / LevelDB → server
```

Conversion pivots on the block **name** (LivingWorld's block ID *is* the Java global
block-state ID), so Java ⇄ LivingWorld is near-identity. **Bedrock import** (`.mcworld`
archive or an extracted LevelDB `db/` folder) maps Bedrock runtime states → names → the
shared palette. Block-state *properties* default and lighting/biomes/entities aren't
transferred (vanilla recomputes lighting on load). `export-bedrock` (LevelDB writer) is
not implemented yet and returns a clear error. The source is never modified.

## 🎨 Resource-pack Conversion

Convert a vanilla resource pack between editions with `packconvert` (auto-detects the source):

```bash
go build -o packconvert ./cmd/packconvert
./packconvert <sourcePack(.zip/.mcpack/.jar/dir)> <outDir>
```

v1 converts pack **structure, manifest, icon and the textures folder layout**. Per-file
texture renaming, 3D models, sounds, languages and Bedrock addons are not converted yet.

## 📁 Project Structure

```
livingworld/
├── cmd/server/            # thin entry point over the public API (+ terminal UI)
├── cmd/worldconvert/      # Java/Bedrock ⇄ LivingWorld world converter
├── cmd/packconvert/       # Java ⇄ Bedrock resource-pack converter
├── server/                # PUBLIC library API (server.New / Run / Host)
├── plugin/                # PUBLIC plugin API (events, Host, manager, JS scripting)
├── config/                # configuration
├── examples/exampleplugin # runnable library + plugin example
├── plugins/               # drop-in JavaScript plugins
├── internal/
│   ├── bedrock/           # Bedrock protocol (gophertunnel)
│   ├── java/              # Java protocol 775 (go-mc)
│   ├── item/              # item registry (wraps go-mc item data)
│   ├── mobs/              # cross-edition mob store + basic AI
│   ├── player/            # shared player model + Controller routing
│   ├── resourcepack/      # resource-pack converter
│   ├── skinbridge/        # Bedrock→Java skin upload/serve
│   ├── worldconvert/      # world import/export (Anvil + Bedrock LevelDB)
│   └── world/             # shared world, chunks, blocks, persistence, registry
│       └── generator/     # world generators (superflat)
├── third_party/go-mc/     # patched go-mc (Java protocol + block/item data)
└── docs/                  # documentation
```

## 🔌 Plugin Events

| Event | Cancellable | Description |
|-------|-------------|-------------|
| `player.join` | no | Player joins (`PlayerName`, `UUID`) |
| `player.leave` | no | Player disconnects |
| `player.chat` | **yes** | Chat message (cancel to suppress) |
| `player.move` | no | Player moved |
| `player.interact` | **yes** | Right-click a block/air |
| `player.attack` | **yes** | Player attacks an entity |
| `player.command` | **yes** | Before a command runs |
| `player.respawn` | no | After a respawn |
| `block.break` | **yes** | Block broken (cancel to keep it) |
| `block.place` | **yes** | Block placed (cancel to prevent) |
| `entity.damage` | **yes** | Entity takes damage (`Cause`, `Amount`) |
| `entity.death` | no | Entity dies |
| `container.click` | **yes** | Container slot clicked |
| `item.drop` / `item.pickup` | **yes** | Item dropped / picked up |
| `server.start` / `server.stop` | no | Lifecycle |

## 🔧 Development

```bash
go build ./...
go test ./...
```

A first-party test foundation lives next to the features it covers
(`internal/{registry,combat,world,version,command}/*_test.go`).
CI on `.github/workflows/ci.yml` enforces `build · vet · test · govulncheck`.

## 🧭 Version matrix

`internal/version` is the single source of truth for which clients the
server accepts. The default registry ships `LWVersion "26 (A)"`
(Java protocol `775`, Bedrock protocol `975`). The matrix is surfaced
three ways:

- `livingworld --version` — script-friendly, tab-separated, exits early.
- `/lwversion` in-game — same data, any player can run it.
- `livingworld-versioncheck` — polls the upstream Mojang manifest,
  exits non-zero when the matrix has drifted behind an official client.

See [VERSION_MATRIX.md](VERSION_MATRIX.md) for the full table.

## 📜 License

MIT — see [LICENSE](LICENSE).

## 🙏 Acknowledgments

- [go-mc](https://github.com/Tnze/go-mc) — Java protocol + block/item data
- [gophertunnel](https://github.com/sandertv/gophertunnel) — Bedrock protocol
- [dragonfly](https://github.com/df-mc/dragonfly) — Bedrock block palette + design inspiration

## 🧑‍🎨 Skins

In offline mode the client doesn't send its skin, so the server resolves it by
username from a configurable source (`java.skinSource`):

- `mojang` — the official premium account with that name (default). Note: for a
  cracked name this returns whoever *owns* that premium name, which may be a
  stranger.
- `ely` — the **Ely.by** skin store used by **LegacyLauncher / TLauncher** and
  similar cracked launchers. Use this if your players are cracked.
- `none` — send no skin (let the client's own launcher skin show).

Vanilla Java clients only load skins from `.minecraft.net` / `.mojang.com`, so
`ely` skins render only for players whose launcher injects authlib-injector
(LegacyLauncher/TLauncher do). Bedrock viewers always see Java players' skins
(the server downloads the PNG and forwards it).

**Bedrock → Java skins** are uploaded to Mojang via MineSkin (set `java.mineSkinAPIKey`)
so Java clients render them as a signed 64×64 texture. Set `java.bedrockHDSkins: true`
to instead serve the full-resolution (128×128) skin from the built-in skin bridge —
this only renders on clients that accept unsigned skin URLs from arbitrary hosts (most
authlib-injector launchers; strict/vanilla clients fall back to the default skin), so it
is **off by default**.

## 🔮 Roadmap

See [ROADMAP.md](ROADMAP.md) for the current phase and milestones, and [PACKAGES.md](PACKAGES.md) for a per-package reference (workstream lanes + status).
