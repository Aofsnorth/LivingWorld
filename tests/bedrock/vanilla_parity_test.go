package bedrocktests

import (
	"math/rand"
	"testing"

	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/mobs"
	context "livingworld/internal/mobs/ai/context"
	aimovement "livingworld/internal/mobs/ai/movement"
	aisystems "livingworld/internal/mobs/ai/systems"
	lwworld "livingworld/internal/world"

	dfworld "github.com/df-mc/dragonfly/server/world"
)

func nonZeroDirectionBits(v any) bool {
	switch b := v.(type) {
	case int32:
		return b != 0
	case uint8:
		return b != 0
	case int:
		return b != 0
	default:
		return false
	}
}

// TestJungleVineResolvesWithVisibleDirectionBits verifies that canonical
// minecraft:vine states (both default and canopy/directional) resolve to a
// Bedrock runtime ID with non-zero vine_direction_bits so jungle vines never
// render invisible on Bedrock clients.
func TestJungleVineResolvesWithVisibleDirectionBits(t *testing.T) {
	dfworld.DefaultBlockRegistry.Finalize()
	bedrockworld.InvalidateRIDCache()

	vineID := lwworld.StateID("minecraft:vine")
	if vineID == lwworld.AirID {
		t.Fatal("minecraft:vine resolved to AirID in canonical palette")
	}
	rid, ok := bedrockworld.LivingWorldBlockIDToBedrockRID(vineID)
	if !ok {
		t.Fatalf("minecraft:vine (state=%d) failed to resolve to Bedrock RID", vineID)
	}
	name, props, found := dfworld.DefaultBlockRegistry.RuntimeIDToState(rid)
	if !found || name != "minecraft:vine" {
		t.Fatalf("resolved RID %d -> (%q, %v), want minecraft:vine", rid, name, props)
	}
	if !nonZeroDirectionBits(props["vine_direction_bits"]) {
		t.Fatalf("minecraft:vine resolved to vine_direction_bits=0 (invisible on Bedrock): %+v", props)
	}

	// Also verify canopy vine (up=true only) resolves to non-zero direction bits.
	canopyID := lwworld.StateIDWithProperties("minecraft:vine", map[string]string{
		"east":  "false",
		"north": "false",
		"south": "false",
		"up":    "true",
		"west":  "false",
	})
	canopyRID, ok := bedrockworld.LivingWorldBlockIDToBedrockRID(canopyID)
	if !ok {
		t.Fatalf("canopy vine (state=%d) failed to resolve to Bedrock RID", canopyID)
	}
	_, canopyProps, _ := dfworld.DefaultBlockRegistry.RuntimeIDToState(canopyRID)
	if !nonZeroDirectionBits(canopyProps["vine_direction_bits"]) {
		t.Fatalf("canopy vine resolved to vine_direction_bits=0 (invisible on Bedrock): %+v", canopyProps)
	}
}

// TestSolidBlockExcludesPlantsAndVines verifies that non-solid vegetation
// (short_grass, fern, dandelion, vine, water, torch, redstone_wire) does not
// report as solid to mob AI, preventing phantom jumps on grass/flowers.
func TestSolidBlockExcludesPlantsAndVines(t *testing.T) {
	nonSolid := []string{
		"minecraft:air",
		"minecraft:short_grass",
		"minecraft:tall_grass",
		"minecraft:fern",
		"minecraft:dandelion",
		"minecraft:poppy",
		"minecraft:vine",
		"minecraft:water",
		"minecraft:torch",
		"minecraft:redstone_wire",
		"minecraft:lever",
	}
	for _, name := range nonSolid {
		id := lwworld.StateID(name)
		if lwworld.IsSolidBlock(id) {
			t.Errorf("IsSolidBlock(%s [id=%d]) = true, want false", name, id)
		}
	}

	solid := []string{
		"minecraft:stone",
		"minecraft:dirt",
		"minecraft:grass_block",
		"minecraft:oak_log",
		"minecraft:redstone_lamp",
	}
	for _, name := range solid {
		id := lwworld.StateID(name)
		if !lwworld.IsSolidBlock(id) {
			t.Errorf("IsSolidBlock(%s [id=%d]) = false, want true", name, id)
		}
	}
}

// TestMobStepUpDoesNotClipIntoBlock verifies that a mob jumping onto a 1-block
// ledge at y=64 (from ground y=64.0 over y=63 onto top surface y=65.0) never
// clips down to y=64.0 inside the ledge block during ascent or descent.
func TestMobStepUpDoesNotClipIntoBlock(t *testing.T) {
	// Ground at y=63 everywhere; a 1-block step at x>=1, y=64.
	solidAt := func(x, y, z int) bool {
		if y <= 63 {
			return true
		}
		if x >= 1 && y == 64 {
			return true
		}
		return false
	}
	ctx := &context.AIContext{
		RNG:     rand.New(rand.NewSource(42)),
		SolidAt: solidAt,
	}
	def := mobs.DefFor("minecraft:cow")
	m := &mobs.Mob{
		Type:     "minecraft:cow",
		X:        0.8,
		Y:        64.0,
		Z:        0.5,
		OnGround: true,
	}

	for tick := 0; tick < 30; tick++ {
		if m.JumpCooldown > 0 {
			m.JumpCooldown--
		}
		aisystems.ApplyGravity(m, def, ctx)
		aimovement.StepHorizontal(m, def, ctx, 1.0, 0.0, 0.25, true)
		if m.X >= 1.0 && m.Y < 65.0 {
			t.Fatalf("tick %d: mob clipped inside step block at X=%.2f, Y=%.2f (want Y >= 65.0)", tick, m.X, m.Y)
		}
	}
	if m.X < 1.0 || m.Y != 65.0 || !m.OnGround {
		t.Fatalf("mob did not finish stepping onto block: X=%.2f, Y=%.2f, OnGround=%v", m.X, m.Y, m.OnGround)
	}
}

// TestRedstoneLeverWireLampAndPiston verifies that toggling a lever powers
// connected redstone wire, lights a redstone lamp, and extends/retracts a piston.
func TestRedstoneLeverWireLampAndPiston(t *testing.T) {
	wm := lwworld.NewManager()
	w := wm.GetDefaultWorld()

	// Place a lever at (0, 64, 0), redstone_wire at (1, 64, 0), redstone_lamp at (2, 64, 0),
	// and a piston facing east at (1, 64, 1) pushing stone at (2, 64, 1).
	w.SetBlock(0, 64, 0, lwworld.BlockByID(lwworld.StateIDWithProperties("minecraft:lever", map[string]string{
		"face":    "floor",
		"facing":  "north",
		"powered": "false",
	})))
	w.SetBlock(1, 64, 0, lwworld.BlockByID(lwworld.StateID("minecraft:redstone_wire")))
	w.SetBlock(2, 64, 0, lwworld.BlockByID(lwworld.StateIDWithProperties("minecraft:redstone_lamp", map[string]string{
		"lit": "false",
	})))
	w.SetBlock(1, 64, 1, lwworld.BlockByID(lwworld.StateIDWithProperties("minecraft:sticky_piston", map[string]string{
		"extended": "false",
		"facing":   "east",
	})))
	w.SetBlock(2, 64, 1, lwworld.BlockByID(lwworld.StateID("minecraft:stone")))

	// Toggle lever ON.
	if !wm.InteractBlock(lwworld.BlockUpdateSourceJava, 0, 64, 0) {
		t.Fatal("InteractBlock on lever returned false")
	}

	wireProps := lwworld.StateProperties(w.GetBlock(1, 64, 0).ID())
	if wireProps["power"] != "15" {
		t.Fatalf("redstone_wire power = %q, want 15", wireProps["power"])
	}
	lampProps := lwworld.StateProperties(w.GetBlock(2, 64, 0).ID())
	if lampProps["lit"] != "true" {
		t.Fatalf("redstone_lamp lit = %q, want true", lampProps["lit"])
	}
	pistonProps := lwworld.StateProperties(w.GetBlock(1, 64, 1).ID())
	if pistonProps["extended"] != "true" {
		t.Fatalf("sticky_piston extended = %q, want true", pistonProps["extended"])
	}
	if got := lwworld.StateName(w.GetBlock(2, 64, 1).ID()); got != "minecraft:piston_head" {
		t.Fatalf("block at (2,64,1) = %q, want minecraft:piston_head", got)
	}
	if got := lwworld.StateName(w.GetBlock(3, 64, 1).ID()); got != "minecraft:stone" {
		t.Fatalf("pushed block at (3,64,1) = %q, want minecraft:stone", got)
	}

	// Toggle lever OFF -> wire depowers, lamp turns off, sticky piston retracts stone back to (2,64,1).
	wm.InteractBlock(lwworld.BlockUpdateSourceJava, 0, 64, 0)
	if got := lwworld.StateProperties(w.GetBlock(2, 64, 0).ID())["lit"]; got != "false" {
		t.Fatalf("redstone_lamp lit after lever off = %q, want false", got)
	}
	if got := lwworld.StateName(w.GetBlock(2, 64, 1).ID()); got != "minecraft:stone" {
		t.Fatalf("retracted block at (2,64,1) = %q, want minecraft:stone", got)
	}
}
