// Package world is LivingWorld's canonical, cross-edition world core.
// It owns ONE shared world state (chunks, block-state mutations, block
// events, drops, mobs, time, weather) and exposes it to both the Java
// and Bedrock edges. Gameplay rules (block updates, entity tick,
// weather) live here; the protocol edges only translate packets to
// and from this canonical state. See Master_Plan.md §4 ("Target
// Model: shared canonical core") and DESIGN §2 for the architectural
// verdict.
//
// Every connected client (Java 775 and Bedrock 975) joins the same
// World.Manager instance; place/break/move/drop/damage events fan
// out via the BlockEventBus and the drops/mobs stores. The Java and
// Bedrock server.NewServer calls both receive the same *Manager so
// they share state without going through a packet bridge
// (DESIGN §2.2 / Master_Plan.md §3 audit row "Cross-edition bridge").
//
// Public surface used by the edges:
//
//   - World / Manager: block get/set, chunk lifecycle, time, weather,
//     autosave, tick loop, mob AI tick, drop physics tick.
//   - Block / BlockAir / StateBlock: canonical block values.
//   - Storage: pluggable persistence backends (NopStorage, DiskStorage,
//     RegionStorage). Phase 3 will consolidate to a single live writer
//     inside this package; the persistence package was a divergent
//     scaffold and has been removed in Phase 1.
//   - BlockEventBus: cross-edition block/break/damage events.
//   - WorldEffectBus: cross-edition action effects (crack overlay,
//     break particles+sound).
package world
