package world

import (
	"math/rand"
	"testing"

	"livingworld/internal/mobs"
)

// TestM2_SpawnDefList_HasAllExpectedMobs ensures the spawn
// director sees every M1 mob that should be naturally
// spawnable. A typo in defs() would silently miss the
// candidate pool and the mob would never appear in the world.
func TestM2_SpawnDefList_HasAllExpectedMobs(t *testing.T) {
	want := []string{
		"minecraft:cow", "minecraft:pig", "minecraft:sheep", "minecraft:chicken", // passives
		"minecraft:zombie", "minecraft:skeleton", "minecraft:creeper", // overworld hostiles
		"minecraft:spider", "minecraft:cave_spider", // spiders
		"minecraft:husk", "minecraft:zombie_villager", "minecraft:drowned", // zombie variants
		"minecraft:stray", "minecraft:bogged", // skeleton variants
		"minecraft:wither_skeleton", // nether
		"minecraft:slime", "minecraft:magma_cube", // slimes
		"minecraft:phantom", "minecraft:witch", "minecraft:enderman", // overworld extras
		"minecraft:blaze", "minecraft:ghast", "minecraft:piglin", // nether
	}
	have := map[string]bool{}
	for _, d := range mobs.SpawnDefList() {
		have[d.Type] = true
	}
	for _, name := range want {
		if !have[name] {
			t.Errorf("spawn director missing %s (no Spawn rule on def)", name)
		}
	}
}

// TestM2_SpawnDefList_NoIronGolem ensures the director does
// NOT include iron_golem. v1: iron golems are only spawned by
// villagers (structures), not by the director.
func TestM2_SpawnDefList_NoIronGolem(t *testing.T) {
	for _, d := range mobs.SpawnDefList() {
		if d.Type == "minecraft:iron_golem" {
			t.Error("iron_golem must NOT be in the spawn pool (v1: villager-only)")
		}
	}
}

// TestM2_Evaluate_LightHostileOnly checks that a zombie
// (MaxLight=7) does not match a column with light 8, and does
// match a column with light 7.
func TestM2_Evaluate_LightHostileOnly(t *testing.T) {
	w := NewWorld("test")
	// Build a chunk at (0,0) and a 1x1 floor at y=64.
	for x := -2; x <= 2; x++ {
		for z := -2; z <= 2; z++ {
			w.SetBlock(x, 64, z, BlockByID(StateID("minecraft:grass_block")))
		}
	}
	// Bright column at (0, 65, 0): light=15 → no zombie.
	w.SetSkyLight(0, 65, 0, 15)
	rule := mobs.DefFor("minecraft:zombie").Spawn
	if rule == nil {
		t.Fatal("zombie has no Spawn rule")
	}
	if evaluateSpawnRule(w, 0, 65, 0, rule, 14000) {
		t.Error("zombie matched at light=15 (should be MaxLight=7)")
	}
	// Dark column at (0, 65, 0): light=0 → zombie matches.
	w.SetSkyLight(0, 65, 0, 0)
	if !evaluateSpawnRule(w, 0, 65, 0, rule, 14000) {
		t.Error("zombie did not match at light=0")
	}
}

// TestM2_Evaluate_DayPassiveOnly checks that a cow (DayOnly,
// MinLight=9) does not match at night and matches during the
// day at light=15.
func TestM2_Evaluate_DayPassiveOnly(t *testing.T) {
	w := NewWorld("test")
	for x := -2; x <= 2; x++ {
		for z := -2; z <= 2; z++ {
			w.SetBlock(x, 64, z, BlockByID(StateID("minecraft:grass_block")))
		}
	}
	w.SetSkyLight(0, 65, 0, 15)
	rule := mobs.DefFor("minecraft:cow").Spawn
	if rule == nil {
		t.Fatal("cow has no Spawn rule")
	}
	// Night dayTime (14000 = 7 ticks after dusk)
	if evaluateSpawnRule(w, 0, 65, 0, rule, 14000) {
		t.Error("cow matched at night (should be DayOnly)")
	}
	// Day dayTime (6000 = midday)
	if !evaluateSpawnRule(w, 0, 65, 0, rule, 6000) {
		t.Error("cow did not match at midday+light=15")
	}
}

// TestM2_Evaluate_SurfaceWhitelist checks that a husk (sand
// only) does not match on grass but matches on sand.
func TestM2_Evaluate_SurfaceWhitelist(t *testing.T) {
	w := NewWorld("test")
	for x := -4; x <= 4; x++ {
		for z := -4; z <= 4; z++ {
			w.SetBlock(x, 64, z, BlockByID(StateID("minecraft:grass_block")))
		}
	}
	w.SetBlock(2, 64, 2, BlockByID(StateID("minecraft:sand")))
	w.SetBlock(3, 64, 3, BlockByID(StateID("minecraft:sand")))
	w.SetSkyLight(2, 65, 2, 0)
	w.SetSkyLight(3, 65, 3, 0)
	rule := mobs.DefFor("minecraft:husk").Spawn
	if rule == nil {
		t.Fatal("husk has no Spawn rule")
	}
	// Grass at (0, 64, 0) — should NOT match (husk wants sand).
	if evaluateSpawnRule(w, 0, 65, 0, rule, 14000) {
		t.Error("husk matched on grass_block (should only match sand)")
	}
	// Sand at (2, 64, 2) — should match.
	if !evaluateSpawnRule(w, 2, 65, 2, rule, 14000) {
		t.Error("husk did not match on sand")
	}
}

// TestM2_Evaluate_DimensionFilter checks that a cow (overworld
// only) does not match in the nether, and a piglin (nether
// only) does not match in the overworld.
func TestM2_Evaluate_DimensionFilter(t *testing.T) {
	w := NewWorld("test")
	for x := -2; x <= 2; x++ {
		for z := -2; z <= 2; z++ {
			w.SetBlock(x, 64, z, BlockByID(StateID("minecraft:grass_block")))
		}
	}
	w.SetSkyLight(0, 65, 0, 15)
	cow := mobs.DefFor("minecraft:cow").Spawn
	piglin := mobs.DefFor("minecraft:piglin").Spawn
	if cow == nil || piglin == nil {
		t.Fatal("missing spawn rule")
	}
	// Overworld (default): cow matches, piglin does not.
	if !evaluateSpawnRule(w, 0, 65, 0, cow, 6000) {
		t.Error("cow did not match in overworld")
	}
	if evaluateSpawnRule(w, 0, 65, 0, piglin, 14000) {
		t.Error("piglin matched in overworld (should be nether-only)")
	}
	// Switch to nether dimension: cow fails, piglin matches.
	w.dimension = DimensionNether
	// Piglin needs a nether block at the feet.
	w.SetBlock(0, 64, 0, BlockByID(StateID("minecraft:netherrack")))
	if evaluateSpawnRule(w, 0, 65, 0, cow, 6000) {
		t.Error("cow matched in nether (should be overworld-only)")
	}
	if !evaluateSpawnRule(w, 0, 65, 0, piglin, 14000) {
		t.Error("piglin did not match in nether on netherrack")
	}
}

// TestM2_Evaluate_RequireDark checks the cave_spider rule
// (sky-light=0). A column with sky-light=5 should NOT match
// even with block-light=0.
func TestM2_Evaluate_RequireDark(t *testing.T) {
	w := NewWorld("test")
	for x := -2; x <= 2; x++ {
		for z := -2; z <= 2; z++ {
			w.SetBlock(x, 64, z, BlockByID(StateID("minecraft:stone")))
		}
	}
	rule := mobs.DefFor("minecraft:cave_spider").Spawn
	if rule == nil {
		t.Fatal("cave_spider has no Spawn rule")
	}
	// Sky=5, block=0 → sky=5 ≠ 0, fail.
	w.SetSkyLight(0, 65, 0, 5)
	if evaluateSpawnRule(w, 0, 65, 0, rule, 14000) {
		t.Error("cave_spider matched with sky=5 (should require sky=0)")
	}
	// Sky=0, block=0 → match.
	w.SetSkyLight(0, 65, 0, 0)
	if !evaluateSpawnRule(w, 0, 65, 0, rule, 14000) {
		t.Error("cave_spider did not match with sky=0")
	}
}

// TestM2_Evaluate_RequireSkyLight15 checks the phantom rule
// (sky-light=15). Phantom needs open night sky, so a column
// under any solid block at y+1 fails.
func TestM2_Evaluate_RequireSkyLight15(t *testing.T) {
	w := NewWorld("test")
	for x := -2; x <= 2; x++ {
		for z := -2; z <= 2; z++ {
			w.SetBlock(x, 64, z, BlockByID(StateID("minecraft:grass_block")))
		}
	}
	// Phantom does not constrain the surface, only the sky.
	rule := mobs.DefFor("minecraft:phantom").Spawn
	if rule == nil {
		t.Fatal("phantom has no Spawn rule")
	}
	// Sky=10, block=0 → max(10,0)=10 ≠ 15, fail.
	w.SetSkyLight(0, 65, 0, 10)
	if evaluateSpawnRule(w, 0, 65, 0, rule, 14000) {
		t.Error("phantom matched with sky=10 (should require sky=15)")
	}
	// Sky=15 → match.
	w.SetSkyLight(0, 65, 0, 15)
	if !evaluateSpawnRule(w, 0, 65, 0, rule, 14000) {
		t.Error("phantom did not match with sky=15")
	}
}

// TestM2_Evaluate_RequireOpenSky checks the ghast rule
// (8+ open-air cells above the candidate). A column with a
// ceiling at y+2 should NOT match.
func TestM2_Evaluate_RequireOpenSky(t *testing.T) {
	w := NewWorld("test")
	// Floor at y=64 in nether
	for x := -2; x <= 2; x++ {
		for z := -2; z <= 2; z++ {
			w.SetBlock(x, 64, z, BlockByID(StateID("minecraft:netherrack")))
		}
	}
	w.dimension = DimensionNether
	rule := mobs.DefFor("minecraft:ghast").Spawn
	if rule == nil {
		t.Fatal("ghast has no Spawn rule")
	}
	// No ceiling → match.
	if !evaluateSpawnRule(w, 0, 65, 0, rule, 14000) {
		t.Error("ghast did not match in open air (nether)")
	}
	// Solid block at y=68 (8 cells up) → block RequireOpenSky.
	w.SetBlock(0, 68, 0, BlockByID(StateID("minecraft:netherrack")))
	if evaluateSpawnRule(w, 0, 65, 0, rule, 14000) {
		t.Error("ghast matched under a ceiling (should require open sky)")
	}
}

// TestM2_DifficultyPeaceful_SuppressesHostile verifies that
// difficultyAllowsCategory blocks all hostiles when the
// difficulty is "peaceful".
func TestM2_DifficultyPeaceful_SuppressesHostile(t *testing.T) {
	if difficultyAllowsCategory("peaceful", mobs.SpawnHostile) {
		t.Error("peaceful should suppress hostile spawns")
	}
	if !difficultyAllowsCategory("peaceful", mobs.SpawnPassive) {
		t.Error("peaceful should still allow passives")
	}
	if !difficultyAllowsCategory("peaceful", mobs.SpawnNeutral) {
		t.Error("peaceful should still allow neutrals")
	}
	if !difficultyAllowsCategory("normal", mobs.SpawnHostile) {
		t.Error("normal should allow hostiles")
	}
	if !difficultyAllowsCategory("hard", mobs.SpawnHostile) {
		t.Error("hard should allow hostiles")
	}
}

// TestM2_SpawnTick_NoAnchors does nothing (no players online).
// A misbehaving director would still try to spawn; the empty
// anchor list must abort.
func TestM2_SpawnTick_NoAnchors(t *testing.T) {
	mgr := NewManager()
	// No player locator → empty anchors.
	rng := rand.New(rand.NewSource(1))
	// Should be a no-op (no panic, no spawn).
	mgr.spawnTick(rng)
	if n := len(mgr.Mobs().All()); n != 0 {
		t.Errorf("expected 0 mobs with no anchors, got %d", n)
	}
}

// TestM2_SpawnTick_BuildsPlayerColumn uses the player locator
// to seed a column and verifies the director spawns at least
// one mob on a successful rule match. The exact mob is
// non-deterministic (weighted pick) but the count must
// increase.
func TestM2_SpawnTick_BuildsPlayerColumn(t *testing.T) {
	mgr := NewManager()
	w := mgr.GetDefaultWorld()
	// Build a 64x64 grass arena with daylight and 2 air cells
	// above. Center on (0, 64, 0).
	for x := -32; x <= 32; x++ {
		for z := -32; z <= 32; z++ {
			w.SetBlock(x, 64, z, BlockByID(StateID("minecraft:grass_block")))
			w.SetSkyLight(x, 65, z, 15)
		}
	}
	// Pin the spawn-radius window inside the arena.
	mgr.SetPlayerLocator(func() []Position {
		return []Position{{X: 0, Y: 65, Z: 0}}
	})
	// Day → passive spawns (cow/pig/sheep/chicken).
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 50; i++ {
		mgr.spawnTick(rng)
	}
	all := mgr.Mobs().All()
	if len(all) == 0 {
		t.Errorf("expected spawns after 50 ticks on grass+day, got 0")
	}
	// Every spawn should be a passive.
	for _, m := range all {
		def := mobs.DefFor(m.Type)
		if def.Spawn == nil || def.Spawn.Category != mobs.SpawnPassive {
			t.Errorf("day spawn produced non-passive %s", m.Type)
		}
	}
}
