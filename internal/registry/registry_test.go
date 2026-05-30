package registry

import "testing"

func TestBlockRoundTrip(t *testing.T) {
	r := New()
	r.RegisterBlock(10, 200)
	if id, ok := r.BlockToBedrock(10); !ok || id != 200 {
		t.Fatalf("BlockToBedrock(10)=%d,%v want 200,true", id, ok)
	}
	if s, ok := r.BlockToJava(200); !ok || s != 10 {
		t.Fatalf("BlockToJava(200)=%d,%v want 10,true", s, ok)
	}
}

func TestItemRoundTrip(t *testing.T) {
	r := New()
	r.RegisterItem("minecraft:stone", 1)
	if id, ok := r.ItemRuntime("minecraft:stone"); !ok || id != 1 {
		t.Fatalf("ItemRuntime=%d,%v want 1,true", id, ok)
	}
	if n := r.ItemName(1); n != "minecraft:stone" {
		t.Fatalf("ItemName(1)=%q want minecraft:stone", n)
	}
}

func TestUnmappedSentinels(t *testing.T) {
	r := New()
	if _, ok := r.BlockToBedrock(999); ok {
		t.Error("unmapped block should report ok=false")
	}
	if s, ok := r.BlockToJava(999); ok || s != AirState {
		t.Errorf("unmapped bedrock->java = %d,%v want air,false", s, ok)
	}
	if n := r.ItemName(12345); n != UnknownItem {
		t.Errorf("unknown item name = %q want %q", n, UnknownItem)
	}
}
