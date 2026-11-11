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
	SoundMobZombieSay   SoundEvent = "minecraft:entity.zombie.ambient"
	SoundMobSkeletonSay SoundEvent = "minecraft:entity.skeleton.ambient"
	SoundMobCreeperSay  SoundEvent = "minecraft:entity.creeper.primed" // primed-hiss (no separate "say")
	SoundMobCowSay      SoundEvent = "minecraft:entity.cow.ambient"
	SoundMobPigSay      SoundEvent = "minecraft:entity.pig.ambient"
	SoundMobSheepSay    SoundEvent = "minecraft:entity.sheep.ambient"
	SoundMobChickenSay  SoundEvent = "minecraft:entity.chicken.ambient"
	SoundMobHurt        SoundEvent = "minecraft:entity.generic.hurt"
	SoundMobDeath       SoundEvent = "minecraft:entity.generic.death"
	SoundMobShoot       SoundEvent = "minecraft:entity.skeleton.shoot" // skeleton shoots arrow
	// M1: per-type ambient sounds for the 16 new mobs. All are
	// vanilla namespaced ids; the bridges translate to their
	// edition's packet.
	SoundMobHuskSay           SoundEvent = "minecraft:entity.husk.ambient"
	SoundMobStraySay          SoundEvent = "minecraft:entity.stray.ambient"
	SoundMobBoggedSay         SoundEvent = "minecraft:entity.bogged.ambient"
	SoundMobDrownedSay        SoundEvent = "minecraft:entity.drowned.ambient"
	SoundMobSpiderSay         SoundEvent = "minecraft:entity.spider.ambient"
	SoundMobCaveSpiderSay     SoundEvent = "minecraft:entity.spider.ambient" // cave spider shares spider's ambient
	SoundMobSlimeSay          SoundEvent = "minecraft:entity.slime.ambient"
	SoundMobMagmaCubeSay      SoundEvent = "minecraft:entity.magma_cube.ambient"
	SoundMobPhantomSay        SoundEvent = "minecraft:entity.phantom.ambient"
	SoundMobBlazeSay          SoundEvent = "minecraft:entity.blaze.ambient"
	SoundMobGhastSay          SoundEvent = "minecraft:entity.ghast.ambient"
	SoundMobWitchSay          SoundEvent = "minecraft:entity.witch.ambient"
	SoundMobEndermanSay       SoundEvent = "minecraft:entity.enderman.ambient"
	SoundMobPiglinSay         SoundEvent = "minecraft:entity.piglin.ambient"
	SoundMobWitherSkeletonSay SoundEvent = "minecraft:entity.wither_skeleton.ambient"
	SoundMobIronGolemSay      SoundEvent = "minecraft:entity.iron_golem.ambient"
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
		emit     SoundEmit
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
//  1. StateShoot (skeleton) → SoundMobShoot
//  2. StateFuse (creeper, fuse>20) → SoundMobCreeperSay
//  3. Took damage this tick → SoundMobHurt
//  4. Just died (Despawn && HP<=0) → SoundMobDeath
//  5. Idle/Flee ambient → type-specific sound, 5% per tick with a
//     per-mob 4 s cooldown.
//
// Sound events are intentionally rare — a steady stream of zombie
// groans is annoying and doesn't match vanilla cadence.
func decideSound(m *Mob, def MobDef, rng *rand.Rand) SoundEmit {
	// 1. Skeleton fires.
	if m.State == StateShoot && m.DrawTicks == 0 {
		return SoundEmit{MobID: m.EntityID, Sound: string(SoundMobShoot), Volume: 1.0, Pitch: 1.0, X: m.X, Y: m.Y, Z: m.Z, MobType: m.Type}
	}
	// 2. Creeper hissing.
	if m.State == StateFuse && m.FuseTicks > 20 {
		return SoundEmit{MobID: m.EntityID, Sound: string(SoundMobCreeperSay), Volume: 1.0, Pitch: 1.0, X: m.X, Y: m.Y, Z: m.Z, MobType: m.Type}
	}
	// 3. Hurt & 4. Death sounds are published immediately from Store.OnHurt / Store.OnDeath
	//    on the exact tick the damage lands so there is zero delay and per-species vanilla
	//    sounds always play.
	// 5. Ambient.
	//    counter increments per tick; when it crosses 80, the mob
	//    has a 5% chance to play its ambient sound and reset.
	//    We piggy-back on FireTicks as a free counter: it's >0
	//    after a hit, <80 on average for passives, so we just use
	//    a "every 80 ticks" pseudo-random pattern. Cheap and good
	//    enough for v1.
	// 5. Ambient sound: in vanilla Mob.baseTick(), ambient sounds have a
	// minimum cooldown of getAmbientSoundInterval() (80-120 ticks) plus a
	// 1/1000 ramp per tick, averaging once every ~15-20 seconds per mob.
	// Previously `m.FireTicks%80 == 0` evaluated `0%80 == 0` (always true)
	// for every non-burning mob, causing a 5% roll every single tick (1 sound/s).
	if m.HurtTicks == 0 && rng.Float64() < 0.0025 {
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
			return SoundEmit{MobID: m.EntityID, Sound: string(s), Volume: 0.8, Pitch: 0.95 + float32(rng.Float64())*0.1, X: m.X, Y: m.Y, Z: m.Z, MobType: m.Type}
		}
	}
	_ = def
	return SoundEmit{}
}

// HurtSoundForMob returns the 1:1 vanilla Java sound identifier for a mob taking damage.
func HurtSoundForMob(mobType string) string {
	switch mobType {
	case "minecraft:zombie":
		return "minecraft:entity.zombie.hurt"
	case "minecraft:skeleton":
		return "minecraft:entity.skeleton.hurt"
	case "minecraft:creeper":
		return "minecraft:entity.creeper.hurt"
	case "minecraft:cow", "minecraft:mooshroom":
		return "minecraft:entity.cow.hurt"
	case "minecraft:pig":
		return "minecraft:entity.pig.hurt"
	case "minecraft:sheep":
		return "minecraft:entity.sheep.hurt"
	case "minecraft:chicken":
		return "minecraft:entity.chicken.hurt"
	case "minecraft:husk":
		return "minecraft:entity.husk.hurt"
	case "minecraft:stray":
		return "minecraft:entity.stray.hurt"
	case "minecraft:bogged":
		return "minecraft:entity.bogged.hurt"
	case "minecraft:drowned":
		return "minecraft:entity.drowned.hurt"
	case "minecraft:spider", "minecraft:cave_spider":
		return "minecraft:entity.spider.hurt"
	case "minecraft:slime":
		return "minecraft:entity.slime.hurt"
	case "minecraft:magma_cube":
		return "minecraft:entity.magma_cube.hurt"
	case "minecraft:phantom":
		return "minecraft:entity.phantom.hurt"
	case "minecraft:blaze":
		return "minecraft:entity.blaze.hurt"
	case "minecraft:ghast":
		return "minecraft:entity.ghast.hurt"
	case "minecraft:witch":
		return "minecraft:entity.witch.hurt"
	case "minecraft:enderman":
		return "minecraft:entity.enderman.hurt"
	case "minecraft:piglin":
		return "minecraft:entity.piglin.hurt"
	case "minecraft:piglin_brute":
		return "minecraft:entity.piglin_brute.hurt"
	case "minecraft:zombified_piglin":
		return "minecraft:entity.zombified_piglin.hurt"
	case "minecraft:wither_skeleton":
		return "minecraft:entity.wither_skeleton.hurt"
	case "minecraft:iron_golem":
		return "minecraft:entity.iron_golem.hurt"
	case "minecraft:snow_golem":
		return "minecraft:entity.snow_golem.hurt"
	case "minecraft:zombie_villager":
		return "minecraft:entity.zombie_villager.hurt"
	case "minecraft:villager":
		return "minecraft:entity.villager.hurt"
	case "minecraft:wandering_trader":
		return "minecraft:entity.wandering_trader.hurt"
	case "minecraft:pillager":
		return "minecraft:entity.pillager.hurt"
	case "minecraft:vindicator":
		return "minecraft:entity.vindicator.hurt"
	case "minecraft:evoker":
		return "minecraft:entity.evoker.hurt"
	case "minecraft:ravager":
		return "minecraft:entity.ravager.hurt"
	case "minecraft:vex":
		return "minecraft:entity.vex.hurt"
	case "minecraft:allay":
		return "minecraft:entity.allay.hurt_without_item"
	case "minecraft:guardian":
		return "minecraft:entity.guardian.hurt"
	case "minecraft:elder_guardian":
		return "minecraft:entity.elder_guardian.hurt"
	case "minecraft:shulker":
		return "minecraft:entity.shulker.hurt"
	case "minecraft:silverfish":
		return "minecraft:entity.silverfish.hurt"
	case "minecraft:endermite":
		return "minecraft:entity.endermite.hurt"
	case "minecraft:hoglin":
		return "minecraft:entity.hoglin.hurt"
	case "minecraft:zoglin":
		return "minecraft:entity.zoglin.hurt"
	case "minecraft:strider":
		return "minecraft:entity.strider.hurt"
	case "minecraft:warden":
		return "minecraft:entity.warden.hurt"
	case "minecraft:breeze":
		return "minecraft:entity.breeze.hurt"
	case "minecraft:creaking":
		return "minecraft:entity.creaking.sway"
	case "minecraft:camel":
		return "minecraft:entity.camel.hurt"
	case "minecraft:sniffer":
		return "minecraft:entity.sniffer.hurt"
	case "minecraft:armadillo":
		return "minecraft:entity.armadillo.hurt"
	case "minecraft:wolf":
		return "minecraft:entity.wolf.hurt"
	case "minecraft:cat":
		return "minecraft:entity.cat.hurt"
	case "minecraft:ocelot":
		return "minecraft:entity.ocelot.hurt"
	case "minecraft:fox":
		return "minecraft:entity.fox.hurt"
	case "minecraft:panda":
		return "minecraft:entity.panda.hurt"
	case "minecraft:polar_bear":
		return "minecraft:entity.polar_bear.hurt"
	case "minecraft:rabbit":
		return "minecraft:entity.rabbit.hurt"
	case "minecraft:horse":
		return "minecraft:entity.horse.hurt"
	case "minecraft:donkey":
		return "minecraft:entity.donkey.hurt"
	case "minecraft:mule":
		return "minecraft:entity.mule.hurt"
	case "minecraft:skeleton_horse":
		return "minecraft:entity.skeleton_horse.hurt"
	case "minecraft:zombie_horse":
		return "minecraft:entity.zombie_horse.hurt"
	case "minecraft:llama", "minecraft:trader_llama":
		return "minecraft:entity.llama.hurt"
	case "minecraft:goat":
		return "minecraft:entity.goat.hurt"
	case "minecraft:bee":
		return "minecraft:entity.bee.hurt"
	case "minecraft:bat":
		return "minecraft:entity.bat.hurt"
	case "minecraft:parrot":
		return "minecraft:entity.parrot.hurt"
	case "minecraft:frog":
		return "minecraft:entity.frog.hurt"
	case "minecraft:tadpole":
		return "minecraft:entity.tadpole.hurt"
	case "minecraft:axolotl":
		return "minecraft:entity.axolotl.hurt"
	case "minecraft:turtle":
		return "minecraft:entity.turtle.hurt"
	case "minecraft:dolphin":
		return "minecraft:entity.dolphin.hurt"
	case "minecraft:squid":
		return "minecraft:entity.squid.hurt"
	case "minecraft:glow_squid":
		return "minecraft:entity.glow_squid.hurt"
	case "minecraft:cod":
		return "minecraft:entity.cod.hurt"
	case "minecraft:salmon":
		return "minecraft:entity.salmon.hurt"
	case "minecraft:tropical_fish":
		return "minecraft:entity.tropical_fish.hurt"
	case "minecraft:pufferfish":
		return "minecraft:entity.puffer_fish.hurt"
	case "minecraft:wither":
		return "minecraft:entity.wither.hurt"
	case "minecraft:ender_dragon":
		return "minecraft:entity.ender_dragon.hurt"
	default:
		return string(SoundMobHurt)
	}
}

// DeathSoundForMob returns the 1:1 vanilla Java sound identifier for a mob dying.
func DeathSoundForMob(mobType string) string {
	switch mobType {
	case "minecraft:zombie":
		return "minecraft:entity.zombie.death"
	case "minecraft:skeleton":
		return "minecraft:entity.skeleton.death"
	case "minecraft:creeper":
		return "minecraft:entity.creeper.death"
	case "minecraft:cow", "minecraft:mooshroom":
		return "minecraft:entity.cow.death"
	case "minecraft:pig":
		return "minecraft:entity.pig.death"
	case "minecraft:sheep":
		return "minecraft:entity.sheep.death"
	case "minecraft:chicken":
		return "minecraft:entity.chicken.death"
	case "minecraft:husk":
		return "minecraft:entity.husk.death"
	case "minecraft:stray":
		return "minecraft:entity.stray.death"
	case "minecraft:bogged":
		return "minecraft:entity.bogged.death"
	case "minecraft:drowned":
		return "minecraft:entity.drowned.death"
	case "minecraft:spider", "minecraft:cave_spider":
		return "minecraft:entity.spider.death"
	case "minecraft:slime":
		return "minecraft:entity.slime.death"
	case "minecraft:magma_cube":
		return "minecraft:entity.magma_cube.death"
	case "minecraft:phantom":
		return "minecraft:entity.phantom.death"
	case "minecraft:blaze":
		return "minecraft:entity.blaze.death"
	case "minecraft:ghast":
		return "minecraft:entity.ghast.death"
	case "minecraft:witch":
		return "minecraft:entity.witch.death"
	case "minecraft:enderman":
		return "minecraft:entity.enderman.death"
	case "minecraft:piglin":
		return "minecraft:entity.piglin.death"
	case "minecraft:piglin_brute":
		return "minecraft:entity.piglin_brute.death"
	case "minecraft:zombified_piglin":
		return "minecraft:entity.zombified_piglin.death"
	case "minecraft:wither_skeleton":
		return "minecraft:entity.wither_skeleton.death"
	case "minecraft:iron_golem":
		return "minecraft:entity.iron_golem.death"
	case "minecraft:snow_golem":
		return "minecraft:entity.snow_golem.death"
	case "minecraft:zombie_villager":
		return "minecraft:entity.zombie_villager.death"
	case "minecraft:villager":
		return "minecraft:entity.villager.death"
	case "minecraft:wandering_trader":
		return "minecraft:entity.wandering_trader.death"
	case "minecraft:pillager":
		return "minecraft:entity.pillager.death"
	case "minecraft:vindicator":
		return "minecraft:entity.vindicator.death"
	case "minecraft:evoker":
		return "minecraft:entity.evoker.death"
	case "minecraft:ravager":
		return "minecraft:entity.ravager.death"
	case "minecraft:vex":
		return "minecraft:entity.vex.death"
	case "minecraft:allay":
		return "minecraft:entity.allay.death"
	case "minecraft:guardian":
		return "minecraft:entity.guardian.death"
	case "minecraft:elder_guardian":
		return "minecraft:entity.elder_guardian.death"
	case "minecraft:shulker":
		return "minecraft:entity.shulker.death"
	case "minecraft:silverfish":
		return "minecraft:entity.silverfish.death"
	case "minecraft:endermite":
		return "minecraft:entity.endermite.death"
	case "minecraft:hoglin":
		return "minecraft:entity.hoglin.death"
	case "minecraft:zoglin":
		return "minecraft:entity.zoglin.death"
	case "minecraft:strider":
		return "minecraft:entity.strider.death"
	case "minecraft:warden":
		return "minecraft:entity.warden.death"
	case "minecraft:breeze":
		return "minecraft:entity.breeze.death"
	case "minecraft:creaking":
		return "minecraft:entity.creaking.death"
	case "minecraft:camel":
		return "minecraft:entity.camel.death"
	case "minecraft:sniffer":
		return "minecraft:entity.sniffer.death"
	case "minecraft:armadillo":
		return "minecraft:entity.armadillo.death"
	case "minecraft:wolf":
		return "minecraft:entity.wolf.death"
	case "minecraft:cat":
		return "minecraft:entity.cat.death"
	case "minecraft:ocelot":
		return "minecraft:entity.ocelot.death"
	case "minecraft:fox":
		return "minecraft:entity.fox.death"
	case "minecraft:panda":
		return "minecraft:entity.panda.death"
	case "minecraft:polar_bear":
		return "minecraft:entity.polar_bear.death"
	case "minecraft:rabbit":
		return "minecraft:entity.rabbit.death"
	case "minecraft:horse":
		return "minecraft:entity.horse.death"
	case "minecraft:donkey":
		return "minecraft:entity.donkey.death"
	case "minecraft:mule":
		return "minecraft:entity.mule.death"
	case "minecraft:skeleton_horse":
		return "minecraft:entity.skeleton_horse.death"
	case "minecraft:zombie_horse":
		return "minecraft:entity.zombie_horse.death"
	case "minecraft:llama", "minecraft:trader_llama":
		return "minecraft:entity.llama.death"
	case "minecraft:goat":
		return "minecraft:entity.goat.death"
	case "minecraft:bee":
		return "minecraft:entity.bee.death"
	case "minecraft:bat":
		return "minecraft:entity.bat.death"
	case "minecraft:parrot":
		return "minecraft:entity.parrot.death"
	case "minecraft:frog":
		return "minecraft:entity.frog.death"
	case "minecraft:tadpole":
		return "minecraft:entity.tadpole.death"
	case "minecraft:axolotl":
		return "minecraft:entity.axolotl.death"
	case "minecraft:turtle":
		return "minecraft:entity.turtle.death"
	case "minecraft:dolphin":
		return "minecraft:entity.dolphin.death"
	case "minecraft:squid":
		return "minecraft:entity.squid.death"
	case "minecraft:glow_squid":
		return "minecraft:entity.glow_squid.death"
	case "minecraft:cod":
		return "minecraft:entity.cod.death"
	case "minecraft:salmon":
		return "minecraft:entity.salmon.death"
	case "minecraft:tropical_fish":
		return "minecraft:entity.tropical_fish.death"
	case "minecraft:pufferfish":
		return "minecraft:entity.puffer_fish.death"
	case "minecraft:wither":
		return "minecraft:entity.wither.death"
	case "minecraft:ender_dragon":
		return "minecraft:entity.ender_dragon.death"
	default:
		return string(SoundMobDeath)
	}
}
