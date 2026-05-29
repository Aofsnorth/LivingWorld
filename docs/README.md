# LivingWorld

**A native Minecraft hybrid server supporting both Bedrock and Java editions in a single unified binary.**

[![Go Version](https://img.shields.io/badge/Go-1.26.1-blue)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

## 🎯 Overview

LivingWorld is a custom Minecraft server implementation written in Go that natively supports **both Bedrock and Java editions simultaneously**. Unlike proxies (BungeeCord, Velocity) or compatibility layers, LivingWorld uses a **unified world system** where all players share the same world state regardless of their client edition.

### Key Features

- **🔄 Cross-Play Native** — Bedrock and Java players share the same world, same blocks, same entities
- **⚡ High Performance** — Built in Go with efficient concurrency patterns
- **🎮 Plugin API** — Extensible event-driven plugin system
- **🛠️ Modular Design** — Clean architecture for easy maintenance and updates
- **🔧 Protocol Adaptors** — Automatic conversion between Bedrock and Java protocols

## 📋 Requirements

- **Go 1.26.1+** (required)
- **Minecraft Java Edition** — version 1.20.2+ (protocol 764+)
- **Minecraft Bedrock Edition** — version 1.21+ (protocol 975+)

## 🚀 Quick Start

### Build from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/livingworld.git
cd livingworld

# Download dependencies
go mod download

# Build the server
go build -o livingworld ./cmd/server

# Run the server
./livingworld
```

### Configuration

Edit `config/config.yml`:

```yaml
serverName: "LivingWorld Server"
motd: "A Minecraft Server — Cross-play enabled!"

world:
  type: superflat  # superflat | nether | end (coming soon)
  seed: 12345
  spawn:
    x: 0
    y: 4
    z: 0
    yaw: 0
    pitch: 0

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
  authDisabled: true  # Disable Xbox auth for offline testing
```

### Environment Variables

Override config values with environment variables:

| Variable | Description |
|----------|-------------|
| `LIVINGWORLD_SERVER_NAME` | Server display name |
| `LIVINGWORLD_JAVA_PORT` | Java edition port |
| `LIVINGWORLD_BEDROCK_PORT` | Bedrock edition port |
| `LIVINGWORLD_PLUGINS_DIR` | Plugin directory path |

## 📁 Project Structure

```
livingworld/
├── cmd/
│   └── server/
│       └── main.go           # Application entry point
├── config/
│   └── config.go              # Configuration management
├── docs/                      # Documentation
│   ├── ARCHITECTURE.md        # Technical architecture
│   ├── PROTOCOL.md            # Protocol specifications
│   └── PLUGIN_API.md          # Plugin development guide
├── internal/
│   ├── bedrock/               # Bedrock edition implementation
│   │   ├── server.go          # Server listener
│   │   ├── handler.go         # Packet handling
│   │   ├── bootstrap.go       # World bootstrap
│   │   ├── spawn.go           # Player spawning
│   │   └── converter.go       # Chunk/state conversion
│   ├── java/                  # Java edition implementation
│   │   ├── server.go          # Server listener
│   │   ├── session.go         # Player session
│   │   ├── bridge.go          # Protocol bridge
│   │   ├── player.go          # Player handling
│   │   ├── packets.go         # Packet building
│   │   └── chunk.go           # Chunk conversion
│   ├── player/                # Unified player management
│   │   └── manager.go         # Player tracking
│   ├── plugin/                # Plugin system
│   │   ├── manager.go         # Plugin lifecycle
│   │   └── event.go           # Event definitions
│   └── world/                 # Unified world system
│       ├── world.go           # World management
│       ├── chunk.go           # Chunk data structure
│       ├── block.go           # Block definitions
│       └── generator/         # World generators
│           └── superflat.go   # Superflat generator
├── third_party/
│   └── go-mc/                 # Patched go-mc library
├── config.yml                 # Default configuration
└── go.mod                     # Go module definition
```

## 🔌 Plugin API

LivingWorld provides an event-driven plugin system:

```go
package main

import (
    "livingworld/internal/plugin"
)

type MyPlugin struct{}

func (p *MyPlugin) Name() string    { return "my-plugin" }
func (p *MyPlugin) Version() string { return "1.0.0" }

func (p *MyPlugin) OnEnable() error {
    plugin.Manager().On(plugin.EventPlayerJoin, p.onPlayerJoin)
    return nil
}

func (p *MyPlugin) OnDisable() error {
    return nil
}

func (p *MyPlugin) onPlayerJoin(event plugin.Event) {
    e := event.(*plugin.PlayerJoinEvent)
    println("Player joined:", e.PlayerName)
}

// Export plugin instance
var PluginInstance = &MyPlugin{}
```

### Available Events

| Event | Description |
|-------|-------------|
| `player.join` | Player joins the server |
| `player.leave` | Player disconnects |
| `player.chat` | Player sends chat message |
| `player.move` | Player changes position |
| `block.break` | Player breaks a block |
| `block.place` | Player places a block |
| `server.start` | Server starts |
| `server.stop` | Server shuts down |

## 🔧 Development

### Adding New Minecraft Versions

1. Update the library dependencies in `go.mod`
2. Check protocol constants in:
   - Java: `third_party/go-mc/`
   - Bedrock: `internal/bedrock/` using `gophertunnel`
3. Update block/item registries if needed
4. Test with both client editions

### Running Tests

```bash
go test ./...
```

## 📜 License

MIT License — see [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [go-mc](https://github.com/Tnze/go-mc) — Java Edition protocol implementation
- [gophertunnel](https://github.com/sandertv/gophertunnel) — Bedrock Edition protocol implementation
- [dragonfly](https://github.com/df-mc/dragonfly) — Block/item registries reference

## ⚠️ Current Limitations

- **Java Protocol**: Currently targets 1.20.2 (protocol 764). Java 1.21+ requires library updates.
- **Bedrock Protocol**: Targets 1.21 (protocol 975). Some features may vary by subversion.
- **World Generation**: Only superflat is fully implemented.
- **Inventory**: Basic inventory support; complex interactions not yet implemented.
- **Auth**: Bedrock auth is disabled by default for offline play.

## 🔮 Roadmap

See [ROADMAP.md](ROADMAP.md) for detailed plans.
