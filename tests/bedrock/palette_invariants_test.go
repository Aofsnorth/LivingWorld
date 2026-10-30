// Package bedrocktests holds regression tests for the Bedrock chunk/palette
// pipeline. They pin the invariants that produced the "corrupted terrain"
// report: every worldgen block must resolve to the CORRECT Bedrock runtime ID
// (not merely non-zero — runtime ID 0 is a real block in the FNV-sorted
// registry), and encoded sub-chunks must round-trip through decode unchanged.
package bedrocktests

import (
	"testing"

	bedrockworld "livingworld/internal/bedrock/world"
)

// dragonfly v0.10.14 registry facts, verified empirically and pinned here so a
// dependency bump that changes them fails loudly instead of silently
// corrupting terrain. Note rid 0 = minecraft:cyan_terracotta: any test that
// treats 0 as air/failure is wrong by construction.
const (
	dragonflyVanillaItemsSHA256 = "0ad3c87f3cc76ad97d4742a24d77ceaf1f8652ed35db2b50937c2f95ff924dec"
)

// TestRIDZeroIsCyanTerracotta pins the FNV-sorted registry property that makes
// "rid == 0 means broken/air" reasoning invalid. If dragonfly's registry
// ordering changes, this test documents it so call sites can be re-audited.
func TestRIDZeroIsCyanTerracotta(t *testing.T) {
	name, _, ok := dfworldRuntimeIDToState(0)
	if !ok {
		t.Fatal("runtime ID 0 not resolvable; dragonfly registry layout changed")
	}
	if name != "minecraft:cyan_terracotta" {
		t.Fatalf("runtime ID 0 is now %s (was minecraft:cyan_terracotta); re-audit all rid==0 sentinels", name)
	}
}

// TestAirRIDIsNotZero pins that air does NOT live at runtime ID 0 — the
// historical LogBlockPaletteVersion predicate was dead code because of this.
func TestAirRIDIsNotZero(t *testing.T) {
	if !bedrockworld.BlockRIDFound("minecraft:air") {
		t.Fatal("minecraft:air missing from Bedrock registry")
	}
	airRID := bedrockworld.BlockRID("minecraft:air")
	if airRID == 0 {
		t.Fatal("air resolved to runtime ID 0; registry ordering assumption broken")
	}
}
