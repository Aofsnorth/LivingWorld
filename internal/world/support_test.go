package world

import "testing"

func TestVineSupportCascade(t *testing.T) {
	wm := NewManager()
	w := wm.GetDefaultWorld()

	stone := BlockByName("minecraft:stone")
	// Place stone wall at (0, 70, -1) (north of (0, 70, 0)), so only the top vine at Y=70
	// touches a solid wall; the lower vines at Y=69, 68, 67 hang from the vine above.
	w.SetBlock(0, 70, -1, stone)

	vineNorthID := StateIDWithProperties("minecraft:vine", map[string]string{
		"north": "true",
		"south": "false",
		"east":  "false",
		"west":  "false",
		"up":    "false",
	})
	vineNorth := BlockByID(vineNorthID)

	for y := 70; y >= 67; y-- {
		w.SetBlock(0, y, 0, vineNorth)
	}

	// Break the top vine at (0, 70, 0): all lower vines at Y=69, 68, 67 must cascade to air.
	wm.SetBlockAndPublish(BlockUpdateSourceJava, 0, 70, 0, BlockAir{})
	wm.OnBlockChanged(BlockUpdateSourceJava, 0, 70, 0)

	for y := 70; y >= 67; y-- {
		got := StateName(w.GetBlock(0, y, 0).ID())
		if got != "minecraft:air" {
			t.Fatalf("vine at (0, %d, 0) after breaking top vine = %q, want minecraft:air (no floating vines)", y, got)
		}
	}
}

func TestVineMultiFaceSupport(t *testing.T) {
	wm := NewManager()
	w := wm.GetDefaultWorld()

	stone := BlockByName("minecraft:stone")
	w.SetBlock(10, 70, 9, stone)  // north of (10, 70, 10)
	w.SetBlock(11, 70, 10, stone) // east of (10, 70, 10)

	vineBothID := StateIDWithProperties("minecraft:vine", map[string]string{
		"north": "true",
		"south": "false",
		"east":  "true",
		"west":  "false",
		"up":    "false",
	})
	w.SetBlock(10, 70, 10, BlockByID(vineBothID))

	// Break the north wall: vine should stay alive attached only to east.
	wm.SetBlockAndPublish(BlockUpdateSourceJava, 10, 70, 9, BlockAir{})
	wm.OnBlockChanged(BlockUpdateSourceJava, 10, 70, 9)

	idAfterNorthBreak := w.GetBlock(10, 70, 10).ID()
	if StateName(idAfterNorthBreak) != "minecraft:vine" {
		t.Fatalf("block at (10,70,10) = %q, want minecraft:vine still attached to east wall", StateName(idAfterNorthBreak))
	}
	props := StateProperties(idAfterNorthBreak)
	if props["north"] != "false" || props["east"] != "true" {
		t.Fatalf("vine props = %+v, want north=false, east=true", props)
	}

	// Break the remaining east wall: vine must now break completely into air.
	wm.SetBlockAndPublish(BlockUpdateSourceJava, 11, 70, 10, BlockAir{})
	wm.OnBlockChanged(BlockUpdateSourceJava, 11, 70, 10)

	if got := StateName(w.GetBlock(10, 70, 10).ID()); got != "minecraft:air" {
		t.Fatalf("block at (10,70,10) after losing all walls = %q, want minecraft:air", got)
	}
}

func TestColumnAndWallMountedSupportCascade(t *testing.T) {
	wm := NewManager()
	w := wm.GetDefaultWorld()

	// 1. Sugar cane column at (20, 65..67, 20) on sand at (20, 64, 20)
	w.SetBlock(20, 64, 20, BlockByName("minecraft:sand"))
	cane := BlockByName("minecraft:sugar_cane")
	for y := 65; y <= 67; y++ {
		w.SetBlock(20, y, 20, cane)
	}
	// Break base sugar cane at Y=65 -> Y=66 and Y=67 must cascade to air
	wm.SetBlockAndPublish(BlockUpdateSourceJava, 20, 65, 20, BlockAir{})
	wm.OnBlockChanged(BlockUpdateSourceJava, 20, 65, 20)
	for y := 65; y <= 67; y++ {
		if got := StateName(w.GetBlock(20, y, 20).ID()); got != "minecraft:air" {
			t.Fatalf("sugar_cane at Y=%d = %q, want minecraft:air", y, got)
		}
	}

	// 2. Wall torch attached to east face of stone at (30, 65, 30) -> torch at (31, 65, 30)
	w.SetBlock(30, 65, 30, BlockByName("minecraft:stone"))
	torchID := StateIDWithProperties("minecraft:wall_torch", map[string]string{"facing": "east"})
	w.SetBlock(31, 65, 30, BlockByID(torchID))

	wm.SetBlockAndPublish(BlockUpdateSourceJava, 30, 65, 30, BlockAir{})
	wm.OnBlockChanged(BlockUpdateSourceJava, 30, 65, 30)
	if got := StateName(w.GetBlock(31, 65, 30).ID()); got != "minecraft:air" {
		t.Fatalf("wall_torch at (31,65,30) after breaking wall = %q, want minecraft:air", got)
	}
}
