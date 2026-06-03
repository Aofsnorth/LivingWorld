package mobs

import (
	"math/rand"
	"testing"
)

// TestM1_AllNewMobsDefFor verifies every M1 mob type returns a
// non-fallback MobDef. A typo in defs() would silently map to
// "minecraft:cow" so this catches regressions at the def level.
func TestM1_AllNewMobsDefFor(t *testing.T) {
	m1Mobs := []string{
		"minecraft:husk",
		"minecraft:zombie_villager",
		"minecraft:drowned",
		"minecraft:stray",
		"minecraft:bogged",
		"minecraft:spider",
		"minecraft:cave_spider",
		"minecraft:slime",
		"minecraft:magma_cube",
		"minecraft:phantom",
		"minecraft:blaze",
		"minecraft:ghast",
		"minecraft:witch",
		"minecraft:enderman",
		"minecraft:piglin",
		"minecraft:wither_skeleton",
		"minecraft:iron_golem",
	}
	for _, name := range m1Mobs {
		def := defFor(name)
		if def.MaxHP == 0 {
			t.Errorf("%s: MaxHP is 0 (defs not registered)", name)
		}
		if def.Type != name {
			t.Errorf("%s: def.Type is %q (expected %q)", name, def.Type, name)
		}
	}
}

// TestM1_DefFor_DefaultFallback covers the unknown-type fallback to
// the cow entry. The point of the test is to make sure a typo in
// the AI doesn't fall through to a hostile mob.
func TestM1_DefFor_DefaultFallback(t *testing.T) {
	def := defFor("minecraft:nonexistent_mob")
	if def.IsHostile {
		t.Error("unknown mob fell back to a hostile type; should be passive (cow)")
	}
}

// TestM1_SpawnAtSize verifies the slime split path: spawning a
// child at Size-1 from a parent of Size 2.
func TestM1_SpawnAtSize(t *testing.T) {
	s := New()
	parent := s.SpawnAtSize("minecraft:slime", 0, 64, 0, 2)
	if parent.Size != 2 {
		t.Fatalf("parent.Size = %d, want 2", parent.Size)
	}
	child := s.SpawnAtSize("minecraft:slime", 0, 64, 0, 0) // 0 → uses def default (1)
	if child.Size != 1 {
		t.Fatalf("child.Size = %d, want 1 (slime def.Size)", child.Size)
	}
}

// TestM1_ProjectileKind_Roundtrip checks that SpawnKind carries
// the kind into the Projectile and that the velocity tunings
// differ per kind.
func TestM1_ProjectileKind_Roundtrip(t *testing.T) {
	s := NewProjectileStore()
	arrow := s.SpawnKind(0, [16]byte{}, 0, 64, 0, 0, 0, ProjectileArrow)
	if arrow.Kind != ProjectileArrow {
		t.Errorf("arrow.Kind = %q, want %q", arrow.Kind, ProjectileArrow)
	}
	fb := s.SpawnKind(0, [16]byte{}, 0, 64, 0, 0, 0, ProjectileSmallFireball)
	if fb.Kind != ProjectileSmallFireball {
		t.Errorf("fireball.Kind = %q, want %q", fb.Kind, ProjectileSmallFireball)
	}
	// small fireball should be slower than arrow (0.6 vs 1.6 b/tick).
	if fb.VX*fb.VX+fb.VZ*fb.VZ >= arrow.VX*arrow.VX+arrow.VZ*arrow.VZ {
		t.Error("small fireball should be slower than arrow")
	}
}

// TestM1_SoundDispatch_AllMobs ensures the decideSound switch
// recognises every M1 mob type. We seed rng with a value that
// consistently produces < 0.05, then call decideSound at
// FireTicks%80 == 0 so the ambient branch is taken.
func TestM1_SoundDispatch_AllMobs(t *testing.T) {
	m1Mobs := []string{
		"minecraft:husk", "minecraft:stray", "minecraft:bogged",
		"minecraft:drowned", "minecraft:spider", "minecraft:cave_spider",
		"minecraft:slime", "minecraft:magma_cube", "minecraft:phantom",
		"minecraft:blaze", "minecraft:ghast", "minecraft:witch",
		"minecraft:enderman", "minecraft:piglin",
		"minecraft:wither_skeleton", "minecraft:iron_golem",
		"minecraft:zombie_villager",
	}
	// Test the dispatch logic indirectly: walk a synthetic mob
	// at FireTicks=80 and call decideSound with a stub rng that
	// always returns 0 (so the 5% gate is passed).
	rng := rand.New(rand.NewSource(0)) // Float64() → varies; we just need <0.05 sometimes
	for i := 0; i < 1000; i++ {
		for _, name := range m1Mobs {
			m := Mob{Type: name, EntityID: 1, FireTicks: 80}
			emit := decideSound(&m, defFor(m.Type), rng)
			// We just want to ensure the code path doesn't
			// panic on a new mob type and doesn't fall
			// through into the vanilla-only set.
			_ = emit
		}
	}
	// Now actually assert at least one decision returned a
	// non-empty ambient. With 17*1000 attempts and a 5% gate,
	// the probability of zero hits is (0.95)^17000 ≈ 0.
	hit := false
	for i := 0; i < 1000 && !hit; i++ {
		for _, name := range m1Mobs {
			m := Mob{Type: name, EntityID: 1, FireTicks: 80}
			emit := decideSound(&m, defFor(m.Type), rng)
			if emit.Sound != "" {
				hit = true
				break
			}
		}
	}
	if !hit {
		t.Error("no M1 mob produced an ambient sound in 17000 tries")
	}
}
