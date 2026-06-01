// Package registry tests: Java<->Bedrock id map round-trips and sentinel
// fallbacks. These are the canonical id maps the entire canonical core builds
// on, so the test is intentionally a true round-trip plus sentinel checks.
package registry

import "testing"

// TestBlockRoundTrip verifies that a Java->Bedrock->Java round-trip yields the
// same canonical state, and that an unregistered Bedrock id falls back to the
// Air sentinel without panicking.
func TestBlockRoundTrip(t *testing.T) {
	r := New()
	const javaStone BlockState = 1 // canonical: Java global state id
	const bedrockStone uint32 = 100
	r.RegisterBlock(javaStone, bedrockStone)

	if got, ok := r.BlockToBedrock(javaStone); !ok || got != bedrockStone {
		t.Fatalf("BlockToBedrock(stone): got (%d,%v), want (%d,true)", got, ok, bedrockStone)
	}
	if got, ok := r.BlockToJava(bedrockStone); !ok || got != javaStone {
		t.Fatalf("BlockToJava(bedrock stone): got (%d,%v), want (%d,true)", got, ok, javaStone)
	}
	// Sentinel: unknown Bedrock id must return AirState and ok=false (DESIGN A.4).
	if got, ok := r.BlockToJava(99999); ok || got != AirState {
		t.Fatalf("BlockToJava(unknown): got (%d,%v), want (AirState,false)", got, ok)
	}
}

// TestItemRoundTrip verifies item name<->runtime id round-trips and the
// UnknownItem sentinel for unmapped runtimes.
func TestItemRoundTrip(t *testing.T) {
	r := New()
	const name = "minecraft:diamond_sword"
	const runtime int32 = 765
	r.RegisterItem(name, runtime)

	if got, ok := r.ItemRuntime(name); !ok || got != runtime {
		t.Fatalf("ItemRuntime(diamond_sword): got (%d,%v), want (%d,true)", got, ok, runtime)
	}
	if got := r.ItemName(runtime); got != name {
		t.Fatalf("ItemName(%d): got %q, want %q", runtime, got, name)
	}
	if got := r.ItemName(99999); got != UnknownItem {
		t.Fatalf("ItemName(unknown): got %q, want %q", got, UnknownItem)
	}
}

// TestEntityNetID verifies that entity net ids are returned only for registered
// types; unknown types must return ok=false.
func TestEntityNetID(t *testing.T) {
	r := New()
	r.RegisterEntity("minecraft:zombie", 32)
	if got, ok := r.EntityNetID("minecraft:zombie"); !ok || got != 32 {
		t.Fatalf("EntityNetID(zombie): got (%d,%v), want (32,true)", got, ok)
	}
	if _, ok := r.EntityNetID("minecraft:doesnotexist"); ok {
		t.Fatalf("EntityNetID(unknown) returned ok=true, want false")
	}
}

// TestSentinelsAreStable guards the documented sentinel values. Bumping
// either is a public-API change; if a bump is intentional, update consumers
// in the same change set.
func TestSentinelsAreStable(t *testing.T) {
	if AirState != 0 {
		t.Fatalf("AirState changed: got %d, want 0", AirState)
	}
	if UnknownItem != "minecraft:air" {
		t.Fatalf("UnknownItem changed: got %q, want %q", UnknownItem, "minecraft:air")
	}
}
