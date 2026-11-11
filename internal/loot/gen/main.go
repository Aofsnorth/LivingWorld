//go:build ignore

// Command gen parses the vanilla 26.3 block loot tables (from the datagen cache
// directory or the local vanilla 26.3 jar) into compact Go maps embedded in the
// loot package. Run from the repo root:
//
//	go run ./internal/loot/gen
//
// It generates two maps in internal/loot/tables_gen.go:
//   - blockDrops: natural survival drops without Silk Touch or Shears
//   - shearDrops: drops when broken with Shears (minecraft:shears) for blocks
//     whose loot table branches on minecraft:tool/can_shear or shears match_tool.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

type evalMode int

const (
	modeHand evalMode = iota
	modeShears
)

type table struct {
	Pools []pool `json:"pools"`
}

type pool struct {
	Rolls      any         `json:"rolls"`
	Entries    []entry     `json:"entries"`
	Condition  *condValue  `json:"condition"`
	Conditions []condValue `json:"conditions"`
}

func (p pool) allConditions() []condValue {
	var out []condValue
	if p.Condition != nil {
		out = append(out, *p.Condition)
	}
	out = append(out, p.Conditions...)
	return out
}

type entry struct {
	Type       string      `json:"type"`
	Name       string      `json:"name"`
	Children   []entry     `json:"children"`
	Condition  *condValue  `json:"condition"`
	Conditions []condValue `json:"conditions"`
	Modifier   funcSlice   `json:"modifier"`
	Modifiers  funcSlice   `json:"modifiers"`
	Functions  funcSlice   `json:"functions"`
}

func (e entry) allConditions() []condValue {
	var out []condValue
	if e.Condition != nil {
		out = append(out, *e.Condition)
	}
	out = append(out, e.Conditions...)
	return out
}

func (e entry) allFunctions() []function {
	var out []function
	out = append(out, e.Modifier...)
	out = append(out, e.Modifiers...)
	out = append(out, e.Functions...)
	return out
}

type funcSlice []function

func (fs *funcSlice) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '[' {
		var arr []function
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*fs = arr
		return nil
	}
	var single function
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*fs = []function{single}
	return nil
}

type function struct {
	Type       string      `json:"type"`
	Function   string      `json:"function"`
	Count      any         `json:"count"`
	Add        bool        `json:"add"`
	Condition  *condValue  `json:"condition"`
	Conditions []condValue `json:"conditions"`
}

func (f function) kind() string {
	if f.Type != "" {
		return f.Type
	}
	return f.Function
}

func (f function) hasConditions() bool {
	return f.Condition != nil || len(f.Conditions) > 0
}

type condValue struct {
	Type       string         `json:"type"`
	Condition  string         `json:"condition"`
	Term       *condValue     `json:"term"`
	Terms      []condValue    `json:"terms"`
	Chances    []float64      `json:"chances"`
	Chance     *float64       `json:"chance"`
	State      map[string]any `json:"state"`
	Properties map[string]any `json:"properties"`
	Predicate  map[string]any `json:"predicate"`
}

func (c *condValue) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		c.Type = s
		return nil
	}
	type rawCond condValue
	var rc rawCond
	if err := json.Unmarshal(data, &rc); err != nil {
		return err
	}
	*c = condValue(rc)
	return nil
}

func (c condValue) kind() string {
	if c.Type != "" {
		return c.Type
	}
	return c.Condition
}

// Drop is one item a block yields.
type Drop struct {
	Item   string
	Min    int
	Max    int
	Chance float64 // 0..1; 1 = always
}

// isDefaultMultiPartState reports whether a block-state condition only checks the
// canonical primary half of a multi-part block (half="lower" for doors and double
// plants, or part="head" for beds). Allowing the primary half ensures doors, beds,
// and double plants produce exactly one drop entry in tables_gen.go.
func isDefaultMultiPartState(state map[string]any) bool {
	if len(state) != 1 {
		return false
	}
	if v, ok := state["half"].(string); ok && v == "lower" {
		return true
	}
	if v, ok := state["part"].(string); ok && v == "head" {
		return true
	}
	return false
}

func predicateMatchesShears(pred map[string]any) bool {
	if pred == nil {
		return false
	}
	switch v := pred["items"].(type) {
	case string:
		return v == "minecraft:shears"
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == "minecraft:shears" {
				return true
			}
		}
	}
	return false
}

// evalCond evaluates whether a loot condition is satisfied under the given mode.
func evalCond(c condValue, mode evalMode) bool {
	switch c.kind() {
	case "", "minecraft:survives_explosion", "minecraft:table_bonus", "minecraft:random_chance":
		return true
	case "minecraft:location_check":
		// Used alongside match_block(half="lower") on double plants to verify the other half.
		return true
	case "minecraft:tool/can_shear":
		return mode == modeShears
	case "minecraft:tool/can_silk_touch":
		return false
	case "minecraft:match_tool":
		if mode == modeShears && predicateMatchesShears(c.Predicate) {
			return true
		}
		return false
	case "minecraft:inverted":
		if c.Term == nil {
			return false
		}
		return !evalCond(*c.Term, mode)
	case "minecraft:any_of":
		for _, t := range c.Terms {
			if evalCond(t, mode) {
				return true
			}
		}
		return false
	case "minecraft:all_of":
		for _, t := range c.Terms {
			if !evalCond(t, mode) {
				return false
			}
		}
		return true
	case "minecraft:match_block":
		return isDefaultMultiPartState(c.State)
	case "minecraft:block_state_property":
		return isDefaultMultiPartState(c.Properties)
	default:
		// Entity properties, damage source properties, etc.
		return false
	}
}

func poolSkipped(p pool, mode evalMode) bool {
	for _, c := range p.allConditions() {
		if !evalCond(c, mode) {
			return true
		}
	}
	return false
}

func entrySkipped(e entry, mode evalMode) bool {
	for _, c := range e.allConditions() {
		if !evalCond(c, mode) {
			return true
		}
	}
	return false
}

// chanceOf returns the drop chance from a table_bonus/random_chance condition,
// taking the no-fortune (first) probability. Returns 1 if unconditional.
func chanceOf(e entry) float64 {
	for _, c := range e.allConditions() {
		if c.Chance != nil {
			return *c.Chance
		}
		if len(c.Chances) > 0 {
			return c.Chances[0] // level-0 (no fortune) chance
		}
	}
	return 1
}

// countOf extracts a fixed/min-max count from unconditional set_count modifiers,
// defaulting to 1..1. State-conditioned set_count modifiers (such as double slabs
// or multi-candle / sea_pickle clusters) are skipped here and handled at runtime
// from the broken block's state properties.
func countOf(e entry) (int, int) {
	min, max := 1, 1
	for _, f := range e.allFunctions() {
		if f.kind() != "minecraft:set_count" {
			continue
		}
		if f.hasConditions() {
			continue
		}
		switch v := f.Count.(type) {
		case float64:
			if f.Add || int(v) <= 0 {
				// Multiface blocks (e.g. glow_lichen, sculk_vein) initialize set_count to 0
				// or adjust via additive set_count modifiers (+1 per face, -1 base). Default to 1..1
				// in the static table and scale by attached face count at runtime.
				continue
			}
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

// collect walks an entry tree, appending the item drops it produces under mode.
func collect(e entry, mode evalMode, out *[]Drop) bool {
	if entrySkipped(e, mode) {
		return false
	}
	switch e.Type {
	case "minecraft:alternatives":
		emittedAny := false
		for _, ch := range e.Children {
			if entrySkipped(ch, mode) {
				continue
			}
			before := len(*out)
			if collect(ch, mode, out) && len(*out) > before {
				emittedAny = true
				if chanceOf(ch) >= 1 {
					return true // unconditional fallback reached -> chain ends
				}
			}
		}
		return emittedAny
	case "minecraft:item":
		if e.Name == "" {
			return false
		}
		min, max := countOf(e)
		if max <= 0 {
			return false
		}
		if min < 0 {
			min = 0
		}
		*out = append(*out, Drop{Item: e.Name, Min: min, Max: max, Chance: chanceOf(e)})
		return true
	case "minecraft:empty":
		// An unconditional empty branch in alternatives terminates the chain with no drop.
		return true
	case "minecraft:group", "minecraft:sequence":
		emitted := false
		for _, ch := range e.Children {
			if collect(ch, mode, out) {
				emitted = true
			}
		}
		return emitted
	}
	return false
}

func resolveTable(t table, mode evalMode) []Drop {
	var ds []Drop
	for _, p := range t.Pools {
		if poolSkipped(p, mode) {
			continue
		}
		for _, e := range p.Entries {
			collect(e, mode, &ds)
		}
	}
	return ds
}

func loadBlockLootTables() (map[string][]byte, string, error) {
	dir := filepath.Join("third_party", "go-mc", "temp", "cache", "26.1-datagen",
		"generated", "data", "minecraft", "loot_table", "blocks")
	if files, err := os.ReadDir(dir); err == nil {
		raws := make(map[string][]byte, len(files))
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err == nil {
				raws[strings.TrimSuffix(f.Name(), ".json")] = b
			}
		}
		if len(raws) > 0 {
			return raws, "vanilla 26.1 datagen", nil
		}
	}

	home, _ := os.UserHomeDir()
	jarCandidates := []string{
		os.Getenv("MC_JAR"),
		filepath.Join(home, ".gradle", "caches", "fabric-loom", "minecraftMaven",
			"net", "minecraft", "minecraft-common-deobf", "26.3", "minecraft-common-deobf-26.3.jar"),
	}
	for _, jarPath := range jarCandidates {
		if jarPath == "" {
			continue
		}
		zr, err := zip.OpenReader(jarPath)
		if err != nil {
			continue
		}
		raws := make(map[string][]byte)
		const prefix = "data/minecraft/loot_table/blocks/"
		for _, f := range zr.File {
			if !strings.HasPrefix(f.Name, prefix) || !strings.HasSuffix(f.Name, ".json") {
				continue
			}
			base := strings.TrimSuffix(strings.TrimPrefix(f.Name, prefix), ".json")
			if strings.Contains(base, "/") {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err == nil {
				raws[base] = data
			}
		}
		zr.Close()
		if len(raws) > 0 {
			return raws, "vanilla 26.3 block loot tables", nil
		}
	}
	return nil, "", fmt.Errorf("no block loot table JSONs found in datagen dir or fabric-loom 26.3 jar")
}

func writeDropMap(b *strings.Builder, varName string, drops map[string][]Drop) []string {
	keys := make([]string, 0, len(drops))
	for k := range drops {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	b.WriteString(fmt.Sprintf("var %s = map[string][]Drop{\n", varName))
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
	return keys
}

func main() {
	rawTables, sourceDesc, err := loadBlockLootTables()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load loot tables: %v\n", err)
		os.Exit(1)
	}

	handDrops := map[string][]Drop{}
	shearDrops := map[string][]Drop{}

	for name, raw := range rawTables {
		block := "minecraft:" + name
		var t table
		if err := json.Unmarshal(raw, &t); err != nil {
			continue
		}
		hds := resolveTable(t, modeHand)
		// Cobweb requires a sword (drops string) or shears (drops cobweb); bare hand drops nothing.
		if len(hds) > 0 && block != "minecraft:cobweb" {
			handDrops[block] = hds
		}
		sds := resolveTable(t, modeShears)
		if len(sds) > 0 && !reflect.DeepEqual(sds, hds) {
			shearDrops[block] = sds
		}
	}

	var b strings.Builder
	b.WriteString("// Code generated by internal/loot/gen; DO NOT EDIT.\n")
	b.WriteString(fmt.Sprintf("// Source: %s. Bare-hand and Shears survival drops.\n\n", sourceDesc))
	b.WriteString("package loot\n\n")
	handKeys := writeDropMap(&b, "blockDrops", handDrops)
	b.WriteString("\n")
	shearKeys := writeDropMap(&b, "shearDrops", shearDrops)

	if err := os.WriteFile(filepath.Join("internal", "loot", "tables_gen.go"), []byte(b.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote internal/loot/tables_gen.go with %d bare-hand tables and %d shears tables\n", len(handKeys), len(shearKeys))
}
