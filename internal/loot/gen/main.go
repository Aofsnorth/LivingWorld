//go:build ignore

// Command gen parses the vanilla 26.1 block loot tables (datagen JSON output)
// into a compact Go map embedded in the loot package. Run from the repo root:
//
//	go run ./internal/loot/gen
//
// It reads third_party/go-mc/temp/cache/26.1-datagen/generated/data/minecraft/
// loot_table/blocks/*.json and writes internal/loot/tables_gen.go.
//
// We model the BARE-HAND survival drop only: branches gated on a tool
// (silk_touch / match_tool / shears) are skipped, so a player with no tools gets
// the natural drop (stone→cobblestone, grass_block→dirt, leaves→sapling/apple by
// chance, ores→raw material). Tool/enchant-aware drops are out of scope for now.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// loot table JSON shapes (only the fields we need).
type table struct {
	Pools []pool `json:"pools"`
}
type pool struct {
	Rolls   any     `json:"rolls"`
	Entries []entry `json:"entries"`
}
type entry struct {
	Type       string      `json:"type"`
	Name       string      `json:"name"`
	Children   []entry     `json:"children"`
	Conditions []condition `json:"conditions"`
	Functions  []function  `json:"functions"`
}
type condition struct {
	Condition string `json:"condition"`
	// match_tool predicate, block_state_property, table_bonus chances, etc.
	Chances []float64 `json:"chances"`
}
type function struct {
	Function string `json:"function"`
	Count    any    `json:"count"` // number or {min,max} via set_count
	Add      bool   `json:"add"`
}

// Drop is one item the block yields bare-handed.
type Drop struct {
	Item   string
	Min    int
	Max    int
	Chance float64 // 0..1; 1 = always
}

// entrySkipped reports whether an entry should be skipped when resolving the
// bare-hand drop. We skip:
//   - tool gates (match_tool / silk touch / shears, wrapped in any_of/all_of)
//   - block-state gates (e.g. a fully-grown crop) — the broken block's state is
//     not modelled, so we fall through to the not-grown (seed) branch, which is
//     also the conservative "you didn't wait for it to ripen" outcome.
func entrySkipped(e entry) bool {
	for _, c := range e.Conditions {
		switch c.Condition {
		case "minecraft:match_tool", "minecraft:any_of", "minecraft:all_of",
			"minecraft:block_state_property":
			return true
		}
	}
	return false
}

// chanceOf returns the drop chance from a table_bonus/random_chance condition,
// taking the no-fortune (first) probability. Returns 1 if unconditional.
func chanceOf(e entry) float64 {
	for _, c := range e.Conditions {
		if len(c.Chances) > 0 {
			return c.Chances[0] // level-0 (no fortune) chance
		}
	}
	return 1
}

// countOf extracts a fixed/min-max count from set_count, defaulting to 1..1.
func countOf(e entry) (int, int) {
	min, max := 1, 1
	for _, f := range e.Functions {
		if f.Function != "minecraft:set_count" {
			continue
		}
		switch v := f.Count.(type) {
		case float64:
			min, max = int(v), int(v)
		case map[string]any:
			if mn, ok := v["min"].(float64); ok {
				min = int(mn)
			}
			if mx, ok := v["max"].(float64); ok {
				max = int(mx)
			}
		}
	}
	return min, max
}

// collect walks an entry tree, appending the bare-hand item drops it produces.
func collect(e entry, out *[]Drop) {
	switch e.Type {
	case "minecraft:alternatives":
		// alternatives = exactly one branch wins, tried in order. For bare-hand
		// drops we take the first child that isn't tool/state gated. A chance-gated
		// child (e.g. gravel → flint at 10%) is NOT exclusive in practice: if it
		// fails, the next child (gravel itself) drops. So we keep emitting children
		// until we reach one that is unconditional (chance 1), which is the
		// guaranteed fallback that ends the chain.
		for _, ch := range e.Children {
			if entrySkipped(ch) {
				continue
			}
			collect(ch, out)
			if chanceOf(ch) >= 1 {
				return // unconditional fallback reached → chain ends
			}
		}
	case "minecraft:item":
		if entrySkipped(e) || e.Name == "" {
			return
		}
		min, max := countOf(e)
		*out = append(*out, Drop{Item: e.Name, Min: min, Max: max, Chance: chanceOf(e)})
	case "minecraft:group", "minecraft:sequence":
		for _, ch := range e.Children {
			collect(ch, out)
		}
	}
}

func main() {
	dir := filepath.Join("third_party", "go-mc", "temp", "cache", "26.1-datagen",
		"generated", "data", "minecraft", "loot_table", "blocks")
	files, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read loot dir: %v\n", err)
		os.Exit(1)
	}

	drops := map[string][]Drop{}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		block := "minecraft:" + strings.TrimSuffix(f.Name(), ".json")
		raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		var t table
		if err := json.Unmarshal(raw, &t); err != nil {
			continue
		}
		var ds []Drop
		for _, p := range t.Pools {
			for _, e := range p.Entries {
				collect(e, &ds)
			}
		}
		if len(ds) > 0 {
			drops[block] = ds
		}
	}

	keys := make([]string, 0, len(drops))
	for k := range drops {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("// Code generated by internal/loot/gen; DO NOT EDIT.\n")
	b.WriteString("// Source: vanilla 26.1 block loot tables (datagen). Bare-hand drops only.\n\n")
	b.WriteString("package loot\n\n")
	b.WriteString("var blockDrops = map[string][]Drop{\n")
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("\t%q: {", k))
		for i, d := range drops[k] {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(fmt.Sprintf("{Item: %q, Min: %d, Max: %d, Chance: %g}", d.Item, d.Min, d.Max, d.Chance))
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n")

	if err := os.WriteFile(filepath.Join("internal", "loot", "tables_gen.go"), []byte(b.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote internal/loot/tables_gen.go with %d block loot tables\n", len(keys))
}
