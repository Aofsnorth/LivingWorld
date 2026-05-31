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
- **💾 World persistence** — chunk edits are saved to disk (gzip region files), autosaved, and reloaded on restart
- **🗺️ World import** — convert vanilla **Java** worlds in and out with the `worldconvert` tool (Anvil ⇄ LivingWorld)
- **🎮 Ergonomic plugin API** — typed event handlers, **cancellable** events, and a `Host` capability surface
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

java:
  port: 25565
  onlineMode: false
  maxPlayers: 100
  viewDistance: 10
  simulationDistance: 10
  bind: "0.0.0.0"

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

> Upgrading from an older build that wrote per-chunk `c.<x>.<z>.bin` files? Those are
> no longer read — delete the old `worlds/` folder once (a superflat world regenerates).

## 🗺️ World Import / Conversion

Vanilla worlds aren't loaded directly — convert them with the bundled `worldconvert` tool:

```bash
go build -o worldconvert ./cmd/worldconvert
./worldconvert import-java <vanillaJavaWorldDir> worlds/world   # vanilla Java → server
./worldconvert export-java worlds/world <vanillaJavaWorldDir>   # server → vanilla Java
```

Conversion pivots on the block **name** (LivingWorld's block ID *is* the Java global
block-state ID), so Java ⇄ LivingWorld is near-identity. The source is never modified.
Block-state *properties* default and lighting/biomes/entities aren't transferred
(vanilla recomputes lighting on load). Bedrock (LevelDB) conversion isn't implemented
yet (`import-bedrock`/`export-bedrock` return a clear error).

## 📁 Project Structure

```
livingworld/
├── cmd/server/            # thin entry point over the public API
├── cmd/worldconvert/      # vanilla Java ⇄ LivingWorld world converter
├── server/                # PUBLIC library API (server.New / Run / Host)
├── plugin/                # PUBLIC plugin API (events, Host, manager)
├── config/                # configuration
├── examples/exampleplugin # runnable library + plugin example
├── internal/
│   ├── bedrock/           # Bedrock protocol (gophertunnel)
│   ├── java/              # Java protocol 775 (go-mc)
│   ├── item/              # item registry (wraps go-mc item data)
│   ├── player/            # shared player model + Controller routing
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
| `block.break` | **yes** | Block broken (cancel to keep it) |
| `block.place` | **yes** | Block placed (cancel to prevent) |
| `server.start` / `server.stop` | no | Lifecycle |

## 🔧 Development

```bash
go build ./...
go test ./...
```

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

## ⚠️ Current Limitations

- **Bedrock block fidelity**: state→Bedrock mapping is name-based; stateful blocks (stairs orientation, slab halves, log axis, …) fall back to defaults. Property overrides can be added in `internal/bedrock/world/block_sync.go`.
- **Bedrock → Java skins**: not shown on vanilla Java clients (Mojang restricts skin URLs to its own domains). A MineSkin-style uploader is the proper fix.
- **Held-item placement**: the block/item registries are complete, but parsing the 26.1 creative item-stack (data components) to place the held item is not yet wired — survival block-break + server/plugin `SetBlock` work today.
- **Bedrock inventory**: opens (server-authoritative inventory enabled), but item *manipulation* (ItemStackRequest handling) is not yet implemented, so moved items snap back.
- **Player actions across protocols**: `Broadcast`/`Message`/`Kick` are wired for **Java** sessions; the Bedrock controller is a follow-up.
- **World generation**: only superflat.

## 🔮 Roadmap

See [ROADMAP.md](ROADMAP.md) for the current phase and milestones, and [PACKAGES.md](PACKAGES.md) for a per-package reference (workstream lanes + status).
