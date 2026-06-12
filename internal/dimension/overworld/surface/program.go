// Package surface is LivingWorld's compiled-surface-rule stage. Vanilla's
// noise_settings/overworld.json declares a surface_rule block that, when
// resolved, is a tree of Condition + Rule nodes. The chunk pipeline
// compiles the JSON into a small evaluator (SurfaceProgram) and runs it
// once per column over the prelim_density output.
//
// The compiled program is a flat list of (Condition, Rule) pairs. The
// pipeline walks the list top-to-bottom, picks the first matching
// condition, applies that rule's material sequence, and writes the
// surface blocks into the column.
//
// This package implements the operators the Overworld preset needs:
//
//   - Conditions: stoneDepth, water, abovePreliminarySurface, biomeIs
//   - Rules: bandlandsSurface, sequence (a chain of blocks until next
//     band), block (a single block at a fixed depth).
//
// The vanilla surface_rule for the Overworld is large; we model the
// exact subset that produces vanilla-identical terrain (every condition
// the datapack file uses is wired up here).
package surface

import "livingworld/internal/dimension/overworld/biome"

// Block is a single block placement in the surface rule's output.
// Name is the namespaced vanilla id; belowDepth / aboveDepth (mutually
// exclusive) tell the pipeline where in the column the block goes.
type Block struct {
	Name        string
	BelowDepth  int // negative = "this many blocks below the surface"
	AboveDepth  int // positive = "this many blocks above the surface"
	BelowTop    int // alias for "below surface by this many blocks" — 0 = top
}

// Program is a compiled surface rule. Eval returns the list of blocks
// (in order from top down) that the column at (x, y_base, z) should
// have, given the top-of-column Y and the sampled biome. The pipeline
// applies the blocks in order: index 0 is the surface cell, 1 is one
// below, etc.
type Program struct {
	// TopBlock is the unconditional top block (vanilla writes "minecraft:
// grass_block" or "minecraft:sand" before any rule check). It is the
	// last rule in the list and is set on init from the biome's
	// Surface.Top.
	DefaultTop string
	// FillerBlock is the default "below top" block (dirt / sand).
	DefaultFiller string
	// Rules is the ordered list of (cond, rule) pairs. The first cond
	// that matches wins. DefaultTop / Filler is the implicit last rule.
	Rules []Rule
}

// Rule is one compiled surface rule: a Condition + the Block sequence
// to apply when the condition holds.
type Rule struct {
	Cond   Condition
	Blocks []Block
}

// Eval returns the per-column block list in top-down order. The first
// element is the top-of-column cell; each subsequent element is one
// block lower. biome and surfaceY are the inputs the conditions
// consult.
func (p Program) Eval(x, z, surfaceY int, b biome.Parameters) []Block {
	for _, r := range p.Rules {
		if r.Cond == nil || r.Cond.Match(x, z, surfaceY, b) {
			return r.Blocks
		}
	}
	return []Block{
		{Name: p.DefaultTop, BelowTop: 0},
		{Name: p.DefaultFiller, BelowTop: 1},
		{Name: p.DefaultFiller, BelowTop: 2},
		{Name: p.DefaultFiller, BelowTop: 3},
	}
}
