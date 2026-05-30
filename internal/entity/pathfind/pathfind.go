// Package pathfind implements A* pathfinding for entities over an abstract
// block grid. The navigable space is supplied by the caller via Nav, so this
// package stays independent of the world/storage layer. See R5.1 / §1.7.
package pathfind

import (
	"container/heap"

	"livingworld/internal/registry"
)

// Nav reports which block positions an entity may occupy. Supplying it keeps
// the pathfinder independent of the world layer.
type Nav interface {
	Walkable(p registry.Pos) bool
}

// Find runs A* from start to goal over the 6 axis-aligned neighbours with
// uniform cost and a Manhattan heuristic. It returns the path (start..goal
// inclusive) or nil if goal is unreachable within maxExpand node expansions.
// Both start and goal must be walkable.
func Find(nav Nav, start, goal registry.Pos, maxExpand int) []registry.Pos {
	if !nav.Walkable(start) || !nav.Walkable(goal) {
		return nil
	}
	gScore := map[registry.Pos]int{start: 0}
	cameFrom := map[registry.Pos]registry.Pos{}
	open := &queue{}
	heap.Push(open, &node{pos: start, f: heuristic(start, goal)})

	for expanded := 0; open.Len() > 0; {
		cur := heap.Pop(open).(*node)
		if cur.g > gScore[cur.pos] {
			continue // stale entry superseded by a cheaper path
		}
		if cur.pos == goal {
			return reconstruct(cameFrom, goal)
		}
		if expanded >= maxExpand {
			return nil
		}
		expanded++
		for _, n := range neighbours(cur.pos) {
			if !nav.Walkable(n) {
				continue
			}
			t := cur.g + 1
			if best, ok := gScore[n]; ok && t >= best {
				continue
			}
			gScore[n] = t
			cameFrom[n] = cur.pos
			heap.Push(open, &node{pos: n, g: t, f: t + heuristic(n, goal)})
		}
	}
	return nil
}

func neighbours(p registry.Pos) [6]registry.Pos {
	return [6]registry.Pos{
		{X: p.X + 1, Y: p.Y, Z: p.Z}, {X: p.X - 1, Y: p.Y, Z: p.Z},
		{X: p.X, Y: p.Y + 1, Z: p.Z}, {X: p.X, Y: p.Y - 1, Z: p.Z},
		{X: p.X, Y: p.Y, Z: p.Z + 1}, {X: p.X, Y: p.Y, Z: p.Z - 1},
	}
}

func heuristic(a, b registry.Pos) int { return abs(a.X-b.X) + abs(a.Y-b.Y) + abs(a.Z-b.Z) }

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func reconstruct(cameFrom map[registry.Pos]registry.Pos, goal registry.Pos) []registry.Pos {
	path := []registry.Pos{goal}
	for cur := goal; ; {
		prev, ok := cameFrom[cur]
		if !ok {
			break
		}
		path = append(path, prev)
		cur = prev
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// node is an open-set entry; the queue orders by f = g + heuristic.
type node struct {
	pos  registry.Pos
	g, f int
}

type queue []*node

func (q queue) Len() int           { return len(q) }
func (q queue) Less(i, j int) bool { return q[i].f < q[j].f }
func (q queue) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any)        { *q = append(*q, x.(*node)) }
func (q *queue) Pop() any {
	old := *q
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return it
}
