# Protocol Documentation

**Technical details of how LivingWorld bridges Bedrock and Java editions.**

## Overview

Minecraft Bedrock and Java editions use completely different network protocols:

| Aspect | Java Edition | Bedrock Edition |
|--------|--------------|-----------------|
| Transport | TCP | UDP (RakNet) |
| Protocol | Custom (VarInt) | RakNet + Custom |
| Encryption | AES-CFB8 | None (local) |
| Compression | Zlib | Snappy |
| Auth | Mojang/Yggdrasil | Xbox Live |

LivingWorld bridges these protocols at the application layer.

## Java Protocol Structure

### Connection Flow

```
Client                        Server
   │                             │
   │──── Handshake ────────────▶│
   │     (protocol version)       │
   │                             │
   │──── Login Start ────────────▶│
   │     (username, uuid)         │
   │                             │
   │◀─── Encryption Request ────│
   │     (server id, pubkey)      │
   │                             │
   │──── Encryption Response ───▶│
   │     (verify token, hash)     │
   │                             │
   │◀─── Login Success ──────────│
   │     (uuid, username)        │
   │                             │
   │◀═══ Game packets ═════════▶│
```

### Key Packets

| Packet ID | Name | Purpose |
|-----------|------|---------|
| 0x00 | Handshake | Protocol version negotiation |
| 0x00 | Login Start | Player authentication |
| 0x02 | Clientbound Game Login | Spawn player |
| 0x0F | Clientbound Player Position | Set player position |
| 0x21 | Clientbound Level Chunk | Send chunk data |
| 0x38 | Clientbound System Chat | Chat messages |

## Bedrock Protocol Structure

### Connection Flow

```
Client                        Server
   │                             │
   │──── RakNet Open ───────────▶│
   │     (protocol version)      │
   │                             │
   │◀─── RakNet ACK ────────────│
   │                             │
   │──── Login ─────────────────▶│
   │     (identity, chain)        │
   │                             │
   │◀─── Play Status ───────────│
   │     (success/spawn)          │
   │                             │
   │◀─── Start Game ────────────│
   │     (world data)            │
   │                             │
   │◀═══ Game packets ═════════▶│
```

### Key Packets

| Packet ID | Name | Purpose |
|-----------|------|---------|
| 0x01 | Login | Authentication |
| 0x02 | Play Status | Connection status |
| 0x03 | Server to Client Handshake | Security |
| 0x05 | Start Game | Initialize world |
| 0x08 | Chunk Radius Update | View distance |
| 0x12 | Move Player | Player movement |
| 0x17 | Update Block | Block changes |

## Bridge Implementation

### Chunk Synchronization

When a player loads chunks:

```
1. World Manager generates unified chunk
         │
         ▼
2. Check player edition
         │
    ┌────┴────┐
    │         │
    ▼         ▼
Java      Bedrock
    │         │
    ▼         ▼
Convert   Convert
to Java   to Bedrock
format    format
    │         │
    └────┬────┘
         │
         ▼
3. Send edition-specific packet
```

### Block ID Mapping

LivingWorld uses internal block IDs (1-2 digit integers):

| Internal | Java Block | Bedrock Block |
|----------|------------|---------------|
| 1 | minecraft:bedrock | 7 |
| 2 | minecraft:dirt | 3 |
| 3 | minecraft:grass_block | 2 |
| 0 | minecraft:air | 0 |

**Java State ID** = Full block state with properties
**Bedrock Runtime ID** = Numeric identifier in palette

### Player Position Sync

Both editions use 3D coordinates:
- **Java**: Double precision (64-bit)
- **Bedrock**: Float (32-bit)

LivingWorld stores as `float64` internally and converts as needed.

### Chat Format

| Edition | Format | Example |
|---------|--------|---------|
| Java | JSON Chat | `{"text":"Hello"}` |
| Bedrock | JSON Chat | `{"rawtext":[{"text":"Hello"}]}` |

LivingWorld normalizes to internal format, converts on send.

## Protocol Constants

### Java Edition (26.1.2)

```go
const (
    ProtocolVersion  = 775        // covers Java 26.1 / 26.1.1 / 26.1.2
    MinecraftVersion = "26.1.2"
)
```

### Bedrock Edition (1.26.20)

```go
const (
    CurrentProtocol = 975
    CurrentVersion  = "1.26.20"
)
```

## Configuration-phase registries (Java 775)

During the Configuration phase the server streams dynamic registries to the
client. New in 26.1 is `minecraft:timeline` (the data-driven day/night sun/moon
angle + sky-colour curves). LivingWorld sends the **full** timeline NBT for each
element (`day`, `moon`, `early_game`, `villager_schedule`) rather than relying on
the client's built-in `core` pack via `SelectKnownPacks`:

- The client matches a known pack by **exact id + version**. Protocol 775 spans
  26.1 / 26.1.1 / 26.1.2, and the server cannot know the connecting patch, so a
  single announced version can never match all of them — a data-less timeline
  then fails with *"Unbound values in registry minecraft:timeline"* and the
  client aborts at `finish_configuration`.
- Sending full data is version-independent and always accepted. The data is
  bundled verbatim from `26.1.2.jar` under `internal/java/server/registrydata/`.

## Cross-edition authority notes

- **Player push** is server-driven (`player.Manager` push loop): a symmetric,
  horizontal-only impulse, applied **only to grounded players** so a descending
  player's fall velocity is never reset (Java `SetEntityMotion` / Bedrock
  `SetActorMotion` replace the whole velocity vector, unlike vanilla's additive
  push). Bedrock uses client-authoritative movement, so its push is amplified to
  compensate for client damping.
- **Game mode** is server-authoritative: a non-op Bedrock client that changes its
  mode via the pause-menu *Settings → Game* selector is snapped back by echoing a
  `SetPlayerGameType{Survival}` plus survival abilities (handler `SetPlayerGameType`).

## World conversion

Vanilla worlds are not loaded directly; the `worldconvert` CLI (`cmd/worldconvert`,
`internal/worldconvert`) converts between vanilla **Java Anvil** (`region/*.mca`)
and LivingWorld's region format (`region/r.<rx>.<rz>.lwr`). The pivot is the block
**name**, because LivingWorld's block ID *is* the Java global block-state ID, making
Java↔LivingWorld near-identity at the state level. Block-state *properties* default,
and lighting/biomes/entities are not transferred (vanilla recomputes lighting on
load). Bedrock (LevelDB) conversion is stubbed pending block-palette mapping.

## Future Protocol Support

### Java 1.21 Changes

New packets in protocol 766+:
- `clientbound_player_loaded`
- `clientbound_ticking_state`
- Updated registry format

### Bedrock 1.21.x Changes

- New actor properties
- Updated simulation distance handling
- Improved resource pack flow

## Testing Protocol Compatibility

### Manual Testing

1. Start LivingWorld server
2. Connect Java 1.20.2 client
3. Connect Bedrock 26.21 client
4. Verify both see same blocks
5. Test block interactions

### Automated Testing

```go
func TestChunkConversion(t *testing.T) {
    wChunk := world.NewChunk()
    lChunk := convertToLevelChunk(wChunk)
    // Verify conversion
}
```
