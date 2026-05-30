# LivingWorld Bug Fixes & Refactoring Plan

## Critical Bugs (Implement First)

### 1. Cross-Edition Crack Animation ❌
**Problem:** Java players can't see Bedrock players breaking blocks (and vice versa)
**Root Cause:** 
- Bedrock crack events only broadcast to Bedrock sessions (`s.forEachSession` in `broadcastBlockCracking`)
- Java has no crack packet handler
**Fix:**
- Add crack state tracking in world manager or player manager
- Broadcast Bedrock crack → Java (ClientboundGameBlockBreakAck packet ID 0x08)
- Broadcast Java crack → Bedrock (already have `broadcastBlockCracking`)
- Track per-player breaking state: `{playerUUID, blockPos, startTime}`

### 2. Crack State Cleanup ❌
**Problem:** When player holds break then moves to another block, old block still shows crack
**Root Cause:** No AbortBreak sent when switching blocks
**Fix:**
- Track last breaking block position per player
- When StartBreak on new position, send StopBlockCracking for old position
- On AbortBreak, clear tracked position

### 3. Item Rendering in Hand ❌
**Problem:** Held items not visible in player hands
**Root Cause:** No equipment packets sent
**Fix:**
- Java: Send ClientboundGameSetEquipment (packet ID 0x5A) when slot changes or pickup
- Bedrock: Send MobEquipment when slot changes or pickup
- Broadcast to all viewers (both editions)
- Track held item per player in Player struct

### 4. Drop Physics ❌
**Problem:** Drop physics doesn't match vanilla (velocity, gravity, magnet)
**Current:** Random scatter ±0.1 X/Z, 0.2 Y pop
**Vanilla:** 
- Initial velocity: 0.0-0.3 random X/Z, 0.2 Y
- Gravity: -0.04 blocks/tick² 
- Drag: 0.98 multiplier per tick
- Magnet range: 1.0 blocks
- Pickup delay: 10 ticks (0.5s)
**Fix:**
- Update AddItemActor velocity calculation
- Add pickup delay tracking (spawn time + 10 ticks)
- Implement magnet pull when player within 1 block

### 5. Pickup Animation ❌
**Problem:** Pickup animation quality is poor
**Fix:**
- Ensure TakeItemActor sent at correct timing
- Add Java ClientboundGameTakeItemEntity (packet ID 0x6C) for cross-edition
- Smooth magnet pull before pickup

### 6. Push Physics Asymmetry ❌
**Problem:** Java pushing Bedrock is very heavy/laggy, Bedrock pushing Java is normal
**Root Cause:** Likely velocity packet rate or collision box mismatch
**Fix:**
- Check Java velocity packet (ClientboundGameSetEntityMotion 0x5F) rate limiting
- Verify Bedrock SetActorMotion packet rate
- Balance push strength constants per edition

### 7. Bedrock Skin Compression ❌
**Problem:** Bedrock skins look compressed in Java
**Root Cause:** Image scaling in `uploadBedrockSkinToMineSkin`
**Fix:**
- Check image.Resize parameters (use Lanczos3 for quality)
- Ensure 64x64 or 128x128 preserved without artifacts
- Test with high-res Bedrock skins

### 8. Join/Left Message Color ✅
**Status:** Bedrock already uses `§e` (yellow)
**Remaining:** Java needs JSON format `{"text":"... joined","color":"yellow"}`

## Architecture Refactoring (After Bugs Fixed)

### Phase 1: SOLID Folder Structure
```
internal/
├── domain/              # Core business logic (no dependencies)
│   ├── block/
│   ├── entity/
│   ├── item/
│   └── world/
├── application/         # Use cases & services
│   ├── player/
│   ├── inventory/
│   └── chat/
├── infrastructure/      # External concerns
│   ├── persistence/
│   ├── network/
│   │   ├── bedrock/
│   │   └── java/
│   └── protocol/
└── presentation/        # Handlers & controllers
    ├── bedrock/
    └── java/
```

### Phase 2: Protocol Abstraction
- Version registry: `internal/protocol/versions/`
- Packet ID mappings: `versions/java775.go`, `versions/bedrock26.go`
- Block/item ID mappings: `versions/mappings.json`
- Entity metadata indices: `versions/entity_meta.go`

### Phase 3: Auto-Update Tooling
- Research: Burger, PixLyzer, wiki.vg
- Tool: `tools/mcupdate/` - scrapes protocol changes
- Codegen: Generate mapping files from scraped data
- Manual review: Flag breaking changes for human decision

## Implementation Order

1. Fix bugs 1-8 (this session)
2. Build & test each fix
3. Refactor architecture (separate session)
4. Protocol abstraction (separate session)
5. Tooling research & prototype (separate session)
