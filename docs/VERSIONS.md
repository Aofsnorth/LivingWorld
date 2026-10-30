# Version Compatibility Matrix

**LivingWorld supports cross-play between Minecraft Bedrock and Java editions.**

## Supported Versions

LivingWorld is **`LivingWorld 26 (B)`** — one server version covering every Minecraft
patch that shares the same wire protocol on each edition.

| Component | Versions | Protocol | Status |
|-----------|----------|----------|--------|
| **Java Edition** | 26.2 | **776** | ✅ Supported |
| **Bedrock Edition** | 1.26.30 / 26.32 | **1001** | ✅ Supported |

Java 26.3 is the current Mojang release (protocol 777) and is **not yet
supported** — see "Future Version Targets" below. Bedrock's wire protocol is
owned entirely by the pinned `gophertunnel`, so `BedrockProtocol` in
`internal/version` must stay in step with `protocol.CurrentVersion` there.

> Earlier builds of this doc listed Java 1.20.2 / protocol 764. That was stale: the
> bundled `third_party/go-mc` fork is hand-patched to `ProtocolVersion = 776`.

## Library Dependencies

| Library | Purpose | Version Tracking |
|---------|---------|------------------|
| `github.com/Tnze/go-mc` | Java protocol | `third_party/go-mc/` |
| `github.com/sandertv/gophertunnel` | Bedrock protocol | `go.mod` |
| `github.com/df-mc/dragonfly` | Block registries | `go.mod` |

## Updating for New Versions

### Step 1: Check Library Updates

```bash
# Update go-mc (may need patching)
cd third_party/go-mc
git pull origin master

# Update other dependencies
go get -u all
```

### Step 2: Verify Protocol Constants

Java: Check `third_party/go-mc/server/server.go`:
```go
const (
    ProtocolVersion = 776  // MC Java 26.2
    // ...
)
```

Bedrock: `gophertunnel` handles this automatically — read
`minecraft/protocol/info.go` for `CurrentProtocol` / `CurrentVersion`.

### Step 3: Test Block Registries

If Mojang adds/removes blocks:
1. Update `internal/java/chunk.go` block mappings
2. Update `internal/bedrock/handler.go` block mappings

## World import / conversion

Vanilla worlds aren't loaded directly; convert them with the `worldconvert` tool
(see [PROTOCOL.md](PROTOCOL.md#world-conversion) and `cmd/worldconvert`). Java Anvil
import/export is supported; Bedrock (LevelDB) is not implemented yet.

## Version History

| LivingWorld | Date | Changes |
|-------------|------|---------|
| 26 (B) | 2026-09-27 | Bedrock → 1.26.50 (proto 2193) via gophertunnel v1.62.0; packet ID/permission/optional-field migrations |
| (future) | TBD | Java 26.3 (proto 777); Nether/End dimensions; full inventory sync |

## Known Compatibility Issues

### Bedrock sub-versions
- **Issue**: minor feature/protocol drift between Bedrock patches
- **Status**: each Bedrock patch that changes the wire protocol needs a
  `gophertunnel` bump. gophertunnel tracks them one per minor release
  (v1.57.1 = 1.26.30/proto 1001, v1.62.0 = 1.26.50/proto 2193), and a
  bump usually breaks our `protocol.Skin` / `packet.PlayerList` /
  `packet.MovePlayer` call sites — check those three first.
- **Fix**: bump `gophertunnel`, then fix the migrated call sites in
  `internal/bedrock/{skin,world,server}` and `plugin/dfcompat`.

## Future Version Targets

| Target | Priority | Notes |
|--------|----------|-------|
| **Java 26.3 (protocol 777)** | **High** | Current Mojang release. Needs a full data regeneration, not just a number bump — see the note below |
| Multiprotocol plugin (26.2 → latest) | Medium | Native ViaVersion-style translation (see TODO §5) |
| Bedrock LevelDB world conversion | Medium | Complete the `worldconvert` Bedrock path |

> **Why Java 26.3 is not a number bump.** The packet ID table in
> `third_party/go-mc/data/packetid` is code-generated from
> `<version>/packets.json` by `go run ./tools/ --version <v>`, and that
> pipeline also regenerates blocks, items, entities, components, registries,
> biomes, sounds, all ~137 language files and the worldgen `vanilladata`
> tree. Packet IDs shift between releases, so declaring protocol 777 while
> the tables are 26.2's would misparse real traffic. The bump has to be the
> full regeneration for 26.3, re-verified against the overworld generator.
