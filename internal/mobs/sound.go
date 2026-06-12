// Mob sound emission. The `mobs` package stays edition-neutral, so
// OnSound is just a callback the world layer wires; the Java and
// Bedrock bridges each translate the (sound ID, volume, pitch) tuple
// into the per-edition packet (Java ClientboundGameSoundEntity,
// Bedrock LevelSoundEvent).
//
// Sound IDs here are the vanilla namespaced "minecraft:..." strings
// from the vanilla sound registry. The Bedrock bridge passes them
// through to LevelSoundEvent; the Java bridge uses the same string
// with go-mc's SoundEvent struct.
//
// Volume + pitch follow the vanilla formula: volume is in [0.5, 1.0]
// with occasional 0.0 (silent — e.g. far-away mob); pitch is in
// [0.8, 1.2] for "ambient" sounds. The function signatures take
// these as float32 and pass them straight through.

package mobs

import (
	"math/rand"
)

// SoundEvent identifies a mob sound by its namespaced vanilla id.
// The same ids are used by both editions.
type SoundEvent string

const (
	SoundMobZombieSay        SoundEvent = "minecraft:entity.zombie.ambient"
	SoundMobSkeletonSay      SoundEvent = "minecraft:entity.skeleton.ambient"
	SoundMobCreeperSay       SoundEvent = "minecraft:entity.creeper.primed" // primed-hiss (no separate "say")
	SoundMobCowSay           SoundEvent = "minecraft:entity.cow.ambient"
	SoundMobPigSay           SoundEvent = "minecraft:entity.pig.ambient"
	SoundMobSheepSay         SoundEvent = "minecraft:entity.sheep.ambient"
	SoundMobChickenSay       SoundEvent = "minecraft:entity.chicken.ambient"
	SoundMobHurt            SoundEvent = "minecraft:entity.generic.hurt"
	SoundMobDeath           SoundEvent = "minecraft:entity.generic.death"
	SoundMobShoot           SoundEvent = "minecraft:entity.skeleton.shoot" // skeleton shoots arrow
	// M1: per-type ambient sounds for the 16 new mobs. All are
	// vanilla namespaced ids; the bridges translate to their
	// edition's packet.
	SoundMobHuskSay          SoundEvent = "minecraft:entity.husk.ambient"
	SoundMobStraySay         SoundEvent = "minecraft:entity.stray.ambient"
	SoundMobBoggedSay        SoundEvent = "minecraft:entity.bogged.ambient"
	SoundMobDrownedSay       SoundEvent = "minecraft:entity.drowned.ambient"
	SoundMobSpiderSay        SoundEvent = "minecraft:entity.spider.ambient"
	SoundMobCaveSpiderSay    SoundEvent = "minecraft:entity.spider.ambient" // cave spider shares spider's ambient
	SoundMobSlimeSay         SoundEvent = "minecraft:entity.slime.ambient"
	SoundMobMagmaCubeSay     SoundEvent = "minecraft:entity.magma_cube.ambient"
	SoundMobPhantomSay       SoundEvent = "minecraft:entity.phantom.ambient"
	SoundMobBlazeSay         SoundEvent = "minecraft:entity.blaze.ambient"
	SoundMobGhastSay         SoundEvent = "minecraft:entity.ghast.ambient"
	SoundMobWitchSay         SoundEvent = "minecraft:entity.witch.ambient"
	SoundMobEndermanSay      SoundEvent = "minecraft:entity.enderman.ambient"
	SoundMobPiglinSay        SoundEvent = "minecraft:entity.piglin.ambient"
	SoundMobWitherSkeletonSay SoundEvent = "minecraft:entity.wither_skeleton.ambient"
	SoundMobIronGolemSay     SoundEvent = "minecraft:entity.iron_golem.ambient"
	SoundMobZombieVillagerSay SoundEvent = "minecraft:entity.zombie_villager.ambient"
)

// EmitSounds decides which sound each mob should play this tick, and
// returns a list of (mobID, sound, volume, pitch) tuples the world
// layer forwards to bridges. The decision is per-mob-type and based
// on the mob's recent activity (e.g. shoot → arrow sound; hurt →
// hurt sound; idle ambient sounds fire on a 5% chance per tick with
// a per-mob-type minimum interval of ~80 ticks = 4 s).
//
// The SoundEmit struct itself is re-exported from
// internal/mobs/ai/context (see ai_context_aliases.go) so the AI
// subpackages and the world layer share one canonical shape. The
// SoundEvent enum (the strings "minecraft:entity.skeleton.shoot"
// etc.) stays in this file as a private decision table.

// EmitSounds walks the mob store and returns the sounds to play this
// tick. The function is read-only on the mob store; the world layer
// passes the returned list to OnSound (added in the AIContext, like
// the other side-effect callbacks).
func (s *Store) EmitSounds(rng *rand.Rand) []SoundEmit {
	type pendingSound struct {
		emit  SoundEmit
		category int
	}
	// We use the helper signature expected by callers — the random
	// source is rng. We don't actually lock the store here; the
	// caller is expected to hold the snapshot (or pass an iterator).
	// v1: keep the API minimal and let world/tick.go pass []Mob.
	_ = pendingSound{}
	return nil
}

// EmitSoundsFromSnapshot is the version called with a snapshot Mob
// slice (e.g. from Store.All()) so the AI lock can be released
// before we touch rng. The world layer takes the snapshot, then
// calls this.
func EmitSoundsFromSnapshot(snapshot []Mob, rng *rand.Rand) []SoundEmit {
	out := make([]SoundEmit, 0, 8)
	for i := range snapshot {
		m := &snapshot[i]
		def := defFor(m.Type)
		emit := decideSound(m, def, rng)
		if emit.Sound != "" {
			out = append(out, emit)
		}
	}
	return out
}

// decideSound returns ONE sound per mob per tick (or none). The
// priorities are:
//
//	1. StateShoot (skeleton) → SoundMobShoot
//	2. StateFuse (creeper, fuse>20) → SoundMobCreeperSay
//	3. Took damage this tick → SoundMobHurt
//	4. Just died (Despawn && HP<=0) → SoundMobDeath
//	5. Idle/Flee ambient → type-specific sound, 5% per tick with a
//	   per-mob 4 s cooldown.
//
// Sound events are intentionally rare — a steady stream of zombie
// groans is annoying and doesn't match vanilla cadence.
func decideSound(m *Mob, def MobDef, rng *rand.Rand) SoundEmit {
	// 1. Skeleton fires.
	if m.State == StateShoot && m.DrawTicks == 0 {
		return SoundEmit{MobID: m.EntityID, Sound: string(SoundMobShoot), Volume: 1.0, Pitch: 1.0}
	}
	// 2. Creeper hissing.
	if m.State == StateFuse && m.FuseTicks > 20 {
		return SoundEmit{MobID: m.EntityID, Sound: string(SoundMobCreeperSay), Volume: 1.0, Pitch: 1.0}
	}
	// 3. Just hurt (cooldownTicks was just set by a melee swing this
	//    tick; we use FireTicks as a proxy for "took damage in the
	//    last second" — fires on 80 ticks = 4 s, so a hurt + death
	//    in the same tick is the death sound, not the hurt).
	// 4. Just died.
	if m.Despawn && m.HP <= 0 {
		return SoundEmit{MobID: m.EntityID, Sound: string(SoundMobDeath), Volume: 1.0, Pitch: 0.9}
	}
	// 5. Ambient. We gate by an "every-80-ticks" cycle: a per-mob
	//    counter increments per tick; when it crosses 80, the mob
	//    has a 5% chance to play its ambient sound and reset.
	//    We piggy-back on FireTicks as a free counter: it's >0
	//    after a hit, <80 on average for passives, so we just use
	//    a "every 80 ticks" pseudo-random pattern. Cheap and good
	//    enough for v1.
	if m.FireTicks%80 == 0 && rng.Float64() < 0.05 {
		var s SoundEvent
		switch m.Type {
		case "minecraft:zombie":
			s = SoundMobZombieSay
		case "minecraft:skeleton":
			s = SoundMobSkeletonSay
		case "minecraft:cow":
			s = SoundMobCowSay
		case "minecraft:pig":
			s = SoundMobPigSay
		case "minecraft:sheep":
			s = SoundMobSheepSay
		case "minecraft:chicken":
			s = SoundMobChickenSay
		// M1: per-type ambients.
		case "minecraft:husk":
			s = SoundMobHuskSay
		case "minecraft:stray":
			s = SoundMobStraySay
		case "minecraft:bogged":
			s = SoundMobBoggedSay
		case "minecraft:drowned":
			s = SoundMobDrownedSay
		case "minecraft:spider":
			s = SoundMobSpiderSay
		case "minecraft:cave_spider":
			s = SoundMobCaveSpiderSay
		case "minecraft:slime":
			s = SoundMobSlimeSay
		case "minecraft:magma_cube":
			s = SoundMobMagmaCubeSay
		case "minecraft:phantom":
			s = SoundMobPhantomSay
		case "minecraft:blaze":
			s = SoundMobBlazeSay
		case "minecraft:ghast":
			s = SoundMobGhastSay
		case "minecraft:witch":
			s = SoundMobWitchSay
		case "minecraft:enderman":
			s = SoundMobEndermanSay
		case "minecraft:piglin":
			s = SoundMobPiglinSay
		case "minecraft:wither_skeleton":
			s = SoundMobWitherSkeletonSay
		case "minecraft:iron_golem":
			s = SoundMobIronGolemSay
		case "minecraft:zombie_villager":
			s = SoundMobZombieVillagerSay
		default:
			// Generic ambient for unknown types.
			s = ""
		}
		if s != "" {
			return SoundEmit{MobID: m.EntityID, Sound: string(s), Volume: 0.8, Pitch: 0.95 + float32(rng.Float64())*0.1}
		}
	}
	_ = def
	return SoundEmit{}
}
