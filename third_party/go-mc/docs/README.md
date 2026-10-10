# go-mc Documentation

Go libraries for Minecraft Java Edition — bot framework, server framework,
world I/O, NBT, and protocol implementation.

**Current version**: Minecraft 1.21.11 (protocol 774)

## Data Coverage

All data is extracted directly from the Minecraft server jar via a container-based
pipeline (`tools/`). No third-party data sources (PrismarineJS, Burger, etc.) are
used. The extracted data is consumed by Go generators that produce type-safe Go code.

### MC 1.21.11 Data Summary

| Category | Count | Package | Source |
|----------|------:|---------|--------|
| Block types | 1,166 | `level/block` | `blocks.json` (MC `--all` report) |
| Block states | 29,671 | `level/block` | `blocks.json` → `block_states.nbt` |
| Block property enums | 29 | `level/block` | `block_properties.json` (Java extractor) |
| Block entity types | 49 | `level/block` | `block_entities.json` (Java extractor) |
| Packet IDs | 264 | `data/packetid` | `packets.json` (MC `--all` report) |
| Entity types | 157 | `data/entity` | `entities.json` (Java extractor) |
| Item types | 1,505 | `data/item` | `items.json` (MC `--all` report) |
| Sound IDs | 1,838 | `data/soundid` | `registries.json` (MC `--all` report) |
| Registries | 95 | `data/registryid` | `registries.json` (MC `--all` report) |
| Biomes | 65 | `level/biome` | `biomes.json` (Java extractor) |
| Data components | 104 | `level/component` | `components.json` (Java extractor) |
| Languages | 147 | `data/lang` | Mojang asset index CDN |

### Implementation Status

| Feature | Status | Notes |
|---------|--------|-------|
| Network protocol (bot) | Complete | Login, config, play phases. E2E tested against vanilla 1.21.11 |
| Network protocol (server) | Complete | Per-registry RegistryData, all phases |
| Chunk I/O (network) | Complete | BitStorage format (1.21.5+ no length prefix) |
| Chunk I/O (save/region) | Complete | Anvil format, tested with MC 1.21.4 + 1.21.11 |
| Chat signing | Complete | HistoryUpdate checksum, globalIndex, PackedSignature |
| Slot / inventory | Complete | Post-1.20.5 format with component data |
| Data components | Complete | All 104 wire protocol types (IDs 0–103) |
| Block state mapping | Complete | 29,671 states, validated via E2E |
| NBT codec | Complete | Full spec, SNBT, RawMessage |
| RCON | Complete | Client and server |

### Data Accuracy

All generated data comes directly from Minecraft's own server jar — no third-party
data sources. The extraction + generation pipeline is fully repeatable:
`cd tools && go run . --extract --version 1.21.11` produces identical output
each run. See [dev/tools.md](dev/tools.md) for the full pipeline documentation.

## Packages

| Package | Description |
|---------|-------------|
| `bot/` | Bot (client) framework — connect, login, keepalive, events |
| `bot/basic/` | Core bot features — position, settings, keepalive, tags |
| `bot/msg/` | Chat message handling with signing support |
| `bot/screen/` | Inventory / slot management |
| `bot/world/` | Chunk tracking for connected bots |
| `server/` | Server framework — handshake, login, config, gameplay |
| `level/` | Chunk data structures, bit storage, palettes |
| `level/block/` | Block types, state IDs, property enums |
| `level/component/` | Data component types (104 types) |
| `level/biome/` | Biome list |
| `nbt/` | NBT codec (binary + SNBT) |
| `net/` | Low-level network (connection, encryption, compression) |
| `net/packet/` | Packet primitives (VarInt, String, etc.) |
| `chat/` | Chat message format (JSON + NBT) |
| `save/` | Anvil world I/O (region files, chunks) |
| `registry/` | Registry codec + network serialization |
| `data/packetid/` | Protocol packet ID constants |
| `data/entity/` | Entity type IDs |
| `data/item/` | Item type IDs |
| `data/soundid/` | Sound event IDs |
| `data/registryid/` | All 95 registry ID mappings |

## Developer Documentation

See [docs/dev/](dev/) for:

- [tools.md](dev/tools.md) — Code generation & MC data extraction pipeline
