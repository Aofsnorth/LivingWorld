# Version Compatibility Matrix

**LivingWorld supports cross-play between Minecraft Bedrock and Java editions.**

## Supported Versions

### Current Support

| Component | Version | Protocol | Status |
|-----------|---------|----------|--------|
| **Java Edition** | 1.20.2 | 764 | ✅ Primary |
| **Java Edition** | 1.20.4 | 765 | 🔄 May work |
| **Java Edition** | 1.21+ | 766+ | ⚠️ Requires update |
| **Bedrock Edition** | 1.21.x | 972 | ✅ Tested |
| **Bedrock Edition** | 26.21.x | 975 | ✅ Your version |
| **Bedrock Edition** | 26.22+ | 976+ | ⚠️ Should work |

### Your Configuration

Based on your versions:
- **Bedrock 26.21** = Minecraft Bedrock 1.21.80+ (protocol 975)
- **Java 26.1** = Minecraft Java 1.21.6+ (protocol 766+)

⚠️ **Note**: The current `go-mc` library may not support Java protocol 766+ yet. Updates may be required.

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
    ProtocolVersion = 766  // Update this
    // ...
)
```

Bedrock: `gophertunnel` handles this automatically.

### Step 3: Test Block Registries

If Mojang adds/removes blocks:
1. Update `internal/java/chunk.go` block mappings
2. Update `internal/bedrock/handler.go` block mappings

## Version History

| LivingWorld | Date | Changes |
|-------------|------|---------|
| 0.0.1 | 2026-05-27 | Initial release with Java 1.20.2 + Bedrock 1.21 support |
| (future) | TBD | Java 1.21+ support |
| (future) | TBD | Nether/End dimensions |
| (future) | TBD | Full inventory sync |

## Known Compatibility Issues

### Java 1.21+ Clients
- **Issue**: Protocol version mismatch
- **Workaround**: Use Java 1.20.2-1.20.4 client for now
- **Fix**: Patching `go-mc` library in `third_party/`

### Bedrock Sub-versions
- **Issue**: Protocol differences between 1.21.x patches
- **Workaround**: Generally compatible, minor features may not work
- **Fix**: Update `gophertunnel` dependency

## Future Version Targets

| Target | Priority | Notes |
|--------|----------|-------|
| Java 1.21 | High | Core crossplay feature |
| Java 1.21.3 | Medium | Latest stable |
| Bedrock 26.22 | Medium | Keep current |
| Bedrock 27.x | Low | Future versions |
