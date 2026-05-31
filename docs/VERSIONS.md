# Version Compatibility Matrix

**LivingWorld supports cross-play between Minecraft Bedrock and Java editions.**

## Supported Versions

LivingWorld is **`LivingWorld 26 (A)`** — one server version covering every Minecraft
patch that shares the same wire protocol on each edition.

| Component | Versions | Protocol | Status |
|-----------|----------|----------|--------|
| **Java Edition** | 26.1, 26.1.1, **26.1.2** | **775** | ✅ Supported |
| **Bedrock Edition** | 1.26.20 (≈26.20) | **975** | ✅ Supported |
| **Bedrock Edition** | 1.26.21 / 1.26.23 | 975 | 🔄 Same protocol — should work |

All three Java patches (26.1 / 26.1.1 / 26.1.2) share Java protocol **775**, so they join
under one LivingWorld version. The server sends the full `minecraft:timeline` registry
data (it can't rely on the client's built-in pack, because the known-pack version match
is exact and the server can't know which 26.1.x patch is connecting) — see
[PROTOCOL.md](PROTOCOL.md).

> Earlier builds of this doc listed Java 1.20.2 / protocol 764. That was stale: the
> bundled `third_party/go-mc` fork is hand-patched to `ProtocolVersion = 775`.

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

Java: Check `third_party/go-mc/net/protocol.go`:
```go
const (
    ProtocolVersion = 775  // MC Java 26.1 / 26.1.1 / 26.1.2
    // ...
)
```

Bedrock: `gophertunnel` handles this automatically.

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
| 26 (A) | 2026-05-31 | Java 26.1/26.1.1/**26.1.2** (proto 775) + Bedrock 1.26.20 (proto 975); 26.1.2 registry fix; world converter |
| (future) | TBD | Nether/End dimensions; full inventory sync |

## Known Compatibility Issues

### Bedrock sub-versions
- **Issue**: minor feature/protocol drift between 1.26.x patches
- **Workaround**: 1.26.20–1.26.23 share protocol 975 and are generally compatible
- **Fix**: bump `gophertunnel` when a patch changes the wire protocol

## Future Version Targets

| Target | Priority | Notes |
|--------|----------|-------|
| Next Java protocol bump | High | New `LWVersion` letter when the wire protocol changes |
| Multiprotocol plugin (1.21 → latest) | Medium | Native ViaVersion-style translation (see TODO §5) |
| Bedrock LevelDB world conversion | Medium | Complete the `worldconvert` Bedrock path |
