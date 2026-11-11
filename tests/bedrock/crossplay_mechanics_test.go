package bedrocktests

import (
	"testing"

	"github.com/google/uuid"

	"livingworld/config"
	"livingworld/internal/combat"
	"livingworld/internal/item"
	"livingworld/internal/player"
	"livingworld/internal/registry"
	"livingworld/internal/world"
)

type envRecorderController struct {
	syncCount   int
	hurtCount   int
	lastDamage  float32
	lastAir     int16
	lastFire    bool
	lastSwim    bool
	lastBreathe bool
}

func (c *envRecorderController) SendMessage(string)                {}
func (c *envRecorderController) Kick(string)                       {}
func (c *envRecorderController) Push(vx, vy, vz float64)           {}
func (c *envRecorderController) Hurt(amount float32)               { c.hurtCount++; c.lastDamage += amount }
func (c *envRecorderController) SyncEnvironment(snap player.PlayerSnapshot) {
	c.syncCount++
	c.lastAir = snap.AirSupply
	c.lastFire = snap.OnFire
	c.lastSwim = snap.Swimming
	c.lastBreathe = snap.Breathing
}

func TestInstantBreakFoliageHardnessAndFluidDetection(t *testing.T) {
	foliage := []string{
		"minecraft:short_grass",
		"minecraft:tall_grass",
		"minecraft:fern",
		"minecraft:large_fern",
		"minecraft:dead_bush",
		"minecraft:seagrass",
		"minecraft:tall_seagrass",
		"minecraft:kelp",
		"minecraft:kelp_plant",
	}
	for _, name := range foliage {
		id := world.StateID(name)
		if h := world.Hardness(id); h != 0 {
			t.Fatalf("Hardness(%q) = %v, want 0 (instant break)", name, h)
		}
	}

	waterID := world.StateID("minecraft:water")
	seagrassID := world.StateID("minecraft:seagrass")
	lavaID := world.StateID("minecraft:lava")
	stoneID := world.StateID("minecraft:stone")

	if !world.IsWaterBlockID(waterID) {
		t.Fatalf("IsWaterBlockID(minecraft:water) = false, want true")
	}
	if !world.IsWaterBlockID(seagrassID) {
		t.Fatalf("IsWaterBlockID(minecraft:seagrass) = false, want true (submerged foliage counts as water)")
	}
	if world.IsWaterBlockID(stoneID) {
		t.Fatalf("IsWaterBlockID(minecraft:stone) = true, want false")
	}
	if !world.IsLavaBlockID(lavaID) {
		t.Fatalf("IsLavaBlockID(minecraft:lava) = false, want true")
	}
	if wbID, ok := item.BlockStateID("minecraft:water_bucket"); !ok || wbID != waterID {
		t.Fatalf("BlockStateID(minecraft:water_bucket) = (%d, %v), want (%d, true)", wbID, ok, waterID)
	}
	if lbID, ok := item.BlockStateID("minecraft:lava_bucket"); !ok || lbID != lavaID {
		t.Fatalf("BlockStateID(minecraft:lava_bucket) = (%d, %v), want (%d, true)", lbID, ok, lavaID)
	}
}

func TestLocatorBarDefaultConfigEnabled(t *testing.T) {
	cfg := config.Default()
	if !cfg.World.LocatorBar {
		t.Fatalf("config.Default().World.LocatorBar = false, want true")
	}
}

func TestEnvironmentTickWaterBreathingDrowningAndLavaFire(t *testing.T) {
	wm := world.NewManager()
	dw := wm.GetDefaultWorld()

	pm := player.NewManager()
	id := uuid.New()
	p := player.NewPlayer(id, "Tester", player.EditionBedrock)
	p.World = dw
	p.Position = world.Position{X: 10.5, Y: 10.0, Z: 10.5}
	ctrl := &envRecorderController{}
	pm.AddPlayer(p)
	pm.SetController(id, ctrl)

	waterID := world.StateID("minecraft:water")
	lavaID := world.StateID("minecraft:lava")
	airID := world.StateID("minecraft:air")

	// Submerge feet (Y=10) and eyes (Y=11) in water while sprinting -> should enter Swimming and drain AirSupply
	dw.SetBlock(10, 10, 10, world.BlockByID(waterID))
	dw.SetBlock(10, 11, 10, world.BlockByID(waterID))
	pm.UpdateSprinting(id, true)

	pm.TickEnvironment(dw)
	snap := pm.GetPlayer(id).Snapshot()
	if !snap.Swimming {
		t.Fatalf("expected sprinting submerged player to enter Swimming state")
	}
	if snap.Breathing {
		t.Fatalf("expected submerged eye block to set Breathing=false")
	}
	if snap.AirSupply != 299 {
		t.Fatalf("AirSupply after 1 submerged tick = %d, want 299", snap.AirSupply)
	}

	// Drain air down to -19 so the next tick hits -20 and triggers 1.0 heart (2.0 HP) drowning damage
	p.AirSupply = -19
	prevHurts := ctrl.hurtCount
	pm.TickEnvironment(dw)
	if ctrl.hurtCount <= prevHurts {
		t.Fatalf("expected drowning damage at -20 air ticks; hurtCount remained %d", ctrl.hurtCount)
	}
	if p.AirSupply != 0 {
		t.Fatalf("expected AirSupply to reset to 0 after drowning tick, got %d", p.AirSupply)
	}

	// Replace water with lava at feet -> should ignite player and deal lava contact damage
	dw.SetBlock(10, 10, 10, world.BlockByID(lavaID))
	dw.SetBlock(10, 11, 10, world.BlockByID(airID))
	hurtsBeforeLava := ctrl.hurtCount
	pm.TickEnvironment(dw)
	if !p.OnFire || p.FireTicks <= 0 {
		t.Fatalf("expected lava contact to set OnFire=true and FireTicks>0 (got OnFire=%v, FireTicks=%d)", p.OnFire, p.FireTicks)
	}
	if ctrl.hurtCount <= hurtsBeforeLava {
		t.Fatalf("expected lava contact damage; hurtCount=%d (was %d)", ctrl.hurtCount, hurtsBeforeLava)
	}

	// Stepping back into water extinguishes fire immediately
	dw.SetBlock(10, 10, 10, world.BlockByID(waterID))
	pm.TickEnvironment(dw)
	if p.OnFire || p.FireTicks != 0 {
		t.Fatalf("expected water contact to extinguish fire (got OnFire=%v, FireTicks=%d)", p.OnFire, p.FireTicks)
	}
}

func TestMobKnockbackDirectionAwayFromAttacker(t *testing.T) {
	// Attacker at X=0, Z=0 hitting a mob at X=3, Z=0:
	// dirX, dirZ passed to combat.Knockback must be (attacker - victim) = (-3, 0)
	// so combat.Knockback's negation pushes the mob in +X (away from attacker).
	attackerX, attackerZ := 0.0, 0.0
	mobX, mobZ := 3.0, 0.0
	dx := attackerX - mobX
	dz := attackerZ - mobZ

	out := combat.Knockback(registry.Vec3{}, 0.4, dx, dz, 0.0, true)
	if out.X <= 0 {
		t.Fatalf("expected mob at +X to be knocked in +X direction (away from attacker), got VX=%v", out.X)
	}
	if out.Y <= 0 {
		t.Fatalf("expected grounded mob to receive upward knockback VY>0, got VY=%v", out.Y)
	}
}

func TestPlayerMeleeKnockbackSprintBonusAndIFrames(t *testing.T) {
	pm := player.NewManager()
	attackerID := uuid.New()
	victimID := uuid.New()

	attacker := player.NewPlayer(attackerID, "Attacker", player.EditionJava)
	attacker.EntityRuntimeID = 101
	attacker.Position = world.Position{X: 0, Y: 64, Z: 0}
	attacker.Sprinting = true
	attacker.OnGround = true

	victim := player.NewPlayer(victimID, "Victim", player.EditionBedrock)
	victim.EntityRuntimeID = 102
	victim.Position = world.Position{X: 2, Y: 64, Z: 0}
	victim.OnGround = true

	victimCtrl := &envRecorderController{}
	pm.AddPlayer(attacker)
	pm.AddPlayer(victim)
	pm.SetController(victimID, victimCtrl)

	pm.MeleeAttack(attackerID, int32(victim.EntityRuntimeID))
	if victimCtrl.hurtCount != 1 {
		t.Fatalf("expected 1 hurt call on first melee hit, got %d", victimCtrl.hurtCount)
	}
	if pm.GetPlayer(attackerID).Sprinting {
		t.Fatalf("expected sprint to be cleared after sprint-knockback melee hit")
	}

	// Immediate second hit within 10-tick I-frames window must be ignored
	pm.MeleeAttack(attackerID, int32(victim.EntityRuntimeID))
	if victimCtrl.hurtCount != 1 {
		t.Fatalf("expected second hit within I-frames to be ignored, hurtCount=%d", victimCtrl.hurtCount)
	}
}
