# LivingWorld Roadmap

**Planned features and improvements for LivingWorld.**

## Version 0.1.0 — Basic Crossplay (In Progress)

### Features
- [x] Java Edition 1.20.2 support
- [x] Bedrock Edition 1.21 support
- [x] Shared world for both editions
- [x] Basic player movement sync
- [x] Block place/break sync
- [x] Superflat world generation
- [x] Plugin API (basic events)

### Known Gaps
- [ ] Java 1.21+ support (protocol 766+)
- [ ] Full inventory synchronization
- [ ] Entity synchronization (mobs, dropped items)
- [ ] Chat synchronization between editions

## Version 0.2.0 — Complete Protocol Support

### Features
- [ ] Java 1.21 protocol support
- [ ] Java 1.21.3 protocol support
- [ ] Automatic protocol negotiation
- [ ] Protocol version fallback messages

### Technical
- [ ] Patched `go-mc` with latest protocols
- [ ] Updated `gophertunnel` integration
- [ ] Protocol detection system

## Version 0.3.0 — World Features

### Features
- [ ] Nether dimension
- [ ] End dimension
- [ ] Portal system between dimensions
- [ ] Biome generation (overworld)
- [ ] Cave generation
- [ ] Ore generation

### Technical
- [ ] Multi-world support
- [ ] World persistence (save/load)
- [ ] Chunk batching for performance

## Version 0.4.0 — Enhanced Gameplay

### Features
- [ ] Full inventory synchronization
- [ ] Crafting synchronization
- [ ] Enchantment system
- [ ] Brewing and potions
- [ ] Trading with villagers

### Technical
- [ ] Item stack synchronization
- [ ] NBT data handling
- [ ] Custom item support

## Version 0.5.0 — Entity System

### Features
- [ ] Mob spawning and AI
- [ ] Entity interpolation for smooth movement
- [ ] Projectile physics
- [ ] Vehicle support (boats, horses, minecarts)
- [ ] Item entity sync (dropped items)

### Technical
- [ ] Entity ID allocation
- [ ] Entity metadata protocol
- [ ] Movement prediction

## Version 0.6.0 — Advanced Plugin API

### Features
- [ ] Command system
- [ ] Permission system
- [ ] Economy system
- [ ] Scheduler/tasks
- [ ] Configuration files per plugin

### Technical
- [ ] Plugin reload without restart
- [ ] Plugin dependencies
- [ ] Soft-depend system

## Version 1.0.0 — Production Ready

### Features
- [ ] All core Minecraft features
- [ ] Performance optimized
- [ ] Comprehensive documentation
- [ ] Production deployment guide

### Quality
- [ ] Unit test coverage >80%
- [ ] Integration tests
- [ ] Stress testing
- [ ] Security audit

## Future Ideas

### Long-term
- [ ] BungeeCord/Velocity plugin compatibility
- [ ] Plugin repository/manager
- [ ] Web admin panel
- [ ] Metrics and monitoring
- [ ] Cloud deployment support
- [ ] macOS Bedrock support (via LAN)

### Community Requested
- [ ] Mini-games support
- [ ] Custom world generation API
- [ ] Resource pack support
- [ ] Behavior pack support
- [ ] Realms integration
