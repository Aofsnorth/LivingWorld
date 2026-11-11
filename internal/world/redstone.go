package world

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"livingworld/internal/mobs"

	"github.com/Tnze/go-mc/level/block"
	"github.com/Tnze/go-mc/nbt"
)

// Flat O(1) lookup tables indexed by canonical state ID (0..len(block.StateList)-1).
// Initialized once at startup so hot paths (mob AI SolidAt at 20 Hz, block
// placement replaceable checks) perform a single bounds-checked slice read with
// zero allocations.
var (
	solidStateLUT       []bool
	replaceableStateLUT []bool
	canonicalStateCache sync.Map // map[int32]string
	statePropsCache     sync.Map // map[int32]map[string]string
)

func init() {
	n := len(block.StateList)
	solidStateLUT = make([]bool, n)
	replaceableStateLUT = make([]bool, n)

	for id := 0; id < n; id++ {
		name := block.StateList[id].ID()
		if isReplaceableByName(name) {
			replaceableStateLUT[id] = true
		}
		if isNonSolidByName(name) {
			solidStateLUT[id] = false
			continue
		}
		// Doors, trapdoors, and fence gates are non-solid when open=true.
		if strings.HasSuffix(name, "_door") || strings.HasSuffix(name, "_trapdoor") || strings.HasSuffix(name, "_fence_gate") {
			props := StateProperties(int32(id))
			if props["open"] == "true" {
				solidStateLUT[id] = false
				continue
			}
		}
		// Single-layer snow is passable like carpet.
		if name == "minecraft:snow" {
			props := StateProperties(int32(id))
			if props["layers"] == "1" {
				solidStateLUT[id] = false
				replaceableStateLUT[id] = true
				continue
			}
		}
		solidStateLUT[id] = true
	}
}

func isReplaceableByName(name string) bool {
	switch name {
	case "minecraft:air", "minecraft:cave_air", "minecraft:void_air",
		"minecraft:water", "minecraft:lava", "minecraft:bubble_column",
		"minecraft:short_grass", "minecraft:tall_grass", "minecraft:fern", "minecraft:large_fern",
		"minecraft:dead_bush", "minecraft:vine", "minecraft:glow_lichen", "minecraft:sculk_vein",
		"minecraft:Seagrass", "minecraft:seagrass", "minecraft:tall_seagrass",
		"minecraft:fire", "minecraft:soul_fire", "minecraft:structure_void", "minecraft:light",
		"minecraft:bush", "minecraft:firefly_bush", "minecraft:short_dry_grass", "minecraft:tall_dry_grass",
		"minecraft:leaf_litter", "minecraft:wildflowers", "minecraft:pink_petals":
		return true
	}
	return false
}

func isNonSolidByName(name string) bool {
	if isReplaceableByName(name) {
		return true
	}
	switch name {
	case "minecraft:dandelion", "minecraft:poppy", "minecraft:blue_orchid", "minecraft:allium",
		"minecraft:azure_bluet", "minecraft:red_tulip", "minecraft:orange_tulip", "minecraft:white_tulip",
		"minecraft:pink_tulip", "minecraft:oxeye_daisy", "minecraft:cornflower", "minecraft:lily_of_the_valley",
		"minecraft:wither_rose", "minecraft:torchflower", "minecraft:torchflower_crop",
		"minecraft:pitcher_plant", "minecraft:pitcher_crop", "minecraft:sunflower", "minecraft:lilac",
		"minecraft:rose_bush", "minecraft:peony", "minecraft:spore_blossom",
		"minecraft:open_eyeblossom", "minecraft:closed_eyeblossom",
		"minecraft:brown_mushroom", "minecraft:red_mushroom", "minecraft:crimson_fungus", "minecraft:warped_fungus",
		"minecraft:crimson_roots", "minecraft:warped_roots", "minecraft:nether_sprouts",
		"minecraft:sugar_cane", "minecraft:kelp", "minecraft:kelp_plant",
		"minecraft:wheat", "minecraft:carrots", "minecraft:potatoes", "minecraft:beetroots",
		"minecraft:melon_stem", "minecraft:pumpkin_stem", "minecraft:attached_melon_stem", "minecraft:attached_pumpkin_stem",
		"minecraft:sweet_berry_bush", "minecraft:nether_wart", "minecraft:cocoa",
		"minecraft:cave_vines", "minecraft:cave_vines_plant", "minecraft:weeping_vines", "minecraft:weeping_vines_plant",
		"minecraft:twisting_vines", "minecraft:twisting_vines_plant", "minecraft:pale_hanging_moss", "minecraft:hanging_roots",
		"minecraft:moss_carpet", "minecraft:pale_moss_carpet",
		"minecraft:torch", "minecraft:wall_torch", "minecraft:soul_torch", "minecraft:soul_wall_torch",
		"minecraft:redstone_torch", "minecraft:redstone_wall_torch", "minecraft:copper_torch", "minecraft:copper_wall_torch",
		"minecraft:redstone_wire", "minecraft:lever", "minecraft:tripwire", "minecraft:tripwire_hook",
		"minecraft:ladder", "minecraft:scaffolding", "minecraft:cobweb", "minecraft:Frogspawn", "minecraft:frogspawn",
		"minecraft:rail", "minecraft:powered_rail", "minecraft:detector_rail", "minecraft:activator_rail":
		return true
	}
	if strings.HasSuffix(name, "_sapling") ||
		strings.HasSuffix(name, "_propagule") ||
		strings.HasSuffix(name, "_button") ||
		strings.HasSuffix(name, "_pressure_plate") ||
		strings.HasSuffix(name, "_carpet") ||
		strings.HasSuffix(name, "_sign") ||
		strings.HasSuffix(name, "_hanging_sign") ||
		strings.HasSuffix(name, "_banner") ||
		strings.HasSuffix(name, "_coral") ||
		strings.HasSuffix(name, "_coral_fan") ||
		strings.HasSuffix(name, "_coral_wall_fan") {
		return true
	}
	return false
}

// IsSolidBlock reports in O(1) time whether canonical block state id blocks
// entity/mob movement and provides a solid collision surface.
func IsSolidBlock(id int32) bool {
	if id <= 0 || int(id) >= len(solidStateLUT) {
		return false
	}
	return solidStateLUT[id]
}

// IsReplaceableBlock reports in O(1) time whether a player placing a block on
// or into state id replaces it (air, short grass, tall grass, fern, water,
// vines, single snow layer, etc.).
func IsReplaceableBlock(id int32) bool {
	if id < 0 || int(id) >= len(replaceableStateLUT) {
		return false
	}
	return replaceableStateLUT[id]
}

// StateProperties returns a copy of the Java block-state properties for id
// (e.g. {"facing": "north", "powered": "true"}).
func StateProperties(id int32) map[string]string {
	if id <= 0 || int(id) >= len(block.StateList) {
		return nil
	}
	if cached, ok := statePropsCache.Load(id); ok {
		src := cached.(map[string]string)
		if len(src) == 0 {
			return nil
		}
		out := make(map[string]string, len(src))
		for k, v := range src {
			out[k] = v
		}
		return out
	}
	raw, err := nbt.Marshal(block.StateList[id])
	if err != nil {
		return nil
	}
	var props map[string]string
	if err := nbt.Unmarshal(raw, &props); err != nil {
		return nil
	}
	statePropsCache.Store(id, props)
	if len(props) == 0 {
		return nil
	}
	out := make(map[string]string, len(props))
	for k, v := range props {
		out[k] = v
	}
	return out
}

// CanonicalState formats a canonical block state ID as its full Java state
// string (e.g. "minecraft:vine[east=false,north=true,south=false,up=false,west=false]").
func CanonicalState(id int32) string {
	if id <= 0 || int(id) >= len(block.StateList) {
		return "minecraft:air"
	}
	if cached, ok := canonicalStateCache.Load(id); ok {
		return cached.(string)
	}
	name := block.StateList[id].ID()
	props := StateProperties(id)
	if len(props) == 0 {
		canonicalStateCache.Store(id, name)
		return name
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('[')
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(props[k])
	}
	sb.WriteByte(']')
	res := sb.String()
	canonicalStateCache.Store(id, res)
	return res
}

// InteractBlock handles right-click / tap block interactions on levers, buttons,
// doors, trapdoors, fence gates, repeaters, comparators, daylight detectors,
// and note blocks across both Java and Bedrock editions. Returns true if the
// block consumed the interaction.
func (m *Manager) InteractBlock(source BlockUpdateSource, x, y, z int) bool {
	w := m.GetDefaultWorld()
	if w == nil {
		return false
	}
	id := w.GetBlock(x, y, z).ID()
	if id == AirID {
		return false
	}
	name := StateName(id)
	props := StateProperties(id)

	switch {
	case name == "minecraft:lever":
		if props == nil {
			props = map[string]string{}
		}
		if props["powered"] == "true" {
			props["powered"] = "false"
		} else {
			props["powered"] = "true"
		}
		newID := StateIDWithProperties(name, props)
		m.SetBlockAndPublish(source, x, y, z, BlockByID(newID))
		m.OnBlockChanged(source, x, y, z)
		return true

	case strings.HasSuffix(name, "_button"):
		if props == nil {
			props = map[string]string{}
		}
		if props["powered"] == "true" {
			return true
		}
		props["powered"] = "true"
		newID := StateIDWithProperties(name, props)
		m.SetBlockAndPublish(source, x, y, z, BlockByID(newID))
		m.OnBlockChanged(source, x, y, z)
		delay := 1500 * time.Millisecond
		if name == "minecraft:stone_button" || name == "minecraft:polished_blackstone_button" {
			delay = 1000 * time.Millisecond
		}
		time.AfterFunc(delay, func() {
			curID := w.GetBlock(x, y, z).ID()
			if StateName(curID) != name {
				return
			}
			curProps := StateProperties(curID)
			if curProps == nil || curProps["powered"] != "true" {
				return
			}
			curProps["powered"] = "false"
			offID := StateIDWithProperties(name, curProps)
			m.SetBlockAndPublish(BlockUpdateSourceServer, x, y, z, BlockByID(offID))
			m.OnBlockChanged(BlockUpdateSourceServer, x, y, z)
		})
		return true

	case strings.HasSuffix(name, "_door") && name != "minecraft:iron_door":
		if props == nil {
			props = map[string]string{}
		}
		newOpen := "true"
		if props["open"] == "true" {
			newOpen = "false"
		}
		props["open"] = newOpen
		newID := StateIDWithProperties(name, props)
		m.SetBlockAndPublish(source, x, y, z, BlockByID(newID))
		otherY := y + 1
		if props["half"] == "upper" {
			otherY = y - 1
		}
		otherID := w.GetBlock(x, otherY, z).ID()
		if StateName(otherID) == name {
			otherProps := StateProperties(otherID)
			if otherProps != nil {
				otherProps["open"] = newOpen
				m.SetBlockAndPublish(source, x, otherY, z, BlockByID(StateIDWithProperties(name, otherProps)))
			}
		}
		return true

	case strings.HasSuffix(name, "_trapdoor") && name != "minecraft:iron_trapdoor":
		if props == nil {
			props = map[string]string{}
		}
		if props["open"] == "true" {
			props["open"] = "false"
		} else {
			props["open"] = "true"
		}
		m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		return true

	case strings.HasSuffix(name, "_fence_gate"):
		if props == nil {
			props = map[string]string{}
		}
		if props["open"] == "true" {
			props["open"] = "false"
		} else {
			props["open"] = "true"
		}
		m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		return true

	case name == "minecraft:repeater":
		if props == nil {
			props = map[string]string{}
		}
		d, _ := strconv.Atoi(props["delay"])
		d = (d % 4) + 1
		props["delay"] = strconv.Itoa(d)
		m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		return true

	case name == "minecraft:comparator":
		if props == nil {
			props = map[string]string{}
		}
		if props["mode"] == "subtract" {
			props["mode"] = "compare"
		} else {
			props["mode"] = "subtract"
		}
		m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		return true

	case name == "minecraft:daylight_detector":
		if props == nil {
			props = map[string]string{}
		}
		if props["inverted"] == "true" {
			props["inverted"] = "false"
		} else {
			props["inverted"] = "true"
		}
		m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		m.OnBlockChanged(source, x, y, z)
		return true

	case name == "minecraft:note_block":
		if props == nil {
			props = map[string]string{}
		}
		n, _ := strconv.Atoi(props["note"])
		props["note"] = fmt.Sprintf("%d", (n+1)%25)
		m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		return true
	}

	return false
}

var cardinalOffsets = [6][3]int{
	{0, -1, 0},
	{0, 1, 0},
	{0, 0, -1},
	{0, 0, 1},
	{-1, 0, 0},
	{1, 0, 0},
}

func facingOffset(facing string) (int, int, int) {
	switch facing {
	case "north":
		return 0, 0, -1
	case "south":
		return 0, 0, 1
	case "west":
		return -1, 0, 0
	case "east":
		return 1, 0, 0
	case "up":
		return 0, 1, 0
	case "down":
		return 0, -1, 0
	}
	return 0, 0, 0
}

// OnBlockChanged evaluates redstone power propagation, wire networks, torches,
// repeaters, comparators, lamps, doors, trapdoors, fence gates, pistons, and TNT
// around (x, y, z).
func (m *Manager) OnBlockChanged(source BlockUpdateSource, x, y, z int) {
	w := m.GetDefaultWorld()
	if w == nil {
		return
	}
	// First cascade any blocks/vines/plants that lost their required support
	// (skip when toggling an in-place switch such as a lever, button, or daylight detector).
	if name := StateName(w.GetBlock(x, y, z).ID()); name != "minecraft:lever" && !strings.HasSuffix(name, "_button") && name != "minecraft:daylight_detector" {
		m.propagateSupportUpdates(source, w, x, y, z)
	}

	// Recompute any connected redstone_wire network within 16 blocks.
	m.updateRedstoneWiresNear(source, w, x, y, z)

	// Evaluate the changed block and its 2-step neighborhood for redstone machines.
	for dy := -2; dy <= 2; dy++ {
		for dz := -2; dz <= 2; dz++ {
			for dx := -2; dx <= 2; dx++ {
				if absInt(dx)+absInt(dy)+absInt(dz) > 2 {
					continue
				}
				m.evaluateRedstoneComponent(source, w, x+dx, y+dy, z+dz)
			}
		}
	}
	// Re-sync wire network in case a torch/repeater toggled.
	m.updateRedstoneWiresNear(source, w, x, y, z)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// emittedPowerAt returns the redstone signal strength (0..15) emitted by block
// (bx, by, bz) itself (ignoring redstone_wire When includeWire is false).
func emittedPowerAt(w *World, bx, by, bz int, includeWire bool) int {
	id := w.GetBlock(bx, by, bz).ID()
	if id == AirID {
		return 0
	}
	name := StateName(id)
	switch name {
	case "minecraft:redstone_block":
		return 15
	case "minecraft:redstone_torch", "minecraft:redstone_wall_torch":
		props := StateProperties(id)
		if props == nil || props["lit"] != "false" {
			return 15
		}
		return 0
	case "minecraft:lever":
		if props := StateProperties(id); props != nil && props["powered"] == "true" {
			return 15
		}
	case "minecraft:repeater", "minecraft:comparator", "minecraft:observer":
		if props := StateProperties(id); props != nil && props["powered"] == "true" {
			return 15
		}
	case "minecraft:daylight_detector":
		props := StateProperties(id)
		inverted := props != nil && props["inverted"] == "true"
		day := isDay(w.GetDayTime())
		if (day && !inverted) || (!day && inverted) {
			return 15
		}
		return 0
	case "minecraft:redstone_wire":
		if !includeWire {
			return 0
		}
		if props := StateProperties(id); props != nil {
			p, _ := strconv.Atoi(props["power"])
			return p
		}
	default:
		if strings.HasSuffix(name, "_button") || strings.HasSuffix(name, "_pressure_plate") {
			if props := StateProperties(id); props != nil && props["powered"] == "true" {
				return 15
			}
		}
	}
	return 0
}

// receivingPowerAt returns the maximum redstone power (0..15) received at (x, y, z)
// from its 6 neighbors.
func receivingPowerAt(w *World, x, y, z int, includeWire bool) int {
	maxP := 0
	for _, off := range cardinalOffsets {
		nx, ny, nz := x+off[0], y+off[1], z+off[2]
		p := emittedPowerAt(w, nx, ny, nz, includeWire)
		if p > maxP {
			maxP = p
		}
	}
	return maxP
}

func (m *Manager) updateRedstoneWiresNear(source BlockUpdateSource, w *World, cx, cy, cz int) {
	type pos3 struct{ x, y, z int }
	var seeds []pos3
	for dy := -1; dy <= 1; dy++ {
		for dz := -1; dz <= 1; dz++ {
			for dx := -1; dx <= 1; dx++ {
				px, py, pz := cx+dx, cy+dy, cz+dz
				if StateName(w.GetBlock(px, py, pz).ID()) == "minecraft:redstone_wire" {
					seeds = append(seeds, pos3{px, py, pz})
				}
			}
		}
	}
	if len(seeds) == 0 {
		return
	}

	// Collect connected wire component (up to 64 wires).
	visited := make(map[pos3]bool, 32)
	queue := append([]pos3(nil), seeds...)
	for _, s := range seeds {
		visited[s] = true
	}
	for head := 0; head < len(queue) && len(visited) < 64; head++ {
		cur := queue[head]
		for _, off := range cardinalOffsets {
			for dy := -1; dy <= 1; dy++ {
				if off[1] != 0 && dy != 0 {
					continue
				}
				np := pos3{cur.x + off[0], cur.y + off[1] + dy, cur.z + off[2]}
				if visited[np] {
					continue
				}
				if StateName(w.GetBlock(np.x, np.y, np.z).ID()) == "minecraft:redstone_wire" {
					visited[np] = true
					queue = append(queue, np)
				}
			}
		}
	}

	// Compute non-wire input power at each wire node, then relax along edges.
	power := make(map[pos3]int, len(queue))
	for _, wp := range queue {
		power[wp] = receivingPowerAt(w, wp.x, wp.y, wp.z, false)
	}
	changed := true
	for changed {
		changed = false
		for _, wp := range queue {
			p := power[wp]
			if p <= 1 {
				continue
			}
			for _, off := range cardinalOffsets {
				np := pos3{wp.x + off[0], wp.y + off[1], wp.z + off[2]}
				if _, ok := power[np]; ok && p-1 > power[np] {
					power[np] = p - 1
					changed = true
				}
			}
		}
	}

	for _, wp := range queue {
		curID := w.GetBlock(wp.x, wp.y, wp.z).ID()
		props := StateProperties(curID)
		if props == nil {
			continue
		}
		want := strconv.Itoa(power[wp])
		if props["power"] != want {
			props["power"] = want
			newID := StateIDWithProperties("minecraft:redstone_wire", props)
			m.SetBlockAndPublish(source, wp.x, wp.y, wp.z, BlockByID(newID))
		}
	}
}

func (m *Manager) evaluateRedstoneComponent(source BlockUpdateSource, w *World, x, y, z int) {
	id := w.GetBlock(x, y, z).ID()
	if id == AirID {
		return
	}
	name := StateName(id)
	props := StateProperties(id)
	powered := receivingPowerAt(w, x, y, z, true) > 0

	switch {
	case name == "minecraft:redstone_lamp":
		if props == nil {
			return
		}
		want := "false"
		if powered {
			want = "true"
		}
		if props["lit"] != want {
			props["lit"] = want
			m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		}

	case strings.HasSuffix(name, "copper_bulb"):
		if props == nil {
			return
		}
		wasPowered := props["powered"] == "true"
		if powered != wasPowered {
			if powered {
				props["powered"] = "true"
				if props["lit"] == "true" {
					props["lit"] = "false"
				} else {
					props["lit"] = "true"
				}
			} else {
				props["powered"] = "false"
			}
			m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		}

	case name == "minecraft:redstone_torch" || name == "minecraft:redstone_wall_torch":
		if props == nil {
			return
		}
		ax, ay, az := x, y-1, z
		if name == "minecraft:redstone_wall_torch" {
			dx, dy, dz := facingOffset(props["facing"])
			ax, ay, az = x-dx, y-dy, z-dz
		}
		// Torch turns off when its support block is powered by another source.
		supportPowered := false
		for _, off := range cardinalOffsets {
			nx, ny, nz := ax+off[0], ay+off[1], az+off[2]
			if nx == x && ny == y && nz == z {
				continue
			}
			if emittedPowerAt(w, nx, ny, nz, true) > 0 {
				supportPowered = true
				break
			}
		}
		wantLit := "true"
		if supportPowered {
			wantLit = "false"
		}
		if props["lit"] != wantLit {
			props["lit"] = wantLit
			m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		}

	case name == "minecraft:repeater" || name == "minecraft:comparator":
		if props == nil {
			return
		}
		dx, dy, dz := facingOffset(props["facing"])
		// In Java block states, a repeater's input is on its facing side (back)
		// or opposite depending on placement; check both back and general input.
		inPower := emittedPowerAt(w, x+dx, y+dy, z+dz, true)
		if inPower == 0 {
			inPower = emittedPowerAt(w, x-dx, y-dy, z-dz, true)
		}
		want := "false"
		if inPower > 0 {
			want = "true"
		}
		if props["powered"] != want {
			props["powered"] = want
			m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		}

	case name == "minecraft:piston" || name == "minecraft:sticky_piston":
		if props == nil {
			return
		}
		m.updatePiston(source, w, x, y, z, name, props, powered)

	case strings.HasSuffix(name, "_door"):
		if props == nil {
			return
		}
		want := "false"
		if powered {
			want = "true"
		}
		if props["powered"] != want {
			props["powered"] = want
			props["open"] = want
			m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
			otherY := y + 1
			if props["half"] == "upper" {
				otherY = y - 1
			}
			otherID := w.GetBlock(x, otherY, z).ID()
			if StateName(otherID) == name {
				if otherProps := StateProperties(otherID); otherProps != nil {
					otherProps["powered"] = want
					otherProps["open"] = want
					m.SetBlockAndPublish(source, x, otherY, z, BlockByID(StateIDWithProperties(name, otherProps)))
				}
			}
		}

	case strings.HasSuffix(name, "_trapdoor") || strings.HasSuffix(name, "_fence_gate"):
		if props == nil {
			return
		}
		want := "false"
		if powered {
			want = "true"
		}
		if props["powered"] != want {
			props["powered"] = want
			props["open"] = want
			m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		}

	case name == "minecraft:powered_rail" || name == "minecraft:activator_rail":
		if props == nil {
			return
		}
		want := "false"
		if powered {
			want = "true"
		}
		if props["powered"] != want {
			props["powered"] = want
			m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		}

	case name == "minecraft:hopper":
		if props == nil {
			return
		}
		wantEnabled := "true"
		if powered {
			wantEnabled = "false"
		}
		if props["enabled"] != wantEnabled {
			props["enabled"] = wantEnabled
			m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		}

	case name == "minecraft:tnt":
		if powered {
			m.SetBlockAndPublish(source, x, y, z, BlockAir{})
			m.PublishExplosion(mobs.ExplosionResult{
				X:      float64(x) + 0.5,
				Y:      float64(y) + 0.5,
				Z:      float64(z) + 0.5,
				Radius: 4.0,
			})
		}
	}
}

func isPistonImmovable(name string) bool {
	switch name {
	case "minecraft:bedrock", "minecraft:obsidian", "minecraft:crying_obsidian",
		"minecraft:respawn_anchor", "minecraft:reinforced_deepslate",
		"minecraft:enchanting_table", "minecraft:end_portal_frame",
		"minecraft:piston_head", "minecraft:moving_piston", "minecraft:barrier":
		return true
	}
	return false
}

func (m *Manager) updatePiston(source BlockUpdateSource, w *World, x, y, z int, name string, props map[string]string, powered bool) {
	facing := props["facing"]
	dx, dy, dz := facingOffset(facing)
	if dx == 0 && dy == 0 && dz == 0 {
		return
	}
	wasExtended := props["extended"] == "true"
	if powered && !wasExtended {
		// Collect up to 12 blocks in front of the piston.
		var chain [][4]int // x, y, z, stateID
		cx, cy, cz := x+dx, y+dy, z+dz
		for step := 0; step < 13; step++ {
			cid := w.GetBlock(cx, cy, cz).ID()
			if IsReplaceableBlock(cid) {
				break
			}
			cname := StateName(cid)
			if isPistonImmovable(cname) || step == 12 {
				return
			}
			if (cname == "minecraft:piston" || cname == "minecraft:sticky_piston") && StateProperties(cid)["extended"] == "true" {
				return
			}
			chain = append(chain, [4]int{cx, cy, cz, int(cid)})
			cx, cy, cz = cx+dx, cy+dy, cz+dz
		}
		// Shift pushed blocks forward from back to front.
		for i := len(chain) - 1; i >= 0; i-- {
			b := chain[i]
			m.SetBlockAndPublish(source, b[0]+dx, b[1]+dy, b[2]+dz, BlockByID(int32(b[3])))
		}
		headType := "normal"
		if name == "minecraft:sticky_piston" {
			headType = "sticky"
		}
		headProps := map[string]string{
			"facing": facing,
			"short":  "false",
			"type":   headType,
		}
		m.SetBlockAndPublish(source, x+dx, y+dy, z+dz, BlockByID(StateIDWithProperties("minecraft:piston_head", headProps)))
		props["extended"] = "true"
		m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
	} else if !powered && wasExtended {
		hx, hy, hz := x+dx, y+dy, z+dz
		if StateName(w.GetBlock(hx, hy, hz).ID()) == "minecraft:piston_head" {
			m.SetBlockAndPublish(source, hx, hy, hz, BlockAir{})
		}
		props["extended"] = "false"
		m.SetBlockAndPublish(source, x, y, z, BlockByID(StateIDWithProperties(name, props)))
		if name == "minecraft:sticky_piston" {
			px, py, pz := x+2*dx, y+2*dy, z+2*dz
			pulledID := w.GetBlock(px, py, pz).ID()
			if pulledID != AirID && !IsReplaceableBlock(pulledID) {
				pname := StateName(pulledID)
				if !isPistonImmovable(pname) {
					m.SetBlockAndPublish(source, hx, hy, hz, BlockByID(pulledID))
					m.SetBlockAndPublish(source, px, py, pz, BlockAir{})
				}
			}
		}
	}
}
