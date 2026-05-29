# LivingWorld Project - Minecraft Bedrock + Java Native Crossplay Server

## Status: Research Complete ✅ | Documentation Created ✅

## User Requirements
- Minecraft Bedrock 26.21 (protocol 975)
- Minecraft Java 26.1 (protocol 766+)
- Project must be updatable continuously (SOLID principles)
- Bedrock + Java server as ONE native binary

## Research Summary

### Current State
- **Java Protocol**: go-mc library stuck at 1.20.2 (protocol 764)
- **Bedrock Protocol**: gophertunnel supports 1.21+ (protocol 975)
- **Block Conversion**: Basic (bedrock, dirt, grass) implemented
- **World Gen**: Superflat only
- **Cross-play**: Shared world manager, shared player manager

### Architecture (Layered)
1. **Protocol Layer** - Java (go-mc) / Bedrock (gophertunnel)
2. **Bridge Layer** - Chunk/packet conversion
3. **Core Layer** - World, Player, Plugin managers
4. **Plugin Layer** - Event-driven extensibility

### Key Files Analyzed
- cmd/server/main.go - Entry point, starts both servers
- internal/bedrock/ - Bedrock handler, chunk bootstrap
- internal/java/ - Session, bridge, chunk conversion
- internal/world/ - Unified chunk/block system
- internal/player/ - Unified player tracking
- config/config.go - YAML + env var configuration

## TODO: Next Steps

### Critical - Java 1.21+ Support
- [ ] Patch go-mc library for protocol 766+
- [ ] Update third_party/go-mc/net/protocol.go
- [ ] Test Java 26.1 client connection

### High Priority
- [ ] Implement chat sync between editions
- [ ] Inventory sync between editions
- [ ] Entity sync (other players visible cross-edition)

### Medium Priority
- [ ] Nether/End world generation
- [ ] World persistence (save/load)
- [ ] More block types in converter

### Documentation Created
- ✅ docs/README.md - Overview, quick start, features
- ✅ docs/ARCHITECTURE.md - Technical architecture
- ✅ docs/VERSIONS.md - Version compatibility matrix
- ✅ docs/ROADMAP.md - Planned features
- ✅ docs/PROTOCOL.md - Protocol bridge details
- ✅ docs/CONTRIBUTING.md - Development guide

### SOLID Principles to Apply
1. **S**ingle Responsibility - Each package one purpose
2. **O**pen/Closed - Extend via plugin API, not modification
3. **L**iskov Substitution - Unified interfaces for Java/Bedrock
4. **I**nterface Segregation - Small, focused interfaces
5. **D**ependency Inversion - Depend on abstractions (World, Player managers)

## Current Limitations
- Java clients must use 1.20.2-1.20.4 for now
- Inventory sync not complete
- Entity sync not implemented
- Chat format conversion pending
