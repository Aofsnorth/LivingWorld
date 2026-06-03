package drops

import "testing"

// TestM7_SpawnXP_AddsOrbToStore asserts SpawnXP inserts an
// XPOrb with the right fields (Experience, position) and
// fires the OnOrbSpawn callback exactly once.
func TestM7_SpawnXP_AddsOrbToStore(t *testing.T) {
	s := New()
	spawned := 0
	var gotOrb XPOrb
	s.OnOrbSpawn(func(o XPOrb) {
		spawned++
		gotOrb = o
	})
	o := s.SpawnXP(5, 10, 65, -3)
	if o.Experience != 5 {
		t.Errorf("orb Experience = %d, want 5", o.Experience)
	}
	if o.X != 10 || o.Y != 65 || o.Z != -3 {
		t.Errorf("orb position = (%v, %v, %v), want (10, 65, -3)", o.X, o.Y, o.Z)
	}
	if o.EntityID == 0 {
		t.Error("orb has zero entity id")
	}
	if spawned != 1 {
		t.Errorf("OnOrbSpawn fired %d times, want 1", spawned)
	}
	if gotOrb.EntityID != o.EntityID {
		t.Errorf("OnOrbSpawn got different orb than SpawnXP returned")
	}
}

// TestM7_SpawnXP_ZeroAmountStillSpawns asserts a 0-amount
// orb is still added to the store (a quirk of vanilla where
// some 0-XP sources like command-block give still create a
// visible orb). The bridge can decide to filter at render
// time; the store is faithful.
func TestM7_SpawnXP_ZeroAmountStillSpawns(t *testing.T) {
	s := New()
	s.SpawnXP(0, 0, 64, 0)
	if n := len(s.Orbs()); n != 1 {
		t.Errorf("orbs after 0-XP SpawnXP = %d, want 1", n)
	}
}

// TestM7_RemoveOrb_DeletesAndFires asserts RemoveOrb clears
// the orb from the map and fires the OnOrbDespawn callback
// once. Returns false on a second call (idempotent).
func TestM7_RemoveOrb_DeletesAndFires(t *testing.T) {
	s := New()
	despawned := 0
	s.OnOrbDespawn(func(id int64) { despawned++ })
	o := s.SpawnXP(5, 0, 64, 0)
	if !s.RemoveOrb(o.EntityID) {
		t.Error("first RemoveOrb returned false, want true")
	}
	if s.RemoveOrb(o.EntityID) {
		t.Error("second RemoveOrb returned true, want false (already gone)")
	}
	if despawned != 1 {
		t.Errorf("OnOrbDespawn fired %d times, want 1", despawned)
	}
	if n := len(s.Orbs()); n != 0 {
		t.Errorf("Orbs() after remove = %d, want 0", n)
	}
}

// TestM7_Orbs_SnapshotIsCopy asserts Orbs returns a copy so
// the caller can iterate without holding the read lock and
// without aliasing the store's pointers.
func TestM7_Orbs_SnapshotIsCopy(t *testing.T) {
	s := New()
	s.SpawnXP(5, 1, 64, 1)
	s.SpawnXP(2, 2, 64, 2)
	all := s.Orbs()
	if len(all) != 2 {
		t.Errorf("orbs snapshot = %d, want 2", len(all))
	}
	// Mutate the snapshot; the store must not see it.
	all[0].X = 999
	again := s.Orbs()
	if again[0].X == 999 {
		t.Error("Orbs() returned aliased slice")
	}
}
