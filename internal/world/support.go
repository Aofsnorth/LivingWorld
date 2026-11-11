package world

import "strings"

// propagateSupportUpdates evaluates block support rules around (x, y, z) and
// cascades updates to connected blocks (such as vines, sugar cane, bamboo, kelp,
// cactus, chorus plants, hanging blocks, doors, tall plants, beds, torches, signs,
// ladders, rails, redstone components, and plants) so no unsupported block remains
// floating in mid-air after a neighbor is broken or changed.
func (m *Manager) propagateSupportUpdates(source BlockUpdateSource, w *World, x, y, z int) {
	if w == nil {
		return
	}
	type pos3 struct{ x, y, z int }
	queue := make([]pos3, 0, 32)
	queued := make(map[pos3]struct{}, 32)

	enqueue := func(px, py, pz int) {
		p := pos3{px, py, pz}
		if _, ok := queued[p]; ok {
			return
		}
		queued[p] = struct{}{}
		queue = append(queue, p)
	}

	enqueue(x, y, z)

	const maxSteps = 2048
	steps := 0
	dirs := [6][3]int{
		{0, -1, 0},
		{0, 1, 0},
		{0, 0, -1},
		{0, 0, 1},
		{-1, 0, 0},
		{1, 0, 0},
	}

	for len(queue) > 0 && steps < maxSteps {
		curr := queue[0]
		queue = queue[1:]
		delete(queued, curr)
		steps++

		if m.updateBlockSupport(source, w, curr.x, curr.y, curr.z) {
			enqueue(curr.x, curr.y, curr.z)
		}
		for _, d := range dirs {
			nx, ny, nz := curr.x+d[0], curr.y+d[1], curr.z+d[2]
			if m.updateBlockSupport(source, w, nx, ny, nz) {
				enqueue(nx, ny, nz)
			}
		}
	}
}

// updateBlockSupport checks whether the block at (bx, by, bz) still satisfies its
// vanilla placement/support constraints. If a multi-face block (like vine, glow_lichen,
// or sculk_vein) loses some of its attached faces, its block state properties are updated;
// if a block loses all valid support, it breaks into air (and drops natural loot when
// appropriate). Returns true if the block state at (bx, by, bz) changed.
func (m *Manager) updateBlockSupport(source BlockUpdateSource, w *World, bx, by, bz int) bool {
	if by < MinWorldHeight || by >= MinWorldHeight+MaxWorldHeight {
		return false
	}
	id := w.GetBlock(bx, by, bz).ID()
	if IsAir(id) || IsFluidBlockID(id) {
		return false
	}
	name := StateName(id)
	if name == "" {
		return false
	}
	props := StateProperties(id)

	// 1. Vines (minecraft:vine):
	// Each horizontal face (north, south, east, west) can be supported EITHER by a solid
	// neighbor block in that direction OR by a vine block directly above (y+1) that has
	// that same horizontal face == "true". The "up" face requires a solid block at y+1.
	if name == "minecraft:vine" {
		aboveID := w.GetBlock(bx, by+1, bz).ID()
		aboveIsVine := StateName(aboveID) == "minecraft:vine"
		var aboveProps map[string]string
		if aboveIsVine {
			aboveProps = StateProperties(aboveID)
		}

		newProps := cloneProps(props)
		changed := false

		if newProps["up"] == "true" && !isFullSupportBlock(aboveID) {
			newProps["up"] = "false"
			changed = true
		}

		horizFaces := [4]struct {
			face   string
			dx, dz int
		}{
			{"north", 0, -1},
			{"south", 0, 1},
			{"west", -1, 0},
			{"east", 1, 0},
		}

		anyFace := newProps["up"] == "true"
		for _, hf := range horizFaces {
			if newProps[hf.face] != "true" {
				continue
			}
			neighborID := w.GetBlock(bx+hf.dx, by, bz+hf.dz).ID()
			supportedByWall := isFullSupportBlock(neighborID)
			supportedByVineAbove := aboveIsVine && aboveProps[hf.face] == "true"
			if !supportedByWall && !supportedByVineAbove {
				newProps[hf.face] = "false"
				changed = true
			} else {
				anyFace = true
			}
		}

		if !anyFace {
			m.breakUnsupportedBlock(source, bx, by, bz, id, false)
			return true
		}
		if changed {
			newID := StateIDWithProperties(name, newProps)
			if newID != id {
				m.SetBlockAndPublish(source, bx, by, bz, BlockByID(newID))
				return true
			}
		}
		return false
	}

	// 2. Multiface blocks (glow_lichen, sculk_vein, resin_clump):
	if name == "minecraft:glow_lichen" || name == "minecraft:sculk_vein" || name == "minecraft:resin_clump" {
		faces := [6]struct {
			face       string
			dx, dy, dz int
		}{
			{"down", 0, -1, 0},
			{"up", 0, 1, 0},
			{"north", 0, 0, -1},
			{"south", 0, 0, 1},
			{"west", -1, 0, 0},
			{"east", 1, 0, 0},
		}
		newProps := cloneProps(props)
		changed := false
		anyFace := false
		for _, f := range faces {
			if newProps[f.face] != "true" {
				continue
			}
			nID := w.GetBlock(bx+f.dx, by+f.dy, bz+f.dz).ID()
			if !isFullSupportBlock(nID) {
				newProps[f.face] = "false"
				changed = true
			} else {
				anyFace = true
			}
		}
		if !anyFace {
			m.breakUnsupportedBlock(source, bx, by, bz, id, false)
			return true
		}
		if changed {
			newID := StateIDWithProperties(name, newProps)
			if newID != id {
				m.SetBlockAndPublish(source, bx, by, bz, BlockByID(newID))
				return true
			}
		}
		return false
	}

	// 3. Downward-hanging chains & plants (require support at y+1):
	switch name {
	case "minecraft:weeping_vines", "minecraft:weeping_vines_plant":
		aboveID := w.GetBlock(bx, by+1, bz).ID()
		aboveName := StateName(aboveID)
		if aboveName != "minecraft:weeping_vines" && aboveName != "minecraft:weeping_vines_plant" && !isFullSupportBlock(aboveID) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	case "minecraft:cave_vines", "minecraft:cave_vines_plant":
		aboveID := w.GetBlock(bx, by+1, bz).ID()
		aboveName := StateName(aboveID)
		if aboveName != "minecraft:cave_vines" && aboveName != "minecraft:cave_vines_plant" && !isFullSupportBlock(aboveID) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	case "minecraft:pale_hanging_moss":
		aboveID := w.GetBlock(bx, by+1, bz).ID()
		aboveName := StateName(aboveID)
		if aboveName != "minecraft:pale_hanging_moss" && !isFullSupportBlock(aboveID) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, false)
			return true
		}
		return false
	case "minecraft:hanging_roots", "minecraft:spore_blossom":
		aboveID := w.GetBlock(bx, by+1, bz).ID()
		if !isFullSupportBlock(aboveID) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, name == "minecraft:spore_blossom")
			return true
		}
		return false
	case "minecraft:pointed_dripstone":
		dir := props["vertical_direction"]
		if dir == "down" {
			aboveID := w.GetBlock(bx, by+1, bz).ID()
			if StateName(aboveID) != "minecraft:pointed_dripstone" && !isFullSupportBlock(aboveID) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		} else {
			belowID := w.GetBlock(bx, by-1, bz).ID()
			if StateName(belowID) != "minecraft:pointed_dripstone" && !isFullSupportBlock(belowID) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		}
		return false
	}

	// 4. Upward-growing columns & plants (require support at y-1):
	switch name {
	case "minecraft:sugar_cane":
		belowID := w.GetBlock(bx, by-1, bz).ID()
		belowName := StateName(belowID)
		if belowName != "minecraft:sugar_cane" && !isSoilOrSandBlock(belowName) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	case "minecraft:cactus":
		belowID := w.GetBlock(bx, by-1, bz).ID()
		belowName := StateName(belowID)
		if belowName != "minecraft:cactus" && belowName != "minecraft:sand" && belowName != "minecraft:red_sand" && belowName != "minecraft:suspicious_sand" {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		// Cactus also breaks if any horizontal neighbor is solid or cactus
		for _, d := range [4][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
			nID := w.GetBlock(bx+d[0], by, bz+d[1]).ID()
			if isFullSupportBlock(nID) || StateName(nID) == "minecraft:cactus" {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		}
		return false
	case "minecraft:cactus_flower":
		belowID := w.GetBlock(bx, by-1, bz).ID()
		belowName := StateName(belowID)
		if belowName != "minecraft:cactus" && !isFullSupportBlock(belowID) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	case "minecraft:bamboo", "minecraft:bamboo_sapling":
		belowID := w.GetBlock(bx, by-1, bz).ID()
		belowName := StateName(belowID)
		if belowName != "minecraft:bamboo" && belowName != "minecraft:bamboo_sapling" && !isBambooSupportBlock(belowName) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	case "minecraft:kelp", "minecraft:kelp_plant":
		belowID := w.GetBlock(bx, by-1, bz).ID()
		belowName := StateName(belowID)
		if belowName != "minecraft:kelp" && belowName != "minecraft:kelp_plant" && !isFullSupportBlock(belowID) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	case "minecraft:twisting_vines", "minecraft:twisting_vines_plant":
		belowID := w.GetBlock(bx, by-1, bz).ID()
		belowName := StateName(belowID)
		if belowName != "minecraft:twisting_vines" && belowName != "minecraft:twisting_vines_plant" && !isFullSupportBlock(belowID) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	case "minecraft:big_dripleaf", "minecraft:big_dripleaf_stem":
		belowID := w.GetBlock(bx, by-1, bz).ID()
		belowName := StateName(belowID)
		if belowName != "minecraft:big_dripleaf_stem" && !isSupportBelow(belowID) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		if name == "minecraft:big_dripleaf_stem" {
			aboveName := StateName(w.GetBlock(bx, by+1, bz).ID())
			if aboveName != "minecraft:big_dripleaf" && aboveName != "minecraft:big_dripleaf_stem" {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		}
		return false
	case "minecraft:chorus_flower":
		belowID := w.GetBlock(bx, by-1, bz).ID()
		belowName := StateName(belowID)
		if belowName == "minecraft:end_stone" || belowName == "minecraft:chorus_plant" {
			return false
		}
		// Can also be supported horizontally by exactly one chorus_plant
		horizChorus := 0
		for _, d := range [4][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
			nName := StateName(w.GetBlock(bx+d[0], by, bz+d[1]).ID())
			if nName == "minecraft:chorus_plant" {
				horizChorus++
			}
		}
		if horizChorus != 1 {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	case "minecraft:chorus_plant":
		belowID := w.GetBlock(bx, by-1, bz).ID()
		belowName := StateName(belowID)
		supported := belowName == "minecraft:end_stone" || belowName == "minecraft:chorus_plant"
		if !supported {
			for _, d := range [4][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
				nName := StateName(w.GetBlock(bx+d[0], by, bz+d[1]).ID())
				if nName == "minecraft:chorus_plant" {
					nbName := StateName(w.GetBlock(bx+d[0], by-1, bz+d[1]).ID())
					if nbName == "minecraft:chorus_plant" || nbName == "minecraft:end_stone" {
						supported = true
						break
					}
				}
			}
		}
		if !supported {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	}

	// 5. Two-block vertical structures: Doors & Double Plants (half == "lower" / "upper")
	if isDoorBlockName(name) || isDoublePlantBlockName(name) {
		half := props["half"]
		if half == "upper" {
			belowID := w.GetBlock(bx, by-1, bz).ID()
			if StateName(belowID) != name || StateProperties(belowID)["half"] != "lower" {
				// Do not double-drop loot when the upper half breaks because the lower half broke
				m.breakUnsupportedBlock(source, bx, by, bz, id, false)
				return true
			}
		} else if half == "lower" {
			belowID := w.GetBlock(bx, by-1, bz).ID()
			if !isSupportBelow(belowID) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
			aboveID := w.GetBlock(bx, by+1, bz).ID()
			if StateName(aboveID) != name || StateProperties(aboveID)["half"] != "upper" {
				// Upper half was broken by player -> remove lower half without duplicate drop
				m.breakUnsupportedBlock(source, bx, by, bz, id, false)
				return true
			}
		}
		return false
	}

	// 6. Two-block horizontal structures: Beds (part == "foot" / "head")
	if strings.HasSuffix(name, "_bed") {
		part := props["part"]
		dx, _, dz := facingOffset(props["facing"])
		if dx == 0 && dz == 0 {
			return false
		}
		if part == "foot" {
			headID := w.GetBlock(bx+dx, by, bz+dz).ID()
			if StateName(headID) != name || StateProperties(headID)["part"] != "head" {
				m.breakUnsupportedBlock(source, bx, by, bz, id, false)
				return true
			}
		} else if part == "head" {
			footID := w.GetBlock(bx-dx, by, bz-dz).ID()
			if StateName(footID) != name || StateProperties(footID)["part"] != "foot" {
				m.breakUnsupportedBlock(source, bx, by, bz, id, false)
				return true
			}
		}
		return false
	}

	// 7. Wall-mounted blocks (attached to the block behind their "facing" direction):
	if isWallMountedFacingAway(name) {
		dx, _, dz := facingOffset(props["facing"])
		if dx != 0 || dz != 0 {
			supportID := w.GetBlock(bx-dx, by, bz-dz).ID()
			if !isFullSupportBlock(supportID) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		}
		return false
	}

	// 8. Cocoa pods (attached to jungle log/wood in front of their "facing" direction):
	if name == "minecraft:cocoa" {
		dx, _, dz := facingOffset(props["facing"])
		if dx != 0 || dz != 0 {
			supportName := StateName(w.GetBlock(bx+dx, by, bz+dz).ID())
			if !strings.Contains(supportName, "jungle_log") && !strings.Contains(supportName, "jungle_wood") {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		}
		return false
	}

	// 9. Face-mounted switches (lever, *_button) & grindstone:
	if name == "minecraft:lever" || strings.HasSuffix(name, "_button") || name == "minecraft:grindstone" {
		face := props["face"]
		switch face {
		case "floor":
			if !isSupportBelow(w.GetBlock(bx, by-1, bz).ID()) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		case "ceiling":
			if !isFullSupportBlock(w.GetBlock(bx, by+1, bz).ID()) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		case "wall":
			dx, _, dz := facingOffset(props["facing"])
			if (dx != 0 || dz != 0) && !isFullSupportBlock(w.GetBlock(bx-dx, by, bz-dz).ID()) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		}
		return false
	}

	// 10. Bell (attachment == "floor" / "ceiling" / "single_wall" / "double_wall"):
	if name == "minecraft:bell" {
		switch props["attachment"] {
		case "floor":
			if !isSupportBelow(w.GetBlock(bx, by-1, bz).ID()) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		case "ceiling":
			aboveID := w.GetBlock(bx, by+1, bz).ID()
			if IsAir(aboveID) || IsFluidBlockID(aboveID) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		case "single_wall", "double_wall":
			dx, _, dz := facingOffset(props["facing"])
			if (dx != 0 || dz != 0) && !isFullSupportBlock(w.GetBlock(bx+dx, by, bz+dz).ID()) && !isFullSupportBlock(w.GetBlock(bx-dx, by, bz-dz).ID()) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		}
		return false
	}

	// 11. Amethyst clusters & buds (6-directional facing):
	if name == "minecraft:amethyst_cluster" || strings.HasSuffix(name, "_amethyst_bud") {
		dx, dy, dz := facingOffset(props["facing"])
		if (dx != 0 || dy != 0 || dz != 0) && !isFullSupportBlock(w.GetBlock(bx-dx, by-dy, bz-dz).ID()) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	}

	// 12. Lanterns & Ceiling Hanging Signs:
	if name == "minecraft:lantern" || name == "minecraft:soul_lantern" {
		if props["hanging"] == "true" {
			aboveID := w.GetBlock(bx, by+1, bz).ID()
			if IsAir(aboveID) || IsFluidBlockID(aboveID) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		} else {
			if !isSupportBelow(w.GetBlock(bx, by-1, bz).ID()) {
				m.breakUnsupportedBlock(source, bx, by, bz, id, true)
				return true
			}
		}
		return false
	}
	if strings.HasSuffix(name, "_hanging_sign") && !strings.HasSuffix(name, "_wall_hanging_sign") {
		aboveID := w.GetBlock(bx, by+1, bz).ID()
		if IsAir(aboveID) || IsFluidBlockID(aboveID) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	}

	// 13. Floor-supported blocks (torches, redstone, rails, carpets, snow, pressure plates,
	// standing signs/banners, candles, flower pots, crops, saplings, flowers, grass/ferns, etc.):
	if requiresFloorSupport(name) {
		belowID := w.GetBlock(bx, by-1, bz).ID()
		if !isSupportBelow(belowID) {
			m.breakUnsupportedBlock(source, bx, by, bz, id, true)
			return true
		}
		return false
	}

	return false
}

func (m *Manager) breakUnsupportedBlock(source BlockUpdateSource, bx, by, bz int, id int32, dropLoot bool) {
	m.PublishBlockDestroy(source, [16]byte{}, bx, by, bz, id)
	if dropLoot {
		m.DropBlockLoot(id, bx, by, bz)
	}
	m.SetBlockAndPublish(source, bx, by, bz, BlockAir{})
}

func cloneProps(props map[string]string) map[string]string {
	out := make(map[string]string, len(props))
	for k, v := range props {
		out[k] = v
	}
	return out
}

func isFullSupportBlock(id int32) bool {
	if IsAir(id) || IsFluidBlockID(id) {
		return false
	}
	name := StateName(id)
	if name == "minecraft:vine" || name == "minecraft:glow_lichen" || name == "minecraft:sculk_vein" || name == "minecraft:resin_clump" {
		return false
	}
	return IsSolidBlock(id)
}

func isSupportBelow(belowID int32) bool {
	if IsAir(belowID) || IsFluidBlockID(belowID) {
		return false
	}
	name := StateName(belowID)
	if IsSolidBlock(belowID) {
		return true
	}
	// Farmland, dirt paths, mud, soul sand, and top slabs/stairs can support floor items/plants
	switch name {
	case "minecraft:farmland", "minecraft:dirt_path", "minecraft:mud", "minecraft:soul_sand",
		"minecraft:Glass", "minecraft:glass", "minecraft:ice", "minecraft:packed_ice", "minecraft:blue_ice",
		"minecraft:glowstone", "minecraft:sea_lantern", "minecraft:observer", "minecraft:piston", "minecraft:sticky_piston",
		"minecraft:hopper", "minecraft:scaffolding":
		return true
	}
	if strings.HasSuffix(name, "_slab") {
		t := StateProperties(belowID)["type"]
		return t == "top" || t == "double"
	}
	if strings.HasSuffix(name, "_stairs") {
		return StateProperties(belowID)["half"] == "top" || IsSolidBlock(belowID)
	}
	if strings.HasSuffix(name, "_fence") || strings.HasSuffix(name, "_wall") || strings.HasSuffix(name, "_glass") {
		return true
	}
	return false
}

func isSoilOrSandBlock(name string) bool {
	switch name {
	case "minecraft:grass_block", "minecraft:dirt", "minecraft:coarse_dirt", "minecraft:podzol",
		"minecraft:rooted_dirt", "minecraft:mycelium", "minecraft:moss_block", "minecraft:pale_moss_block",
		"minecraft:mud", "minecraft:muddy_mangrove_roots", "minecraft:sand", "minecraft:red_sand", "minecraft:suspicious_sand",
		"minecraft:farmland":
		return true
	}
	return false
}

func isBambooSupportBlock(name string) bool {
	if isSoilOrSandBlock(name) {
		return true
	}
	return name == "minecraft:gravel" || name == "minecraft:suspicious_gravel"
}

func isDoorBlockName(name string) bool {
	return strings.HasSuffix(name, "_door") && !strings.HasSuffix(name, "_trapdoor")
}

func isDoublePlantBlockName(name string) bool {
	switch name {
	case "minecraft:tall_grass", "minecraft:large_fern", "minecraft:sunflower", "minecraft:lilac",
		"minecraft:rose_bush", "minecraft:peony", "minecraft:tall_seagrass", "minecraft:pitcher_plant",
		"minecraft:small_dripleaf":
		return true
	}
	return false
}

func isWallMountedFacingAway(name string) bool {
	switch name {
	case "minecraft:wall_torch", "minecraft:soul_wall_torch", "minecraft:redstone_wall_torch",
		"minecraft:ladder", "minecraft:tripwire_hook":
		return true
	}
	return strings.HasSuffix(name, "_wall_sign") ||
		strings.HasSuffix(name, "_wall_banner") ||
		strings.HasSuffix(name, "_wall_fan") ||
		strings.HasSuffix(name, "_wall_head") ||
		strings.HasSuffix(name, "_wall_skull")
}

func requiresFloorSupport(name string) bool {
	switch name {
	case "minecraft:torch", "minecraft:soul_torch", "minecraft:redstone_torch",
		"minecraft:redstone_wire", "minecraft:repeater", "minecraft:comparator",
		"minecraft:rail", "minecraft:powered_rail", "minecraft:detector_rail", "minecraft:activator_rail",
		"minecraft:snow", "minecraft:flower_pot", "minecraft:cake", "minecraft:SEA_PICKLE", "minecraft:sea_pickle",
		"minecraft:lily_pad", "minecraft:wheat", "minecraft:carrots", "minecraft:potatoes", "minecraft:beetroots",
		"minecraft:melon_stem", "minecraft:attached_melon_stem", "minecraft:pumpkin_stem", "minecraft:attached_pumpkin_stem",
		"minecraft:torchflower_crop", "minecraft:pitcher_crop", "minecraft:nether_wart", "minecraft:sweet_berry_bush",
		"minecraft:short_grass", "minecraft:fern", "minecraft:dead_bush", "minecraft:bush", "minecraft:firefly_bush",
		"minecraft:short_dry_grass", "minecraft:tall_dry_grass", "minecraft:seagrass",
		"minecraft:crimson_roots", "minecraft:warped_roots", "minecraft:nether_sprouts",
		"minecraft:crimson_fungus", "minecraft:warped_fungus", "minecraft:brown_mushroom", "minecraft:red_mushroom",
		"minecraft:dandelion", "minecraft:poppy", "minecraft:blue_orchid", "minecraft:allium", "minecraft:azure_bluet",
		"minecraft:red_tulip", "minecraft:orange_tulip", "minecraft:white_tulip", "minecraft:pink_tulip",
		"minecraft:oxeye_daisy", "minecraft:cornflower", "minecraft:lily_of_the_valley", "minecraft:wither_rose",
		"minecraft:torchflower", "minecraft:open_eyeblossom", "minecraft:closed_eyeblossom",
		"minecraft:pink_petals", "minecraft:wildflowers", "minecraft:leaf_litter",
		"minecraft:heavy_core", "minecraft:turtle_egg", "minecraft:sniffer_egg", "minecraft:frogspawn":
		return true
	}
	if strings.HasPrefix(name, "minecraft:potted_") {
		return true
	}
	if strings.HasSuffix(name, "_carpet") ||
		strings.HasSuffix(name, "_pressure_plate") ||
		strings.HasSuffix(name, "_sapling") ||
		strings.HasSuffix(name, "_propagule") ||
		strings.HasSuffix(name, "_candle") || name == "minecraft:candle" ||
		strings.HasSuffix(name, "_candle_cake") || name == "minecraft:candle_cake" ||
		strings.HasSuffix(name, "_coral") || strings.HasSuffix(name, "_coral_fan") {
		return true
	}
	if strings.HasSuffix(name, "_sign") && !strings.HasSuffix(name, "_wall_sign") && !strings.HasSuffix(name, "_hanging_sign") {
		return true
	}
	if strings.HasSuffix(name, "_banner") && !strings.HasSuffix(name, "_wall_banner") {
		return true
	}
	return false
}
