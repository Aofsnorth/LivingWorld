package world

// hasHeadroom reports a 2-block air gap at (x,y,z) so a mob fits.
func hasHeadroom(w *World, x, y, z int) bool {
	return w.GetBlock(x, y, z).ID() == AirID && w.GetBlock(x, y+1, z).ID() == AirID
}

// hasOpenSky returns true if the cell column above (x, z) is
// open air for at least openSkyCheckCells blocks. Used by the
// ghast rule.
func hasOpenSky(w *World, x, y, z int) bool {
	for i := 1; i <= openSkyCheckCells; i++ {
		if w.GetBlock(x, y+i, z).ID() != AirID {
			return false
		}
	}
	return true
}

// lightLevel returns the max of sky and block light at (x, y, z).
// 0..15.
func lightLevel(w *World, x, y, z int) uint8 {
	sky := w.GetSkyLight(x, y, z)
	block := w.GetBlockLight(x, y, z)
	if sky > block {
		return sky
	}
	return block
}

// isNight reports whether dayTime is in the hostile-spawn
// window (dusk→dawn).
func isNight(dayTime int64) bool {
	t := dayTime % 24000
	return t >= 13000 && t < 23000
}

// isDay reports whether dayTime is in the passive-spawn
// window (sunrise→dusk).
func isDay(dayTime int64) bool {
	return !isNight(dayTime)
}

// surfaceName is the namespaced block at (x, y-1, z) — the
// block the mob will stand on. We compare against the rule's
// Surfaces whitelist.
func surfaceName(w *World, x, y, z int) string {
	return StateName(w.GetBlock(x, y-1, z).ID())
}
