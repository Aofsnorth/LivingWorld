package world

import (
	"testing"

	"livingworld/internal/mobs"
)

// TestSpawnMode_DefaultsToJava verifies an unset/unknown mode reports "java".
func TestSpawnMode_DefaultsToJava(t *testing.T) {
	m := NewManager()
	if got := m.SpawnMode(); got != spawnModeJava {
		t.Errorf("default SpawnMode = %q, want %q", got, spawnModeJava)
	}
	m.SetSpawnMode("nonsense")
	if got := m.SpawnMode(); got != spawnModeJava {
		t.Errorf("unknown SpawnMode = %q, want fallback %q", got, spawnModeJava)
	}
	m.SetSpawnMode(spawnModeBedrock)
	if got := m.SpawnMode(); got != spawnModeBedrock {
		t.Errorf("SpawnMode = %q, want %q", got, spawnModeBedrock)
	}
}

// TestSpawnMode_BedrockBlockLightSuppresses verifies the BE hostile-light rule:
// a column lit only by block light (sky=0, block=5) rejects a hostile in
// Bedrock mode but is accepted in Java mode (max(block,sky)=5 ≤ 7).
func TestSpawnMode_BedrockBlockLightSuppresses(t *testing.T) {
	w := NewWorld("test")
	for x := -2; x <= 2; x++ {
		for z := -2; z <= 2; z++ {
			w.SetBlock(x, 64, z, BlockByID(StateID("minecraft:grass_block")))
		}
	}
	// Torch-lit cave: sky dark, block light 5.
	w.SetSkyLight(0, 65, 0, 0)
	w.SetBlockLight(0, 65, 0, 5)
	rule := mobs.DefFor("minecraft:zombie").Spawn

	if !evaluateSpawnRuleMode(w, 0, 65, 0, rule, 14000, spawnModeJava) {
		t.Error("Java mode: zombie should spawn under block light 5 (internal light ≤ 7)")
	}
	if evaluateSpawnRuleMode(w, 0, 65, 0, rule, 14000, spawnModeBedrock) {
		t.Error("Bedrock mode: zombie must NOT spawn under any block light")
	}
}

// TestSpawnMode_BedrockDarkAllows verifies the BE rule still allows a fully
// dark column (sky=0, block=0).
func TestSpawnMode_BedrockDarkAllows(t *testing.T) {
	w := NewWorld("test")
	for x := -2; x <= 2; x++ {
		for z := -2; z <= 2; z++ {
			w.SetBlock(x, 64, z, BlockByID(StateID("minecraft:grass_block")))
		}
	}
	w.SetSkyLight(0, 65, 0, 0)
	w.SetBlockLight(0, 65, 0, 0)
	rule := mobs.DefFor("minecraft:zombie").Spawn
	if !evaluateSpawnRuleMode(w, 0, 65, 0, rule, 14000, spawnModeBedrock) {
		t.Error("Bedrock mode: zombie should spawn in full darkness")
	}
}

// TestSpawnMode_EffectiveCaps verifies BE uses the static 200 cap per category
// while JE scales from the loaded-chunk count (floored at the base).
func TestSpawnMode_EffectiveCaps(t *testing.T) {
	m := NewManager()
	w := NewWorld("test")
	// Touch a block so at least one chunk is loaded.
	w.SetBlock(0, 64, 0, BlockByID(StateID("minecraft:grass_block")))

	be := m.effectiveCaps(w, spawnModeBedrock)
	if be[mobs.SpawnHostile] != beStaticCap {
		t.Errorf("BE hostile cap = %d, want %d", be[mobs.SpawnHostile], beStaticCap)
	}

	je := m.effectiveCaps(w, spawnModeJava)
	if je[mobs.SpawnHostile] < capHostile {
		t.Errorf("JE hostile cap = %d, want >= base %d (floored)", je[mobs.SpawnHostile], capHostile)
	}
	if je[mobs.SpawnHostile] == beStaticCap {
		t.Errorf("JE hostile cap should not equal the BE static cap")
	}
}
