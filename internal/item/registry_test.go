package item

import "testing"

func TestItemLookup(t *testing.T) {
	stone, ok := ByName("stone")
	if !ok {
		t.Fatal("stone not found by short name")
	}
	if stone.Name != "minecraft:stone" {
		t.Errorf("stone.Name = %q", stone.Name)
	}
	if _, ok := ByName("minecraft:stone"); !ok {
		t.Error("stone not found by namespaced name")
	}
	if byIDItem, ok := ByID(stone.ID); !ok || byIDItem.Name != stone.Name {
		t.Errorf("ByID round-trip failed for stone")
	}
}

func TestItemRegistryComplete(t *testing.T) {
	if Count() < 1000 {
		t.Fatalf("only %d items registered; expected the full 26.1 set", Count())
	}
}

func TestBlockItemLinkage(t *testing.T) {
	if id, ok := BlockStateID("minecraft:stone"); !ok || id == 0 {
		t.Errorf("stone should place a block, got id=%d ok=%v", id, ok)
	}
	if _, ok := BlockStateID("minecraft:diamond_sword"); ok {
		t.Errorf("a sword should not be a placeable block")
	}
	if _, ok := BlockStateID("minecraft:air"); ok {
		t.Errorf("air should not be reported placeable")
	}
}
