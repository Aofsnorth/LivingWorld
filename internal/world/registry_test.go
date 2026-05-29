package world

import "testing"

func TestStateRegistryRoundTrip(t *testing.T) {
	cases := []string{
		"minecraft:stone",
		"minecraft:dirt",
		"minecraft:bedrock",
		"minecraft:grass_block",
		"minecraft:oak_planks",
	}
	for _, name := range cases {
		id := StateID(name)
		if id == AirID {
			t.Errorf("StateID(%q) resolved to air; block missing from palette", name)
			continue
		}
		if !ValidStateID(id) {
			t.Errorf("StateID(%q)=%d is out of palette range", name, id)
		}
		if got := StateName(id); got != name {
			t.Errorf("StateName(StateID(%q))=%q, want %q", name, got, name)
		}
	}
}

func TestAirIsZero(t *testing.T) {
	if StateID("minecraft:air") != AirID {
		t.Fatalf("air StateID = %d, want %d", StateID("minecraft:air"), AirID)
	}
	if !IsAir(AirID) {
		t.Fatalf("IsAir(AirID) = false")
	}
	if b := BlockByID(AirID); b.ID() != AirID {
		t.Fatalf("BlockByID(AirID).ID() = %d, want %d", b.ID(), AirID)
	}
	if _, ok := BlockByName("minecraft:stone").(StateBlock); !ok {
		t.Fatalf("BlockByName(stone) is not a StateBlock")
	}
}

func TestUnknownBlockResolvesToAir(t *testing.T) {
	if StateID("minecraft:does_not_exist_block") != AirID {
		t.Fatalf("unknown block did not resolve to air")
	}
}

func TestPaletteHasAllBlocks(t *testing.T) {
	// Sanity: the full 26.1 palette should be present (tens of thousands of states).
	if StateCount() < 20000 {
		t.Fatalf("palette only has %d states; expected the full vanilla set", StateCount())
	}
}
