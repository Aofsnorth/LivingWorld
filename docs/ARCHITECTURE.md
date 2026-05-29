# LivingWorld Architecture

**Technical documentation of the LivingWorld server design.**

## Overview

LivingWorld follows a **layered architecture** with clear separation between:

1. **Protocol Layer** — Edition-specific network handling
2. **Bridge Layer** — Cross-edition translation
3. **Core Layer** — Shared world, player, and game logic
4. **Plugin Layer** — Extensibility system

```
┌─────────────────────────────────────────────────────────┐
│                    LivingWorld Core                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐│
│  │ World Manager│  │Player Manager│  │  Plugin Manager  ││
│  └─────────────┘  └─────────────┘  └─────────────────┘│
└─────────────────────────────────────────────────────────┘
         ▲                    ▲                    ▲
         │                    │                    │
┌────────┴────────┐ ┌────────┴────────┐ ┌────────┴────────┐
│  Java Protocol   │ │Bridge Protocol   │ │Bedrock Protocol │
│    (go-mc)       │ │  (Conversion)    │ │ (gophertunnel)  │
└─────────────────┘ └─────────────────┘ └─────────────────┘
         ▲                    │                    ▲
         │                    │                    │
    [Java Client]◄──────────►│◄──────────►[Bedrock Client]
                        [Network]
```

## Core Components

### 1. World Manager (`internal/world`)

The world manager maintains the canonical game state:

- **Chunk Storage**: In-memory chunk cache keyed by `(chunkX, chunkZ)`
- **Block Access**: Unified block API independent of edition
- **Player Tracking**: Which players are in which world
- **Time System**: Day/night cycle state

```go
type Manager struct {
    worlds       map[string]*World
    defaultWorld *World
}

type World struct {
    chunks    map[ChunkPos]*Chunk
    players   map[uint64]*Player
    generator ChunkGenerator
    dimension Dimension
}
```

### 2. Player Manager (`internal/player`)

Tracks all connected players regardless of edition:

```go
type Player struct {
    UUID       uuid.UUID
    Username   string
    Edition    Edition  // "java" or "bedrock"
    XUID       uint64   // Bedrock XUID (0 for Java)
    World      *World
    Position   world.Position
    Inventory  *Inventory
    // ... health, gamemode, etc.
}
```

### 3. Plugin Manager (`internal/plugin`)

Event-driven plugin system:

```go
type Plugin interface {
    Name() string
    Version() string
    OnEnable() error
    OnDisable() error
}
```

## Protocol Implementation

### Java Edition (`internal/java`)

Uses the `go-mc` library with custom handlers:

- **Server**: Listens on TCP with `gmnet.ListenMC()`
- **Session**: Per-player state (position, inventory, etc.)
- **Bridge**: Translates between protocol and core

**Key Handlers:**
- Login (`AcceptPlayer`) — Authenticate and spawn player
- GamePlay (`HandlePacket`) — Route game packets
- KeepAlive — Prevent timeout disconnections

### Bedrock Edition (`internal/bedrock`)

Uses the `gophertunnel` library:

- **Server**: Listens on UDP with `raknet` protocol
- **Handler**: Process incoming packets
- **Bootstrap**: Send initial chunks to client

**Packet Handling:**
```go
func (s *Server) handlePacket(conn *minecraft.Conn, pk packet.Packet) {
    switch p := pk.(type) {
    case *packet.MovePlayer:       // Movement
    case *packet.PlayerAuthInput:   // Auth input
    case *packet.InventoryTransaction: // Inventory
    // ... etc
    }
}
```

## Cross-Edition Synchronization

### Player Visibility

When a Java player moves, their position is broadcast to:
1. All other Java players (native format)
2. All Bedrock players (via converter)

### Block Changes

When a block is placed/broken:
1. Update world manager
2. Broadcast to all players in the same world
3. Convert packet format per edition

### Chunk Loading

Players load chunks based on their view distance:
- Server generates unified chunks
- Convert to edition-specific format on demand
- Cache converted chunks for performance

## Chunk Conversion (`internal/java/chunk.go`)

```
World chunk format (unified)
type Chunk struct {
    Sections []*Section  // 24 sections (-64 to 319)
    HeightMaps map[string][]int
}

// Java chunk format (go-mc)
type level.Chunk struct {
    Sections []*level.Section
    Biomes   *level.BiomesPaletteContainer
}
```

**Conversion Process:**
1. Read unified chunk sections
2. Map block IDs to Java state IDs
3. Build heightmaps
4. Serialize to Java packet format

## Configuration (`config/config.go`)

```go
type Config struct {
    ServerName string
    MOTD       string
    World      WorldConfig
    Java       JavaConfig
    Bedrock    BedrockConfig
}
```

## Concurrency Model

- **Per-connection goroutines**: Each client runs in its own goroutine
- **RWMutex for shared state**: World chunks, player list
- **Atomic operations**: Entity IDs, time tracking

## Extension Points

### Adding New Block Types

1. Define in `internal/world/block.go`
2. Add Java state ID mapping in `internal/java/chunk.go`
3. Add Bedrock runtime ID in `internal/bedrock/` (via dragonfly)

### Adding New Packets

**Java:**
1. Add handler in `PlayerSession.HandlePacket()`
2. Implement packet-specific logic

**Bedrock:**
1. Add case in `Server.handlePacket()`
2. Handle the packet type

## Performance Considerations

1. **Chunk Cache**: Recently accessed chunks stay in memory
2. **Batch Packet Sending**: Combine packets where possible
3. **Lazy Conversion**: Only convert chunks when requested
4. **Connection Pooling**: Reuse buffers for packet encoding

## Security

- **Java Online Mode**: Configurable authentication via Mojang
- **Bedrock Auth**: Xbox Live authentication (toggleable)
- **Packet Validation**: Input sanitization on all client data
- **Rate Limiting**: Future improvement planned

## Testing Strategy

1. **Unit Tests**: Individual component testing
2. **Integration Tests**: Full connection flow
3. **Protocol Tests**: Packet format validation
4. **Cross-Play Tests**: Both clients in same world
