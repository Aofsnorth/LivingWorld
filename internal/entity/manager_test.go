package entity

import (
	"testing"

	"livingworld/internal/registry"
)

func TestSpawnAssignsUniqueIDs(t *testing.T) {
	m := NewManager()
	a := m.Spawn("minecraft:pig", registry.Vec3{X: 1})
	b := m.Spawn("minecraft:cow", registry.Vec3{Y: 2})
	if a.ID == b.ID {
		t.Fatalf("ids not unique: %d == %d", a.ID, b.ID)
	}
	if a.UUID == b.UUID {
		t.Error("uuids not unique")
	}
	if m.Count() != 2 {
		t.Fatalf("count=%d want 2", m.Count())
	}
	if got, ok := m.Get(a.ID); !ok || got.Type != "minecraft:pig" {
		t.Fatalf("Get(a)=%v,%v want pig,true", got, ok)
	}
}

func TestDespawn(t *testing.T) {
	m := NewManager()
	e := m.Spawn("minecraft:zombie", registry.Vec3{})
	if !m.Despawn(e.ID) {
		t.Fatal("despawn existing should return true")
	}
	if m.Despawn(e.ID) {
		t.Error("despawn missing should return false")
	}
	if _, ok := m.Get(e.ID); ok {
		t.Error("entity still present after despawn")
	}
	if m.Count() != 0 {
		t.Fatalf("count=%d want 0", m.Count())
	}
}
