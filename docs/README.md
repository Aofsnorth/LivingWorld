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
- **💾 World persistence** — chunk edits are saved to disk (gzip per-chunk files), autosaved, and reloaded on restart
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
- Chunks are stored as gzip-compressed files under `worlds/<world>/region/c.<x>.<z>.bin` (atomic temp-file + rename).
- Autosave runs on an interval and a final save runs on graceful shutdown.
- On access, a chunk is loaded from disk if present, otherwise generated.

## 📁 Project Structure

```
livingworld/
├── cmd/server/            # thin entry point over the public API
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

## ⚠️ Current Limitations

- **Bedrock block fidelity**: state→Bedrock mapping is name-based; stateful blocks (stairs orientation, slab halves, log axis, …) fall back to defaults. Property overrides can be added in `internal/bedrock/world/block_sync.go`.
- **Held-item placement**: the block/item registries are complete, but parsing the 26.1 creative item-stack (data components) to place the held item is not yet wired — survival block-break + server/plugin `SetBlock` work today.
- **Player actions across protocols**: `Broadcast`/`Message`/`Kick` are wired for **Java** sessions; the Bedrock controller is a follow-up.
- **World generation**: only superflat.

## 🔮 Roadmap

See [ROADMAP.md](ROADMAP.md).
