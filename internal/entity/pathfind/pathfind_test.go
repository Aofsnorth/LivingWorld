package pathfind

import (
	"testing"

	"livingworld/internal/registry"
)

// blockedNav: everything walkable except the listed positions.
type blockedNav map[registry.Pos]bool

func (b blockedNav) Walkable(p registry.Pos) bool { return !b[p] }

func TestStraightLine(t *testing.T) {
	path := Find(blockedNav{}, registry.Pos{}, registry.Pos{X: 3}, 1000)
	if len(path) != 4 || path[0] != (registry.Pos{}) || path[3] != (registry.Pos{X: 3}) {
		t.Fatalf("path=%v want 4 nodes 0..3 on X", path)
	}
}

func TestDetourAroundWall(t *testing.T) {
	// Wall at (1,0,0) forces a one-block detour: 4 moves => 5 nodes.
	nav := blockedNav{{X: 1}: true}
	path := Find(nav, registry.Pos{}, registry.Pos{X: 2}, 1000)
	if path == nil {
		t.Fatal("expected a path around the wall")
	}
	if len(path) != 5 {
		t.Fatalf("len=%d want 5: %v", len(path), path)
	}
	for _, p := range path {
		if p == (registry.Pos{X: 1}) {
			t.Fatalf("path goes through the wall: %v", path)
		}
	}
}

func TestUnreachableCagedStart(t *testing.T) {
	// All six neighbours of the start are blocked.
	nav := blockedNav{
		{X: 1}: true, {X: -1}: true,
		{Y: 1}: true, {Y: -1}: true,
		{Z: 1}: true, {Z: -1}: true,
	}
	if path := Find(nav, registry.Pos{}, registry.Pos{X: 5}, 1000); path != nil {
		t.Fatalf("expected nil for caged start, got %v", path)
	}
}

func TestBudgetExhausted(t *testing.T) {
	if path := Find(blockedNav{}, registry.Pos{}, registry.Pos{X: 100}, 5); path != nil {
		t.Fatalf("expected nil under tight budget, got len %d", len(path))
	}
}
