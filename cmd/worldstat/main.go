// Command worldstat reports what the overworld generator actually produces:
// surface-height profile and block composition across a sampled region. It
// exists to answer "does the pipeline generate real vanilla-like terrain?"
// with numbers instead of assumptions.
package main

import (
	"fmt"
	"sort"

	"livingworld/internal/dimension/overworld/pipeline"
	"livingworld/internal/world"
)

func main() {
	const seed = 12345
	const step = 16 // sample every Nth chunk
	const span = 512

	g := pipeline.NewGenerator(seed)

	heights := make([]int, 0, span*span/step)
	minH, maxH := 1<<30, -(1 << 30)
	blocks := map[string]int{}
	total := 0

	for cx := -span / 2; cx < span/2; cx += step {
		for cz := -span / 2; cz < span/2; cz += step {
			c := g.Generate(cx, cz)
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					top := -1
					name := ""
					for y := 320; y >= -64; y-- {
						id := c.GetBlock(x, y, z).ID()
						if !world.IsAir(id) {
							top, name = y, world.StateName(id)
							break
						}
					}
					if top < 0 {
						continue
					}
					heights = append(heights, top)
					blocks[name]++
					total++
					if top < minH {
						minH = top
					}
					if top > maxH {
						maxH = top
					}
				}
			}
		}
	}

	if len(heights) == 0 {
		fmt.Println("NO TERRAIN: every sampled column is empty air")
		return
	}
	sort.Ints(heights)
	sum := 0
	for _, h := range heights {
		sum += h
	}

	fmt.Printf("seed=%d  solid columns=%d\n", seed, total)
	fmt.Printf("surface height: min=%d max=%d mean=%d p10=%d p50=%d p90=%d\n",
		minH, maxH, sum/len(heights),
		heights[len(heights)/10], heights[len(heights)/2], heights[len(heights)*9/10])

	type kv struct {
		k string
		v int
	}
	list := make([]kv, 0, len(blocks))
	for k, v := range blocks {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })

	fmt.Printf("distinct surface blocks: %d\n", len(list))
	for i, e := range list {
		if i >= 10 {
			break
		}
		fmt.Printf("  %-38s %6d (%5.1f%%)\n", e.k, e.v, 100*float64(e.v)/float64(total))
	}
}
