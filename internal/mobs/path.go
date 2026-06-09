// A* pathfinding for mobs.
//
// Why a custom A* and not `internal/entity/pathfind/pathfind.go`? That one
// targets player navigation (single-mob movement with full physics) and
// imports `internal/entity`. The mob pathfinder only needs to find a
// series of integer cells (Node) from `from` to `to`; it does not
// integrate physics, just emits waypoints that `aiStep` walks toward.
// Decoupling keeps the `mobs` package free of `entity` (already true) and
// gives us a much smaller API.
//
// Search parameters:
//   - Manhattan heuristic, weighted (typical w=1.0; tweakable per mob).
//   - 6-way (X/Y/Z neighbors); Y is split into [step-up: 1 block,
//     step-down: 4 blocks]. This matches the `follow_range` walk envelope.
//   - MaxExpand = 1024. A* on a 1024-node budget explores ~1024 cells
//     before giving up. For the typical chase distance (< 35 blocks), the
//     search finishes in well under 1024 expansions.
//
// Walkability: a cell is walkable if `walkableAt(x,y,z)` returns true
// (default: air at the feet AND the cell at y+1 is also walkable AND
// the cell at y-1 is solid). Ladders, fences, water, and open doors
// would be added by extending walkableAt.
package mobs

import (
	"math"
	"sort"
)

// PathNode is one cell on a found path. y is the *feet* coordinate
// (vanilla's block-y, not the mob's centered position).
type PathNode struct {
	X, Y, Z int
}

// Path is an ordered list of waypoints from start to end. The mob walks
// from Path[i] toward Path[i+1] in straight line, jumping if the next
// node's Y is higher. The last node is the goal cell.
type Path []PathNode

// PathFindResult bundles a path with diagnostics. `Found` is false when
// the search budget was exhausted.
type PathFindResult struct {
	Found     bool
	Nodes     Path
	Expansions int
	BudgetHit bool
}

const (
	// MaxExpand is the A* node-expansion cap. 1024 is generous for
	// ~35-block chases; raise if longer chases are observed
	// timing out.
	MaxExpand = 1024

	// maxStepUp is the maximum vertical climb between consecutive
	// path nodes. Vanilla mobs (without jump boost) clear a 1-block
	// step; spiders, horses, and rabbits clear more but those are
	// mob-specific overrides (M1).
	maxStepUp = 1

	// maxStepDown is the maximum drop without intermediate nodes.
	// Vanilla mobs that aren't parkour-capable fall ~3 blocks
	// before re-planning. We allow 4 to give a little slack.
	maxStepDown = 4
)

// walkableQuery checks the three cells that decide whether a mob can
// occupy (x, y, z) as feet. Built once per A* call so the closure
// captures a single context.
type walkableQuery func(x, y, z int) bool

// costFn returns the per-cell movement malus (penalty) for entering the cell
// at (x, y, z) — vanilla's PathfindingMalus model. 0 is the neutral cost; a
// positive value makes the cell expensive but passable (Blaze over lava = 8);
// MalusBlocked makes it impassable (water for a land mob). May be nil, in
// which case every walkable cell costs the base step only.
type costFn func(x, y, z int) float64

// MalusBlocked is the sentinel cost meaning "do not path through this cell".
// Anything ≥ MalusBlocked is treated as impassable by PathFind.
const MalusBlocked = 1e6

// PathFind returns the shortest A* path from `from` to `to` (integer
// cell coordinates) under the walkable predicate, or a not-Found result
// if the budget was exhausted.
//
// The heuristic is Manhattan distance in cells. The cost is 1 per cell
// traversed (uniform — no diagonal cost or surface slope for v1).
//
// `walkable` is the canonical "is the cell at (x,y,z) walkable for this
// mob" predicate. `cost` adds the per-cell movement malus (nil = uniform):
// a cell whose cost ≥ MalusBlocked is treated as impassable even if walkable,
// and a positive cost makes the cell expensive but usable (Strider lava = 0,
// Blaze lava = 8, water = blocked for land mobs).
func PathFind(from, to PathNode, walkable walkableQuery, cost costFn) PathFindResult {
	if from == to {
		return PathFindResult{Found: true, Nodes: Path{from}, Expansions: 0}
	}

	type aNode struct {
		pos  PathNode
		g    float64
		f    float64
		open bool
		prev *aNode
	}

	// Index the open/closed sets by PathNode directly. PathNode is a
	// comparable struct (no slices/maps), so Go's built-in map handles
	// it without a custom hash.

	all := make(map[PathNode]*aNode, 256)
	get := func(p PathNode) *aNode {
		if n, ok := all[p]; ok {
			return n
		}
		n := &aNode{pos: p, g: math.Inf(1), f: math.Inf(1)}
		all[p] = n
		return n
	}

	start := get(from)
	goal := get(to)
	start.g = 0
	start.f = manhattan(start.pos, goal.pos)
	start.open = true
	openList := []*aNode{start}

	neighbors := func(p PathNode) []PathNode {
		// 4 horizontal neighbors at the same Y, plus optional
		// step-up / step-down neighbors.
		out := make([]PathNode, 0, 4)
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, nz := p.X+d[0], p.Z+d[1]
			// 1) same level
			if walkable(nx, p.Y, nz) {
				out = append(out, PathNode{X: nx, Y: p.Y, Z: nz})
				continue
			}
			// 2) step up maxStepUp blocks
			climbed := false
			for up := 1; up <= maxStepUp; up++ {
				if walkable(nx, p.Y+up, nz) && !walkable(nx, p.Y+up+1, nz) {
					out = append(out, PathNode{X: nx, Y: p.Y + up, Z: nz})
					climbed = true
					break
				}
			}
			if climbed {
				continue
			}
			// 3) step down up to maxStepDown blocks
			for dn := 1; dn <= maxStepDown; dn++ {
				if walkable(nx, p.Y-dn, nz) && !walkable(nx, p.Y-dn-1, nz) {
					out = append(out, PathNode{X: nx, Y: p.Y - dn, Z: nz})
					break
				}
			}
		}
		return out
	}

	expansions := 0
	for len(openList) > 0 {
		// Pop the lowest-f node.
		sort.Slice(openList, func(i, j int) bool {
			if openList[i].f != openList[j].f {
				return openList[i].f < openList[j].f
			}
			return openList[i].g > openList[j].g // tie-break: prefer deeper (closer to goal)
		})
		cur := openList[0]
		openList = openList[1:]
		cur.open = false
		expansions++

		if cur.pos == goal.pos {
			// Reconstruct the path.
			nodes := make(Path, 0, 16)
			for n := cur; n != nil; n = n.prev {
				nodes = append(nodes, n.pos)
			}
			// Reverse.
			for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
			// Smooth: drop collinear intermediate nodes.
			nodes = smoothPath(nodes)
			return PathFindResult{Found: true, Nodes: nodes, Expansions: expansions}
		}

		if expansions >= MaxExpand {
			return PathFindResult{Found: false, BudgetHit: true, Expansions: expansions}
		}

		for _, nb := range neighbors(cur.pos) {
			stepCost := 1.0
			if nb.Y != cur.pos.Y {
				stepCost += 0.5 // climbing or falling costs a little more
			}
			// Per-cell malus (lava/water/powder-snow per the mob's profile).
			// A blocked cell is skipped entirely; the goal cell is exempt so
			// a mob can still path *to* an otherwise-costly destination.
			if cost != nil && nb != goal.pos {
				m := cost(nb.X, nb.Y, nb.Z)
				if m >= MalusBlocked {
					continue
				}
				stepCost += m
			}
			tentativeG := cur.g + stepCost
			n := get(nb)
			if tentativeG < n.g {
				n.g = tentativeG
				n.f = tentativeG + manhattan(nb, goal.pos)
				n.prev = cur
				if !n.open {
					n.open = true
					openList = append(openList, n)
				}
			}
		}
	}

	return PathFindResult{Found: false, Expansions: expansions}
}

func manhattan(a, b PathNode) float64 {
	return float64(absI(a.X-b.X) + absI(a.Y-b.Y) + absI(a.Z-b.Z))
}

// smoothPath drops intermediate nodes that lie on a straight line
// between two non-adjacent nodes. Reduces the per-tick step count
// and gives mobs a more natural-looking "diagonal across the room"
// path rather than zig-zagging cell-by-cell.
func smoothPath(in Path) Path {
	if len(in) <= 2 {
		return in
	}
	out := make(Path, 0, len(in))
	out = append(out, in[0])
	for i := 1; i < len(in)-1; i++ {
		a, b, c := in[i-1], in[i], in[i+1]
		// Collinear if (b-a) and (c-b) are parallel (cross product
		// is zero in this 3-axis comparison).
		dx1, dy1, dz1 := b.X-a.X, b.Y-a.Y, b.Z-a.Z
		dx2, dy2, dz2 := c.X-b.X, c.Y-b.Y, c.Z-b.Z
		if dx1*dy2-dy1*dx2 == 0 &&
			dx1*dz2-dz1*dx2 == 0 &&
			dy1*dz2-dz1*dy2 == 0 {
			continue // skip b
		}
		out = append(out, b)
	}
	out = append(out, in[len(in)-1])
	return out
}
