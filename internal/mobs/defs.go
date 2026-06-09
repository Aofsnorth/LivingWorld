// Package mobs — per-mob-type attribute table. The values here are vanilla
// 1.20.x, used by the AI in ai.go. They are deliberately not load-balanced:
//   - follow_range is the radius the mob will *try* to detect a target. Vanilla
//     applies regional-difficulty / spawn-bonus modifiers, which are out of
//     scope for v1; the random-spawn-bonus (Gaussian 0..0.05 additive) is
//     folded into EffectiveFollowRange per mob at spawn time.
//   - movement_speed is the on-foot *attribute* value. The actual blocks/tick
//     = attribute * 0.1 (vanilla). The AI multiplies the attribute by 0.1
//     inside the integrator; the per-tick delta is attribute * 0.1 * dt.
//   - attack_damage is half-hearts (vanilla hearts). 1 hp = 0.5 hearts.
//   - attack_range is the distance at which a melee hit registers (squared
//     distance check is cheaper).
//   - attack_cooldown is in ticks at 20 Hz. Vanilla zombie: 20 (1s),
//     skeleton: 40-60 (2-3s on land).
//   - has_ranged: spawns an arrow at the target. attack_cooldown applies to
//     the bow draw; the arrow is fired at full draw.
//   - has_explosion: creeper. fuse_ticks is the time from "in range" to boom.
//     explosion_power is the radius (3 for normal creeper).
//   - burns_in_daylight: zombie + skeleton. 1 hp/s while sky-light >= 12
//     AND no head armor (skeleton never wears one; zombie only burns if the
//     mob has no helmet — we don't track that today, so they always burn).
package mobs

// Dimension constants for spawn rules. Mirrored from
// internal/world.Dimension so the mobs package doesn't import
// world (which would create an import cycle: world imports
// mobs for the mob store, mobs would import world for
// Dimension).
const (
	DimOverworld = "overworld"
	DimNether    = "nether"
	DimEnd       = "end"
)

// MobDef holds the vanilla 1.20 attribute set for one mob type. Lookup is
// done by Type (the namespaced mob id, e.g. "minecraft:zombie") in defs().
type MobDef struct {
	Type string

	IsHostile bool

	FollowRange float64 // vanilla: zombie 35, skel/creeper 16, passive 16
	WanderSpeed float64 // attribute value (multiply by 0.1 to get b/tick)
	ChaseSpeed  float64 // attribute value when pursuing
	AttackDamage float64
	AttackRange float64 // squared distance is checked; ~2² = 4 for melee
	AttackCooldown int

	// Skeleton only:
	HasRanged bool
	// M1: ranged projectile flavor. "arrow" (skeleton/stray/bogged),
	// "small_fireball" (blaze), "large_fireball" (ghast), "potion"
	// (witch), "none". Decides what OnShootArrow actually spawns.
	RangedProjectile string
	// M1: how many ticks the ranged attack warms up. Skeleton = 20
	// (draw bow), blaze = 10 (small fireball), ghast = 40 (wail +
	// fireball).
	RangedWarmupTicks int
	// M1: explosion radius for fireballs. Small = 1, large = 3.
	FireballRadius float64
	// M1: fireball damage (in hearts).
	FireballDamage float32

	// Creeper only:
	HasExplosion    bool
	FuseTicks       int // time from "in range" to boom (30 = 1.5s)
	ExplosionPower  int // 3 for normal creeper
	ExplosionRadius float64

	// M1: on-hit status effect applied to the target on a melee swing.
	// Status effects themselves (the inventory side) land in M5; for
	// M1 we apply the effect directly via the world layer. Empty
	// Type means "regular melee damage only".
	OnHit HitEffect

	// Zombie + skeleton only:
	BurnsInDaylight bool

	// M0.6: HP. Vanilla zombie/skeleton/creeper have 20; passive
	// animals vary (cow/sheep/pig 10, chicken 4). Use 20 / 20 / 20
	// / 10 for the v1 set; M1 adds the rest.
	MaxHP float32

	// M1: size. Slime / magma cube are tagged with a size (1, 2, or
	// 4) which controls HP, hitbox, and split-on-death. Other mobs
	// leave this at 0.
	Size int

	// M1: split behaviour. true = on death, spawn (Size-1) copies
	// of the mob with smaller size. false = no split.
	SplitsOnDeath bool

	// M1: ambient fire immunity (magma cube, blaze, wither
	// skeleton). true = no fire/burn damage.
	FireImmune bool

	// M1: water damage. Endermen take 1 HP/s in water or rain;
	// snow golems also melt. Vanilla formula.
	WaterSensitive bool

	// M1: ranged (bows) flight. Slime/magma cube hop (jumping
	// movement, not gravity walk). Phantom flies (no gravity,
	// const Y glide). Ghast hovers. Iron golem walks + throws.
	Movement string // "walk" | "hop" | "fly" | "hover"

	// M1: throw-power. Iron golem throws picked-up players 3+
	// blocks up. Other mobs leave this 0.
	ThrowDamage float32

	// M1: drops. A list of (item id, count min, count max, chance
	// 0-1). Spawned by the world tick when the mob is killed.
	Drops []Drop

	// M2: natural-spawn rule. nil means "do not spawn naturally
	// (only via /summon, spawn egg, structure generation)".
	// The world tick director evaluates CanSpawn at every
	// candidate column and picks from the matching pool.
	Spawn *SpawnRule

	// M7.10: BreaksDoors. Zombie variants (zombie, husk,
	// zombie_villager, drowned) can smash a wooden door in
	// the way of their path. Vanilla 1.20: 1.5× normal door
	// break hardness on hard difficulty, takes ~30 ticks to
	// fully break a door. We model the "block the path"
	// case by removing the door block (the world layer
	// resolves the actual block break) when the mob tries
	// to step into a door cell.
	BreaksDoors bool

	// M7.10: AggressiveAtNight. Spiders are hostile at night
	// and neutral during the day (a daylight spider walks
	// around, doesn't chase, but retaliates if hit). The AI
	// gate is in pickTarget: a daytime spider falls back to
	// StateIdle. Other mobs leave this false.
	AggressiveAtNight bool

	// M7.10: AggressiveUnlessGold. Piglins are hostile
	// unless the target is wearing at least one piece of
	// gold armor (vanilla rule). The AI gate is in
	// pickTarget: a gold-armored player is ignored. Other
	// mobs leave this false.
	AggressiveUnlessGold bool

	// M7.10: NoKnockback. Slimes and magma cubes have
	// vanilla 0 knockback resistance. A hit that would
	// normally set KnockbackVX/VY/VZ is suppressed.
	NoKnockback bool

	// M7.10: BabyChance. 0..1. Vanilla zombies (and
	// variants) have a 5% chance of spawning as a baby.
	// Babies move 1.5× faster, are 0.5× tall, and have the
	// same HP. The chance is rolled at spawn time by the
	// world layer (M0.6 spawned the mob; M7.10 adds the
	// baby flag on the existing spawn call). Other mobs
	// leave this 0.
	BabyChance float32

	// FoodItem is the namespaced item id that attracts this
	// passive mob when held by a player. Vanilla: wheat for
	// cow/sheep, carrot/potato/beetroot for pig, seeds for
	// chicken. Empty means "no food attraction". The AI
	// checks the nearest player's held item via HeldItem
	// callback; if it matches, the mob enters StateFollow.
	FoodItem string

	// AggravatedByGaze marks a mob (enderman) that is neutral until a
	// player looks directly at it. The brain-backed gaze target goal
	// (ai_goals_brain.go) only aggros players whose view cone is on the
	// mob's head, and keeps it angry for a timer after provocation. Other
	// mobs leave this false and use the normal nearest-target acquisition.
	AggravatedByGaze bool
}

// LightAny is the sentinel for "don't care" in SpawnRule.MinLight
// and MaxLight. -1 is used because light is 0..15 in vanilla.
const (
	LightAny = -1
)

// SpawnCategory classifies the mob for the global cap (passive vs
// hostile vs neutral). Hostile mobs are suppressed on peaceful
// difficulty. Neutral mobs (enderman, piglin, iron golem) are
// not capped as hostiles.
type SpawnCategory int

const (
	SpawnPassive SpawnCategory = iota // cow, pig, sheep, chicken
	SpawnHostile                      // zombie, skeleton, creeper, ...
	SpawnNeutral                      // enderman, piglin, iron golem, spider
)

// SpawnRule is the per-mob natural-spawn condition set. Empty /
// zero values mean "don't care" (any). The director combines the
// rule with the candidate column's block / light / sky / biome
// values from the world.
//
// M2: a mob with Spawn == nil does not appear via natural spawning
// (only /summon, spawn egg, structure gen). The 7 vanilla-hostile
// mobs (zombie, skeleton, creeper, spider, enderman, ...) and the
// 4 passives (cow, pig, sheep, chicken) all get a Spawn rule.
//
// Vanilla reference (M.OBVIOUS rules, simplified for v1):
//
//	cow / pig / sheep / chicken:  overworld, day, grass_block,
//	    light >= 9
//	zombie / skeleton / creeper:  overworld, night OR dark cave,
//	    light <= 7
//	spider:                       overworld, night, light <= 7
//	enderman:                     overworld OR end, night OR
//	    light <= 7
//	cave_spider:                  overworld, light <= 0
//	slime:                        overworld, swamp, light <= 7
//	magma_cube:                   nether, light-agnostic
//	phantom:                      overworld, night, sky light 15
//	blaze:                        nether, light-agnostic
//	ghast:                        nether, open sky above
//	witch:                        overworld, light <= 7
//	piglin:                       nether, light-agnostic
//	wither_skeleton:              nether, light-agnostic
//	husk:                         overworld, desert, night,
//	    light <= 7
//	drowned:                      overworld, ocean / river,
//	    light <= 7
//	struck:                       overworld, snow biome, light <= 7
//	bogged:                       overworld, mangrove, light <= 7
//	zombie_villager:              5% of zombie spawns (the
//	    director's zombie picker rolls a separate 5% chance)
//	iron_golem:                   not spawnable in v1 (villager
//	    only)
type SpawnRule struct {
	// Category for the global cap. Required.
	Category SpawnCategory
	// Dimension restricts the rule to a specific dimension.
	// Empty string means "any".
	Dimension string
	// MinLight and MaxLight bound the light level at the spawn
	// column. -1 in either means "don't care". 0..15.
	MinLight, MaxLight int
	// RequireDark means the sky light at the spawn column's
	// surface must be 0 (true cave). Used by cave_spider.
	RequireDark bool
	// RequireOpenSky means the column has at least 8 cells of
	// open air above it (ghast rule).
	RequireOpenSky bool
	// RequireSkyLight15 means the sky light at the spawn
	// column's head is 15 (phantom rule).
	RequireSkyLight15 bool
	// NightOnly / DayOnly restrict the rule to a time-of-day
	// window. Neither = any.
	NightOnly bool
	DayOnly   bool
	// Surfaces is the namespaced set of acceptable blocks at
	// the mob's feet. Empty = any.
	Surfaces []string
	// Cap is the per-mob-type cap on top of the global cap.
	// 0 = use the global capPassive/capHostile.
	Cap int
	// Chance is the per-attempt probability of picking this
	// mob. 0 = use default 1.0. <1 weights the picker.
	Chance float32
}

// HitEffect is the on-hit status effect applied by a hostile mob. The
// effect itself (icon, particle, modifiers) is not implemented in M1;
// we apply the underlying damage / slowness / etc. directly via the
// world layer's player API. M5 wires the full effect model.
type HitEffect struct {
	Type    string // "hunger" | "wither" | "poison" | "slowness" | "instant_damage" | "levitation" | ""
	Level   int    // amplifier (0 = I, 1 = II, ...)
	Seconds int    // duration in ticks / 20
}

// Drop is one entry in a mob's loot table. Empty Item means "no
// drop"; chance < 1 means a roll on each kill. The world tick
// resolves this when the mob's HP hits 0.
type Drop struct {
	Item     string
	Min, Max int
	Chance   float32 // 0..1, default 1.0 (guaranteed)
}

// defs returns the per-type attribute table. A mob whose Type is not in the
// table falls back to the cow entry (passive, no attack) so a future mob
// added to the spawn director renders and moves without crashing.
func defs() map[string]MobDef {
	return map[string]MobDef{
"minecraft:cow": {
		Type: "minecraft:cow", IsHostile: false,
		FollowRange: 16, WanderSpeed: 0.20,
		AttackRange: 4, AttackCooldown: 0,
		MaxHP: 10,
		FoodItem: "minecraft:wheat",
			Spawn: &SpawnRule{
				Category: SpawnPassive,
				Dimension: DimOverworld,
				DayOnly:   true,
				MinLight:  9, MaxLight: 15,
				Surfaces: []string{"minecraft:grass_block"},
			},
		},
"minecraft:pig": {
		Type: "minecraft:pig", IsHostile: false,
		FollowRange: 16, WanderSpeed: 0.20,
		AttackRange: 4, AttackCooldown: 0,
		MaxHP: 10,
		FoodItem: "minecraft:carrot",
			Spawn: &SpawnRule{
				Category: SpawnPassive,
				Dimension: DimOverworld,
				DayOnly:   true,
				MinLight:  9, MaxLight: 15,
				Surfaces: []string{"minecraft:grass_block"},
			},
		},
"minecraft:sheep": {
		Type: "minecraft:sheep", IsHostile: false,
		FollowRange: 16, WanderSpeed: 0.20,
		AttackRange: 4, AttackCooldown: 0,
		MaxHP: 8,
		FoodItem: "minecraft:wheat",
			Spawn: &SpawnRule{
				Category: SpawnPassive,
				Dimension: DimOverworld,
				DayOnly:   true,
				MinLight:  9, MaxLight: 15,
				Surfaces: []string{"minecraft:grass_block"},
			},
		},
"minecraft:chicken": {
		Type: "minecraft:chicken", IsHostile: false,
		FollowRange: 16, WanderSpeed: 0.20,
		AttackRange: 4, AttackCooldown: 0,
		MaxHP: 4,
		FoodItem: "minecraft:wheat_seeds",
			Spawn: &SpawnRule{
				Category: SpawnPassive,
				Dimension: DimOverworld,
				DayOnly:   true,
				MinLight:  9, MaxLight: 15,
				Surfaces: []string{"minecraft:grass_block"},
			},
		},
		"minecraft:zombie": {
			Type: "minecraft:zombie", IsHostile: true,
			FollowRange: 35, WanderSpeed: 0.23, ChaseSpeed: 0.35,
			AttackDamage: 3, AttackRange: 4, AttackCooldown: 20,
			BurnsInDaylight: true,
			MaxHP: 20,
			BreaksDoors: true,
			BabyChance:  0.05,
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimOverworld,
				NightOnly: true,
				MaxLight:  7,
				Surfaces: []string{"minecraft:grass_block", "minecraft:sand", "minecraft:snow_block", "minecraft:dirt"},
				Cap:       50,
			},
		},
		"minecraft:skeleton": {
			Type: "minecraft:skeleton", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.25, ChaseSpeed: 0.35,
			AttackDamage: 2, AttackRange: 225, // 15 blocks squared — kite at 15
			AttackCooldown: 40, // 2s on normal
			HasRanged: true,
			BurnsInDaylight: true,
			MaxHP: 20,
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimOverworld,
				NightOnly: true,
				MaxLight:  7,
				Surfaces: []string{"minecraft:grass_block", "minecraft:sand", "minecraft:snow_block"},
				Cap:       30,
			},
		},
		"minecraft:creeper": {
			Type: "minecraft:creeper", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.20, ChaseSpeed: 0.35,
			AttackRange: 4, AttackCooldown: 0,
			HasExplosion: true, FuseTicks: 30, ExplosionPower: 3,
			ExplosionRadius: 3.0,
			MaxHP: 20,
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimOverworld,
				NightOnly: true,
				MaxLight:  7,
				Surfaces: []string{"minecraft:grass_block", "minecraft:sand", "minecraft:snow_block"},
				Cap:       20,
			},
		},

		// --- M1: zombie variants ---
		"minecraft:husk": {
			Type: "minecraft:husk", IsHostile: true,
			FollowRange: 35, WanderSpeed: 0.23, ChaseSpeed: 0.35,
			AttackDamage: 2, AttackRange: 4, AttackCooldown: 20,
			// Husk does NOT burn in daylight and applies Hunger
			// (1.0) on hit instead of regular damage scaling. We
			// keep the 2-3 hp base damage and add the effect.
			BurnsInDaylight: false,
			OnHit:           HitEffect{Type: "hunger", Level: 0, Seconds: 14 * 20},
			MaxHP:           20,
			BreaksDoors:     true,
			BabyChance:      0.05,
			Drops: []Drop{
				{Item: "minecraft:rotten_flesh", Min: 0, Max: 2, Chance: 1.0},
				{Item: "minecraft:iron_ingot", Min: 0, Max: 1, Chance: 0.05},
				{Item: "minecraft:carrot", Min: 0, Max: 1, Chance: 0.05},
				{Item: "minecraft:potato", Min: 0, Max: 1, Chance: 0.05},
			},
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimOverworld,
				NightOnly: true,
				MaxLight:  7,
				Surfaces: []string{"minecraft:sand"},
				Cap:       8,
			},
		},
		"minecraft:zombie_villager": {
			Type: "minecraft:zombie_villager", IsHostile: true,
			FollowRange: 35, WanderSpeed: 0.23, ChaseSpeed: 0.35,
			AttackDamage: 3, AttackRange: 4, AttackCooldown: 20,
			BurnsInDaylight: true,
			MaxHP:           20,
			BreaksDoors:     true,
			BabyChance:      0.05,
			Drops: []Drop{
				{Item: "minecraft:rotten_flesh", Min: 0, Max: 2, Chance: 1.0},
				{Item: "minecraft:iron_ingot", Min: 0, Max: 1, Chance: 0.05},
				{Item: "minecraft:carrot", Min: 0, Max: 1, Chance: 0.05},
				{Item: "minecraft:potato", Min: 0, Max: 1, Chance: 0.05},
			},
			// 5% chance on a normal zombie spawn — handled in the
			// director's picker, not as a separate candidate.
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimOverworld,
				NightOnly: true,
				MaxLight:  7,
				Surfaces: []string{"minecraft:grass_block", "minecraft:sand", "minecraft:snow_block"},
				Cap:       10,
				Chance:    0.05, // weighted 1/20 of zombie-like spawns
			},
		},
		"minecraft:drowned": {
			Type: "minecraft:drowned", IsHostile: true,
			FollowRange: 35, WanderSpeed: 0.23, ChaseSpeed: 0.35,
			AttackDamage: 3, AttackRange: 4, AttackCooldown: 20,
			BurnsInDaylight: false, // drowned don't burn in sun
			// 8.5% chance of being a "trident drowned" that throws
			// a trident; the rest melee. For v1 we make ALL
			// drowned throw tridents (a separate MobDef could
			// model the variant).
			HasRanged:        true,
			RangedProjectile: "trident",
			RangedWarmupTicks: 20,
			MaxHP:            20,
			BreaksDoors:      true,
			BabyChance:       0.05,
			Drops: []Drop{
				{Item: "minecraft:rotten_flesh", Min: 0, Max: 2, Chance: 1.0},
				{Item: "minecraft:gold_ingot", Min: 0, Max: 1, Chance: 0.05},
				{Item: "minecraft:fish", Min: 0, Max: 1, Chance: 0.05},
			},
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimOverworld,
				MaxLight:  7,
				// vanilla: ocean / river / swamp biomes. v1
				// approximates with surfaces (water above
				// sand / dirt). The director checks the
				// block at the candidate column.
				Surfaces: []string{"minecraft:gravel", "minecraft:sand", "minecraft:dirt", "minecraft:clay"},
				Cap:      12,
			},
		},

		// --- M1: skeleton variants ---
		"minecraft:stray": {
			Type: "minecraft:stray", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.25, ChaseSpeed: 0.35,
			AttackDamage: 2, AttackRange: 225,
			AttackCooldown: 40,
			HasRanged:      true,
			// Stray fires Arrow of Slowness (0:07s = 140 ticks).
			RangedProjectile: "arrow_slowness",
			RangedWarmupTicks: 20,
			BurnsInDaylight: true,
			MaxHP:           20,
			Drops: []Drop{
				{Item: "minecraft:bone", Min: 0, Max: 2, Chance: 1.0},
				{Item: "minecraft:arrow", Min: 0, Max: 2, Chance: 1.0},
			},
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimOverworld,
				NightOnly: true,
				MaxLight:  7,
				Surfaces: []string{"minecraft:snow_block", "minecraft:snow", "minecraft:ice"},
				Cap:       10,
			},
		},
		"minecraft:bogged": {
			Type: "minecraft:bogged", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.25, ChaseSpeed: 0.35,
			AttackDamage: 2, AttackRange: 225,
			AttackCooldown: 40,
			HasRanged:      true,
			// Bogged (1.21) fires Arrow of Poison.
			RangedProjectile: "arrow_poison",
			RangedWarmupTicks: 20,
			BurnsInDaylight: true,
			MaxHP:           16,
			Drops: []Drop{
				{Item: "minecraft:bone", Min: 0, Max: 2, Chance: 1.0},
				{Item: "minecraft:arrow", Min: 0, Max: 2, Chance: 1.0},
			},
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimOverworld,
				NightOnly: true,
				MaxLight:  7,
				// Mangrove swamp in vanilla; v1
				// approximates with mud block.
				Surfaces: []string{"minecraft:mud", "minecraft:muddy_mangrove_roots", "minecraft:mangrove_roots"},
				Cap:      4,
			},
		},
		"minecraft:wither_skeleton": {
			Type: "minecraft:wither_skeleton", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.25, ChaseSpeed: 0.4,
			AttackDamage: 4, AttackRange: 4, AttackCooldown: 20,
			// Wither effect on hit (II for 10 s = 200 ticks).
			OnHit:           HitEffect{Type: "wither", Level: 0, Seconds: 10 * 20},
			FireImmune:      true,
			BurnsInDaylight: false,
			MaxHP:           20,
			Drops: []Drop{
				{Item: "minecraft:coal", Min: 0, Max: 1, Chance: 0.5},
				{Item: "minecraft:bone", Min: 0, Max: 2, Chance: 1.0},
				{Item: "minecraft:skull", Min: 0, Max: 1, Chance: 0.025}, // wither skull
			},
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimNether,
				MaxLight:  15, // light is meaningless in the nether; just bound
				Surfaces: []string{"minecraft:nether_brick", "minecraft:netherrack"},
				Cap:       8,
			},
		},

		// --- M1: cave spider (small + poison) ---
		"minecraft:cave_spider": {
			Type: "minecraft:cave_spider", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.30, ChaseSpeed: 0.45,
			AttackDamage: 2, AttackRange: 4, AttackCooldown: 20,
			// Cave spider inflicts poison (I, 7 s = 140 ticks) and
			// is smaller (Size 0 → no special handling; same as
			// spider for v1).
			OnHit: HitEffect{Type: "poison", Level: 0, Seconds: 7 * 20},
			MaxHP: 12,
			Drops: []Drop{
				{Item: "minecraft:string", Min: 0, Max: 2, Chance: 1.0},
				{Item: "minecraft:spider_eye", Min: 0, Max: 1, Chance: 0.33},
			},
			Spawn: &SpawnRule{
				Category:   SpawnHostile,
				Dimension:  DimOverworld,
				RequireDark: true, // mine / cave only in v1
				Surfaces:   []string{"minecraft:cobblestone", "minecraft:stone", "minecraft:mossy_cobblestone", "minecraft:cobbled_deepslate"},
				Cap:        4,
			},
		},

		// --- M1: spider (big) ---
		"minecraft:spider": {
			Type: "minecraft:spider", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.30, ChaseSpeed: 0.45,
			AttackDamage: 2, AttackRange: 4, AttackCooldown: 20,
			BurnsInDaylight: false, // neutral in daylight
			Movement:          "climb", // can walk up walls
			AggressiveAtNight: true,  // neutral during the day
			MaxHP:             16,
			Drops: []Drop{
				{Item: "minecraft:string", Min: 0, Max: 2, Chance: 1.0},
				{Item: "minecraft:spider_eye", Min: 0, Max: 1, Chance: 0.33},
			},
			Spawn: &SpawnRule{
				Category:  SpawnNeutral,
				Dimension: DimOverworld,
				NightOnly: true,
				MaxLight:  7,
				Surfaces: []string{"minecraft:grass_block", "minecraft:sand", "minecraft:dirt"},
				Cap:       12,
			},
		},

		// --- M1: slime (size 1/2/4) ---
		"minecraft:slime": {
			Type: "minecraft:slime", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.20, ChaseSpeed: 0.30,
			AttackDamage: 2, AttackRange: 4, AttackCooldown: 20,
			Movement:       "hop", // jumping motion
			SplitsOnDeath:  true,
			BurnsInDaylight: false,
			NoKnockback:     true, // vanilla knockback_resistance=1.0
			MaxHP:           16,
			Size:            1, // M1.4: child default
			Drops: []Drop{
				{Item: "minecraft:slime_ball", Min: 0, Max: 2, Chance: 1.0},
			},
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimOverworld,
				MaxLight:  7,
				// vanilla: swamp biome. v1 surfaces are
				// proxies; the director still checks
				// light ≤ 7.
				Surfaces: []string{"minecraft:grass_block", "minecraft:mud", "minecraft:slime"},
				Chance:   0.25, // 1/4 of attempts (vanilla weight)
				Cap:      6,
			},
		},

		// --- M1: magma cube (size 1/2/4) ---
		"minecraft:magma_cube": {
			Type: "minecraft:magma_cube", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.20, ChaseSpeed: 0.30,
			AttackDamage: 3, AttackRange: 4, AttackCooldown: 20,
			Movement:       "hop",
			SplitsOnDeath:  true,
			FireImmune:     true,
			BurnsInDaylight: false,
			NoKnockback:    true,
			MaxHP:           16,
			Size:            1,
			Drops: []Drop{
				{Item: "minecraft:magma_cream", Min: 0, Max: 1, Chance: 0.5},
			},
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimNether,
				Surfaces:  []string{"minecraft:netherrack", "minecraft:soul_sand", "minecraft:soul_soil", "minecraft:magma_block"},
				Chance:    0.25,
				Cap:       6,
			},
		},

		// --- M1: phantom ---
		"minecraft:phantom": {
			Type: "minecraft:phantom", IsHostile: true,
			FollowRange: 20, WanderSpeed: 0.18, ChaseSpeed: 0.35,
			AttackDamage: 3, AttackRange: 9, AttackCooldown: 20,
			Movement:       "fly", // const Y + dive
			BurnsInDaylight: false,
			MaxHP:           20,
			Drops: []Drop{
				{Item: "minecraft:phantom_membrane", Min: 0, Max: 1, Chance: 0.5},
			},
			Spawn: &SpawnRule{
				Category:          SpawnHostile,
				Dimension:         DimOverworld,
				NightOnly:         true,
				MinLight:          LightAny,
				MaxLight:          LightAny,
				RequireSkyLight15: true, // open night sky
				Cap:               4,
				// vanilla: 3+ sleepless nights. v1 doesn't
				// track sleepless days — phantoms still
				// spawn at night as a v1 simplification.
			},
		},

		// --- M1: blaze (small fireball) ---
		"minecraft:blaze": {
			Type: "minecraft:blaze", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.20, ChaseSpeed: 0.30,
			AttackDamage: 3, AttackRange: 225, // shoots from a distance
			AttackCooldown:  20,
			HasRanged:       true,
			RangedProjectile: "small_fireball",
			RangedWarmupTicks: 10,
			Movement:        "fly", // hovers
			FireImmune:      true,
			BurnsInDaylight: false,
			MaxHP:           20,
			FireballRadius:  1.0,
			FireballDamage:  2.5,
			Drops: []Drop{
				{Item: "minecraft:blaze_rod", Min: 0, Max: 1, Chance: 0.5},
			},
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimNether,
				MaxLight:  15,
				Surfaces:  []string{"minecraft:nether_brick", "minecraft:netherrack"},
				Chance:    0.5, // 1/2 of nether attempts (vanilla weight)
				Cap:       6,
			},
		},

		// --- M1: ghast (large fireball) ---
		"minecraft:ghast": {
			Type: "minecraft:ghast", IsHostile: true,
			FollowRange: 64, WanderSpeed: 0.10, ChaseSpeed: 0.15,
			AttackDamage: 3, AttackRange: 1600, // 40 blocks squared
			AttackCooldown:  40,
			HasRanged:       true,
			RangedProjectile: "large_fireball",
			RangedWarmupTicks: 40, // 2 s wail then fire
			Movement:        "hover", // big, slow, drifts
			FireImmune:      true,
			BurnsInDaylight: false,
			MaxHP:           20,
			FireballRadius:  3.0,
			FireballDamage:  6,
			Drops: []Drop{
				{Item: "minecraft:ghast_tear", Min: 0, Max: 1, Chance: 0.5},
				{Item: "minecraft:gunpowder", Min: 0, Max: 2, Chance: 0.5},
			},
			Spawn: &SpawnRule{
				Category:       SpawnHostile,
				Dimension:      DimNether,
				RequireOpenSky: true, // 8+ cells of open air above
				MaxLight:       15,
				Cap:            4,
			},
		},

		// --- M1: witch (potion thrower) ---
		"minecraft:witch": {
			Type: "minecraft:witch", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.20, ChaseSpeed: 0.30,
			AttackDamage: 0, AttackRange: 225, // throws potions instead
			AttackCooldown:  40,
			HasRanged:       true,
			RangedProjectile: "potion",
			RangedWarmupTicks: 20,
			BurnsInDaylight: false,
			MaxHP:           26,
			Drops: []Drop{
				{Item: "minecraft:glowstone_dust", Min: 0, Max: 1, Chance: 0.5},
				{Item: "minecraft:redstone", Min: 0, Max: 1, Chance: 0.5},
				{Item: "minecraft:sugar", Min: 0, Max: 1, Chance: 0.5},
				{Item: "minecraft:spider_eye", Min: 0, Max: 1, Chance: 0.5},
			},
			Spawn: &SpawnRule{
				Category:  SpawnHostile,
				Dimension: DimOverworld,
				MaxLight:  7,
				Surfaces:  []string{"minecraft:grass_block", "minecraft:mud", "minecraft:swamp"},
				Chance:    0.05, // 5% of swamp attempts
				Cap:       1,
			},
		},

		// --- M1: enderman ---
		"minecraft:enderman": {
			Type: "minecraft:enderman", IsHostile: true,
			FollowRange: 64, WanderSpeed: 0.30, ChaseSpeed: 0.45,
			AttackDamage: 4, AttackRange: 4, AttackCooldown: 20,
			WaterSensitive:   true, // 1 HP/s in water
			BurnsInDaylight:  false,
			AggravatedByGaze: true, // neutral until looked at (vanilla)
			MaxHP:            40,
			Drops: []Drop{
				{Item: "minecraft:ender_pearl", Min: 0, Max: 1, Chance: 0.5},
			},
			Spawn: &SpawnRule{
				Category:  SpawnNeutral,
				Dimension: DimOverworld,
				MaxLight:  7,
				Surfaces:  []string{"minecraft:grass_block", "minecraft:sand", "minecraft:end_stone"},
				Cap:       4,
			},
		},

		// --- M1: piglin (gold neutral) ---
		// M7.10: vanilla piglin is "neutral by default" but
		// attacks any non-gold-armored player on sight (and
		// any wither skeleton). To model the gold-armor
		// neutrality, we mark IsHostile=true so pickTarget
		// runs, and gate the pick with AggressiveUnlessGold
		// (skips gold-armored players). The mob effectively
		// behaves "hostile to all but gold-armored players"
		// — the same observable behaviour as vanilla.
		"minecraft:piglin": {
			Type: "minecraft:piglin", IsHostile: true,
			FollowRange: 16, WanderSpeed: 0.20, ChaseSpeed: 0.35,
			AttackDamage: 3, AttackRange: 4, AttackCooldown: 20,
			BurnsInDaylight: false,
			AggressiveUnlessGold: true,
			MaxHP:                16,
			Drops: []Drop{
				{Item: "minecraft:porkchop", Min: 1, Max: 3, Chance: 1.0},
				{Item: "minecraft:gold_nugget", Min: 1, Max: 3, Chance: 0.5},
			},
			Spawn: &SpawnRule{
				Category:  SpawnNeutral,
				Dimension: DimNether,
				MaxLight:  15,
				Surfaces:  []string{"minecraft:netherrack", "minecraft:soul_sand", "minecraft:soul_soil", "minecraft:grass_block"},
				Cap:       12,
			},
		},

		// --- M1: iron golem (defender) ---
		"minecraft:iron_golem": {
			Type: "minecraft:iron_golem", IsHostile: false, // neutral
			FollowRange: 16, WanderSpeed: 0.20, ChaseSpeed: 0.40,
			AttackDamage: 6, AttackRange: 9, AttackCooldown: 20,
			ThrowDamage:    4.0,
			BurnsInDaylight: false,
			MaxHP:           100,
			Drops: []Drop{
				{Item: "minecraft:iron_ingot", Min: 3, Max: 5, Chance: 1.0},
				{Item: "minecraft:poppy", Min: 0, Max: 1, Chance: 1.0},
			},
		},
	}
}

// defFor returns the MobDef for a type, or the cow fallback.
func defFor(mobType string) MobDef {
	if d, ok := defs()[mobType]; ok {
		return d
	}
	return defs()["minecraft:cow"]
}

// DefFor is the exported form of defFor for use by packages that
// need to look up a mob's attributes from outside the mobs
// package (e.g. server.go's drop / split callbacks).
func DefFor(mobType string) MobDef { return defFor(mobType) }

// SpawnDefList returns a snapshot of every mob def that has a
// non-nil Spawn rule. Used by the natural-spawn director in
// internal/world to build the candidate pool.
func SpawnDefList() []MobDef {
	all := defs()
	out := make([]MobDef, 0, len(all))
	for _, d := range all {
		if d.Spawn != nil {
			out = append(out, d)
		}
	}
	return out
}

// XPRewardFor returns the vanilla 1.20.x experience points a mob drops on
// death. Returns 0 for mobs that never drop XP (iron_golem, villagers).
//
// Sources:
//   https://minecraft.fandom.com/wiki/Mob#Death  (hostile tables)
//   https://minecraft.fandom.com/wiki/Experience#Mob_experience
//
// The numbers below are the in-game yield (slime/magma_cube use 1 + size-1
// so a 4-size slime gives 4; the despawn path is free to roll a random
// amount in that band — v1 ships the low end of the range for those).
func XPRewardFor(mobType string) int {
	switch mobType {
	// Passives: 1-3 except sheep, cow, pig, chicken = 1-3 each
	case "minecraft:cow", "minecraft:pig", "minecraft:sheep", "minecraft:chicken":
		return 2 // 1-3, v1 midpoint
	// Hostile 5
	case "minecraft:zombie", "minecraft:skeleton", "minecraft:creeper",
		"minecraft:husk", "minecraft:zombie_villager", "minecraft:drowned",
		"minecraft:stray", "minecraft:bogged", "minecraft:cave_spider",
		"minecraft:spider", "minecraft:enderman", "minecraft:witch",
		"minecraft:piglin", "minecraft:phantom", "minecraft:wither_skeleton":
		return 5
	// Slime / magma cube: 1-4 (size 1)
	case "minecraft:slime", "minecraft:magma_cube":
		return 2 // 1-4, v1 midpoint
	// Blaze, ghast: 10
	case "minecraft:blaze", "minecraft:ghast":
		return 10
	// Iron golem: 0 (player-spawned or villager, never drops XP)
	case "minecraft:iron_golem":
		return 0
	}
	return 0
}
