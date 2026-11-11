// Package bedrocktests holds regression tests for the Bedrock chunk/palette
// pipeline. They pin the invariants that produced the "corrupted terrain"
// report: every worldgen block must resolve to the CORRECT Bedrock runtime ID
// (not merely non-zero — runtime ID 0 is a real block in the FNV-sorted
// registry), and encoded sub-chunks must round-trip through decode unchanged.
package bedrocktests

import (
	"os"
	"testing"

	bedrockworld "livingworld/internal/bedrock/world"

	dfworld "github.com/df-mc/dragonfly/server/world"
)

// dragonfly v0.11.5 registry facts, verified empirically and pinned here so a
// dependency bump that changes them fails loudly instead of silently
// corrupting terrain. Note rid 0 = minecraft:cyan_terracotta: any test that
// treats 0 as air/failure is wrong by construction.
const (
	dragonflyVanillaItemsSHA256 = "fee2d053125407a228731226e9025598d4d9facf19da1f0ad6aa7349d8389633"
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

// TestVanillaBlockEntriesMatchesDragonflyDataDrivenBlocks pins that
// VanillaBlockEntries() exposes all vanilla data-driven blocks from
// dragonfly's server/world package and that internal/bedrock/server/handler.go
// wires CustomBlocks into StartGame's GameData. Without this, Bedrock 1.26.50 /
// 26.52 clients omit ~3,931 data-driven block states from their runtime-ID
// palette and shift every terrain block into doors/stairs.
func TestVanillaBlockEntriesMatchesDragonflyDataDrivenBlocks(t *testing.T) {
	want := dfworld.DataDrivenBlocks()
	if len(want) == 0 {
		t.Fatal("dfworld.DataDrivenBlocks() returned 0 entries; expected vanilla data-driven blocks in dragonfly v0.11.5")
	}
	got := bedrockworld.VanillaBlockEntries()
	if len(got) != len(want) {
		t.Fatalf("VanillaBlockEntries() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i].Name {
			t.Fatalf("entry[%d].Name = %q, want %q", i, got[i].Name, want[i].Name)
		}
		if got[i].Properties == nil {
			t.Fatalf("entry[%d] (%s) has nil Properties", i, got[i].Name)
		}
	}

	handlerSrc, err := os.ReadFile("../../internal/bedrock/server/handler.go")
	if err != nil {
		t.Fatalf("read handler.go: %v", err)
	}
	if !containsBytes(handlerSrc, "CustomBlocks:") || !containsBytes(handlerSrc, "bedrockworld.VanillaBlockEntries()") {
		t.Fatal("internal/bedrock/server/handler.go must populate GameData.CustomBlocks with bedrockworld.VanillaBlockEntries()")
	}
}

func containsBytes(haystack []byte, needle string) bool {
	nb := []byte(needle)
	if len(nb) == 0 {
		return true
	}
	for i := 0; i+len(nb) <= len(haystack); i++ {
		match := true
		for j := range nb {
			if haystack[i+j] != nb[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
