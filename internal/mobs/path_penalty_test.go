package mobs

import "testing"

// allWalkable is a walkableQuery that treats every cell as open floor (the
// pathfinder's step model still keeps the path on one Y plane here).
func allWalkable(x, y, z int) bool { return true }

// TestPathFind_NilCostUniform confirms the back-compat behaviour: nil cost ==
// uniform-cost A* (straight line along one axis).
func TestPathFind_NilCostUniform(t *testing.T) {
	res := PathFind(PathNode{0, 64, 0}, PathNode{4, 64, 0}, allWalkable, nil)
	if !res.Found {
		t.Fatal("expected a path with nil cost")
	}
}

// TestPathFind_RoutesAroundMalusCorridor verifies a high-malus cell on the
// straight line is avoided in favour of a longer but cheaper detour.
func TestPathFind_RoutesAroundMalusCorridor(t *testing.T) {
	// Penalise the single cell (2,64,0) heavily; the optimal path should
	// step off the z=0 line to dodge it.
	cost := func(x, y, z int) float64 {
		if x == 2 && z == 0 {
			return 50
		}
		return 0
	}
	res := PathFind(PathNode{0, 64, 0}, PathNode{4, 64, 0}, allWalkable, cost)
	if !res.Found {
		t.Fatal("expected a path around the malus cell")
	}
	for _, n := range res.Nodes {
		if n.X == 2 && n.Z == 0 {
			t.Errorf("path should avoid the high-malus cell (2,64,0); nodes=%v", res.Nodes)
		}
	}
}

// TestPathFind_BlockedCellImpassable verifies MalusBlocked cells are never
// entered (the search must detour entirely).
func TestPathFind_BlockedCellImpassable(t *testing.T) {
	// Wall off z=0 at x=2 with an impassable cell; force a detour.
	cost := func(x, y, z int) float64 {
		if x == 2 && z == 0 {
			return MalusBlocked
		}
		return 0
	}
	res := PathFind(PathNode{0, 64, 0}, PathNode{4, 64, 0}, allWalkable, cost)
	if !res.Found {
		t.Fatal("expected a detour path; got none")
	}
	for _, n := range res.Nodes {
		if n.X == 2 && n.Z == 0 {
			t.Errorf("blocked cell entered; nodes=%v", res.Nodes)
		}
	}
}

// TestNavProfile_LandMobRefusesWater confirms a land mob's profile prices
// water as strongly avoided and a Strider treats lava as free.
func TestNavProfile_LandMobRefusesWater(t *testing.T) {
	land := navProfileFor(defFor("minecraft:zombie"))
	if land.malus[blkLava] < MalusBlocked {
		t.Errorf("land mob should treat lava as impassable, got %v", land.malus[blkLava])
	}
	if land.malus[blkWater] <= 0 {
		t.Errorf("land mob should avoid water (positive malus), got %v", land.malus[blkWater])
	}

	strider := navProfileFor(MobDef{Type: "minecraft:strider"})
	if strider.malus[blkLava] != 0 {
		t.Errorf("strider should walk lava freely (malus 0), got %v", strider.malus[blkLava])
	}
	if strider.malus[blkWater] < MalusBlocked {
		t.Errorf("strider should refuse water, got %v", strider.malus[blkWater])
	}
}

// TestNavProfile_CostBindsToProbe verifies the costFn reads the world probe and
// maps block ids to their malus.
func TestNavProfile_CostBindsToProbe(t *testing.T) {
	prof := navProfileFor(defFor("minecraft:zombie"))
	ctx := &AIContext{BlockNameAt: func(x, y, z int) string {
		if x == 1 {
			return blkLava
		}
		return "minecraft:air"
	}}
	cf := prof.cost(ctx)
	if cf == nil {
		t.Fatal("expected a non-nil cost function")
	}
	if cf(1, 64, 0) < MalusBlocked {
		t.Errorf("lava cell should be impassable via probe, got %v", cf(1, 64, 0))
	}
	if cf(0, 64, 0) != 0 {
		t.Errorf("air cell should be free, got %v", cf(0, 64, 0))
	}
}
