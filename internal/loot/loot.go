// Package loot resolves what item stacks a block drops when broken, using the
// vanilla 26.1 block loot tables (generated into tables_gen.go). Only the
// bare-hand survival drop is modelled — tools, silk touch, and fortune are not
// applied. Regenerate the table with: go run ./internal/loot/gen
package loot

import "math/rand"

// Drop describes one item a block can yield: a count range [Min,Max] produced
// with probability Chance (1 = always).
type Drop struct {
	Item   string
	Min    int
	Max    int
	Chance float64
}

// ItemStack is a resolved drop: a concrete item name and count.
type ItemStack struct {
	Item  string
	Count int
}

// Rolls returns the concrete item stacks a block drops when broken bare-handed.
// blockName is the namespaced block name ("minecraft:stone"); rng is used for
// chance- and range-based drops. Returns nil if the block has no drops (e.g. a
// drop table gated entirely on tools, or an unknown block).
func Rolls(blockName string, rng *rand.Rand) []ItemStack {
	drops, ok := blockDrops[blockName]
	if !ok {
		return nil
	}
	var out []ItemStack
	for _, d := range drops {
		if d.Chance < 1 && rng.Float64() >= d.Chance {
			continue
		}
		count := d.Min
		if d.Max > d.Min {
			count += rng.Intn(d.Max - d.Min + 1)
		}
		if count <= 0 {
			continue
		}
		out = append(out, ItemStack{Item: d.Item, Count: count})
	}
	return out
}

// HasDrops reports whether a block has any bare-hand drop entry.
func HasDrops(blockName string) bool {
	_, ok := blockDrops[blockName]
	return ok
}
