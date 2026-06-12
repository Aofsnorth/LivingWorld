package pipeline

import (
	"livingworld/internal/world"
)

// carveCaves cuts cheese caves (big rooms via 3D noise threshold) and
// spaghetti caves (winding tunnels where two independent 3D fields are
// simultaneously near zero) out of the chunk. Carving runs AFTER the
// surface pass so cave mouths show through hillsides, and BEFORE ores
// so veins can be exposed on cave walls.
//
// Two guards keep the old "craters in a flat world" failure mode out:
//   - underwater columns only carve well below the sea floor, so the
//     ocean doesn't drain into a fresh chasm;
//   - the bedrock floor band is never carved.
func (g *Generator) carveCaves(c *world.Chunk, cols *[16][16]column, cx, cz int) {
	for lz := 0; lz < 16; lz++ {
		for lx := 0; lx < 16; lx++ {
			col := cols[lz][lx]
			wx, wz := cx*16+lx, cz*16+lz
			fx, fz := float64(wx), float64(wz)

			// Land columns may carve right up to the surface (cave
			// entrances); sea-floor columns keep a 10-block roof.
			yTop := col.height
			if col.height < seaLevel-1 {
				yTop = col.height - 10
			}

			for y := minY + 6; y <= yTop; y++ {
				fy := float64(y)

				carve := false
				// Cheese caves: big rooms, fading out above y≈40 so the
				// surface stays intact.
				cheese := g.cheese.at3(fx, fy, fz, 1.6)
				thr := 0.50
				if y > 40 {
					thr += float64(y-40) * 0.013
				}
				if cheese > thr {
					carve = true
				}
				if !carve {
					// Spaghetti tunnels. Near the surface the window
					// narrows so entrances are mouth-sized, not craters.
					win := 0.062
					if y > col.height-12 {
						win = 0.034
					}
					s1 := g.spaghetti1.at3(fx, fy, fz, 1.4)
					if s1 < win && s1 > -win {
						s2 := g.spaghetti2.at3(fx, fy, fz, 1.4)
						if s2 < win && s2 > -win {
							carve = true
						}
					}
				}
				if !carve {
					continue
				}
				cur := c.GetBlock(lx, y, lz).ID()
				if cur == world.AirID || cur == g.ids.bedrock || cur == g.ids.water {
					continue
				}
				if y < lavaLevel {
					c.SetBlock(lx, y, lz, world.BlockByID(g.ids.lava))
				} else {
					c.SetBlock(lx, y, lz, world.BlockAir{})
				}
			}
		}
	}
}
