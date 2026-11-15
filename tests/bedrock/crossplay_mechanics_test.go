package bedrocktests

import (
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"

	"livingworld/config"
	"livingworld/internal/combat"
	"livingworld/internal/item"
	"livingworld/internal/mobs"
	aisystems "livingworld/internal/mobs/ai/systems"
	"livingworld/internal/player"
	"livingworld/internal/registry"
	"livingworld/internal/world"
)

type envRecorderController struct {
	syncCount    int
	hurtCount    int
	healCount    int
	hungerCount  int
	lastDamage   float32
	lastHeal     float32
	lastFood     int
	lastSat      float32
	lastAir      int16
	lastFire     bool
	lastSwim     bool
	lastBreathe  bool
}

func (c *envRecorderController) SendMessage(string)                {}
func (c *envRecorderController) Kick(string)                       {}
func (c *envRecorderController) Push(vx, vy, vz float64)           {}
func (c *envRecorderController) Hurt(amount float32)               { c.hurtCount++; c.lastDamage += amount }
func (c *envRecorderController) Heal(amount float32)               { c.healCount++; c.lastHeal += amount }
func (c *envRecorderController) SyncHunger(food int, sat, exh float32) {
	c.hungerCount++
	c.lastFood = food
	c.lastSat = sat
}
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

func TestSurfaceSprintDoesNotTriggerSwimming(t *testing.T) {
	wm := world.NewManager()
	dw := wm.GetDefaultWorld()
	pm := player.NewManager()

	waterID := world.StateID("minecraft:water")
	airID := world.StateID("minecraft:air")

	// Water at Y=10, Air at Y=11..12 (surface at Y=11.0).
	dw.SetBlock(5, 10, 5, world.BlockByID(waterID))
	dw.SetBlock(5, 11, 5, world.BlockByID(airID))
	dw.SetBlock(5, 12, 5, world.BlockByID(airID))

	id := uuid.New()
	p := player.NewPlayer(id, "BedrockSwimmer", player.EditionBedrock)
	p.World = dw
	// Floating on water surface: feet at Y=10.65 (touching water Y=10), upper body & head in air.
	p.Position = world.Position{X: 5.5, Y: 10.65, Z: 5.5}
	ctrl := &envRecorderController{}
	pm.AddPlayer(p)
	pm.SetController(id, ctrl)

	pm.UpdateSprinting(id, true)
	if pm.GetPlayer(id).Swimming {
		t.Fatalf("expected sprinting while floating on water surface NOT to enter Swimming")
	}

	// Client-initiated StartSwimming at the water surface must be rejected and resynced.
	prevSync := ctrl.syncCount
	pm.UpdateSwimming(id, true)
	if pm.GetPlayer(id).Swimming {
		t.Fatalf("expected UpdateSwimming(true) at surface to be rejected")
	}
	if ctrl.syncCount <= prevSync {
		t.Fatalf("expected rejected UpdateSwimming(true) to trigger SyncEnvironment to clear client swim flag")
	}

	// Standing in 1-block shallow water (feet at Y=10.0, eyes at Y=11.62 in air) must also not enter swim.
	pm.UpdatePosition(id, 5.5, 10.0, 5.5, 0, 0, true)
	pm.TickEnvironment(dw)
	if pm.GetPlayer(id).Swimming {
		t.Fatalf("expected sprinting in 1-block shallow water (eyes in air) NOT to enter Swimming")
	}

	// Submerging 2 blocks deep (water at Y=10 and Y=11) allows swimming, and popping back up to surface exits swim.
	dw.SetBlock(5, 11, 5, world.BlockByID(waterID))
	pm.UpdatePosition(id, 5.5, 10.0, 5.5, 0, 0, false)
	if !pm.GetPlayer(id).Swimming {
		t.Fatalf("expected submerged sprinting player to enter Swimming")
	}
	// Float up to surface of Y=11 water block (feet at Y=11.65 in water, body y+0.4=12.05 in air Y=12).
	pm.UpdatePosition(id, 5.5, 11.65, 5.5, 0, 0, false)
	if pm.GetPlayer(id).Swimming {
		t.Fatalf("expected player floating onto top of water surface to exit Swimming")
	}
}

func TestPlayerPushesMobsAndMobsSeparate(t *testing.T) {
	store := mobs.New()
	m1 := store.Spawn("minecraft:cow", 10.3, 64, 10.0)
	m2 := store.Spawn("minecraft:pig", 20.0, 64, 20.0)
	m3 := store.Spawn("minecraft:sheep", 20.1, 64, 20.0)

	var movedIDs []int64
	store.OnMove(func(m mobs.Mob) {
		movedIDs = append(movedIDs, m.EntityID)
	})

	var playerUUID [16]byte
	playerUUID[0] = 1
	ctx := mobs.AIContext{
		RNG:     rand.New(rand.NewSource(42)),
		SolidAt: func(x, y, z int) bool { return y < 64 },
		Players: func() []mobs.PlayerTarget {
			return []mobs.PlayerTarget{
				{UUID: playerUUID, X: 10.0, Y: 64.0, Z: 10.0, Gamemode: 0},
			}
		},
	}

	aisystems.Tick(store, ctx)

	snap := store.All()
	var after1, after2, after3 mobs.Mob
	for _, m := range snap {
		switch m.EntityID {
		case m1.EntityID:
			after1 = m
		case m2.EntityID:
			after2 = m
		case m3.EntityID:
			after3 = m
		}
	}

	if after1.X <= 10.3 {
		t.Fatalf("expected player at X=10.0 to push mob at X=10.3 in +X direction, got X=%v", after1.X)
	}
	if after3.X-after2.X <= 0.1 {
		t.Fatalf("expected overlapping mobs to separate horizontally, got m2.X=%v m3.X=%v", after2.X, after3.X)
	}
	if len(movedIDs) == 0 {
		t.Fatalf("expected pushed mobs to emit OnMove updates")
	}
}

func TestWeatherClearResetsCycleTimer(t *testing.T) {
	wm := world.NewManager()
	defer wm.Close()

	wm.SetWeather(true, false)
	wm.StartWeatherCycle(true, world.WeatherDurations{
		ClearMin:   3,
		ClearMax:   3,
		RainMin:    1,
		RainMax:    1,
		ThunderMin: 1,
		ThunderMax: 1,
	})

	// Immediately clear weather via /weather clear equivalent.
	wm.SetWeather(false, false)
	time.Sleep(1500 * time.Millisecond)

	raining, thundering := wm.GetDefaultWorld().Weather()
	if raining || thundering {
		t.Fatalf("expected /weather clear to reset weather cycle timer to Clear duration (3s), but weather reverted after 1.5s: raining=%v thundering=%v", raining, thundering)
	}
}

func TestHungerMechanicsSprintGateRegenAndEating(t *testing.T) {
	ht := world.NewHungerTracker()
	pm := player.NewManager()

	pm.SetHungerHooks(
		func(id uuid.UUID, amount float32) { ht.AddExhaustion(id, amount) },
		func(id uuid.UUID) bool { return ht.CanSprint(id) },
	)
	ht.SetSyncCallback(func(id uuid.UUID, food int, sat, exh float32) {
		pm.SyncHunger(id, food, sat, exh)
	})
	ht.SetCanHealCallback(func(id uuid.UUID) bool {
		p := pm.GetPlayer(id)
		return p != nil && p.Health > 0 && p.Health < 20
	})
	ht.SetHealCallback(func(id uuid.UUID, amount float32) {
		pm.HealPlayer(id, amount)
	})
	ht.SetStarveCallback(func(id uuid.UUID, amount float32) {
		pm.DamagePlayer(id, amount)
	})

	id := uuid.New()
	p := player.NewPlayer(id, "HungryTester", player.EditionBedrock)
	p.Position = world.Position{X: 0, Y: 64, Z: 0}
	ctrl := &envRecorderController{}
	pm.AddPlayer(p)
	pm.SetController(id, ctrl)
	ht.RegisterPlayer(id, 20, 2.0, "normal")

	// Sprinting 45 meters adds > 4.0 exhaustion -> drains 1.0 saturation.
	pm.UpdateSprinting(id, true)
	for i := 1; i <= 45; i++ {
		pm.UpdatePosition(id, float64(i), 64, 0, 0, 0, true)
	}
	ht.Tick()
	if ctrl.lastSat >= 2.0 {
		t.Fatalf("expected sprinting exhaustion to drain saturation below 2.0, got %v", ctrl.lastSat)
	}

	// Drain food down to 6 -> must automatically stop sprinting and block new sprinting.
	ht.RegisterPlayer(id, 7, 0, "normal")
	pm.UpdateSprinting(id, true)
	if !pm.GetPlayer(id).Sprinting {
		t.Fatalf("expected player with food=7 to be able to sprint")
	}
	ht.AddExhaustion(id, 4.0)
	ht.Tick()
	if pm.GetPlayer(id).Food != 6 {
		t.Fatalf("expected food to drop to 6, got %d", pm.GetPlayer(id).Food)
	}
	if pm.GetPlayer(id).Sprinting {
		t.Fatalf("expected sprinting to automatically stop when food drops to 6")
	}
	pm.UpdateSprinting(id, true)
	if pm.GetPlayer(id).Sprinting {
		t.Fatalf("expected UpdateSprinting(true) to be rejected when food <= 6")
	}

	// Eating cooked_beef (+8 food, +12.8 saturation) restores food to 14 and re-enables sprinting.
	def, ok := world.LookupFood("minecraft:cooked_beef")
	if !ok || !ht.CanEat(id, def.AlwaysEdible) {
		t.Fatalf("expected cooked_beef to be edible at food=6")
	}
	ht.Feed(id, def.FoodPoints, def.Saturation)
	if pm.GetPlayer(id).Food != 14 {
		t.Fatalf("expected food=14 after eating cooked_beef from 6, got %d", pm.GetPlayer(id).Food)
	}
	pm.UpdateSprinting(id, true)
	if !pm.GetPlayer(id).Sprinting {
		t.Fatalf("expected player to be able to sprint again after eating food to 14")
	}

	// Natural regeneration only fires when health < maxHealth.
	ht.ResetPlayer(id, "normal")
	pm.SetHealth(id, 15)
	for i := 0; i < 10; i++ {
		ht.Tick()
	}
	if pm.GetPlayer(id).Health <= 15 || ctrl.healCount == 0 {
		t.Fatalf("expected rapid saturation healing at food=20 sat>0 when damaged, got HP=%v healCount=%d", pm.GetPlayer(id).Health, ctrl.healCount)
	}
}

func TestDeadPlayerWaterImmunityAndRespawnReset(t *testing.T) {
	wm := world.NewManager()
	defer wm.Close()
	dw := wm.GetDefaultWorld()
	pm := player.NewManager()

	waterID := world.StateID("minecraft:water")
	dw.SetBlock(0, 60, 0, world.BlockByID(waterID))
	dw.SetBlock(0, 61, 0, world.BlockByID(waterID))

	id := uuid.New()
	p := player.NewPlayer(id, "DrownedHero", player.EditionBedrock)
	p.World = dw
	p.Position = world.Position{X: 0.5, Y: 60.0, Z: 0.5}
	p.AirSupply = 0
	p.Breathing = false
	ctrl := &envRecorderController{}
	pm.AddPlayer(p)
	pm.SetController(id, ctrl)

	// Kill player in water (Health = 0).
	pm.SetHealth(id, 0)
	hurtsBefore := ctrl.hurtCount

	// Ticking environment 60 times while dead in water must NOT deal any drowning damage.
	for i := 0; i < 60; i++ {
		pm.TickEnvironment(dw)
	}
	if ctrl.hurtCount != hurtsBefore {
		t.Fatalf("expected dead player in water to take 0 drowning hits while on respawn screen, got %d additional hits", ctrl.hurtCount-hurtsBefore)
	}

	// Respawning restores full health (20), full air (300), and teleports to safe spawn.
	snap := pm.RespawnPlayer(id, 10.5, 65.0, 10.5, 0, 0)
	if pm.GetPlayer(id).Health != 20 || snap.AirSupply != 300 || !snap.Breathing || snap.OnFire {
		t.Fatalf("unexpected respawn state: health=%v snap=%+v", pm.GetPlayer(id).Health, snap)
	}
}

func TestVanillaMobBehaviorsDeadPlayerCreeperSpider(t *testing.T) {
	store := mobs.New()
	creeper := store.Spawn("minecraft:creeper", 0, 64, 0)
	spider := store.Spawn("minecraft:spider", 50, 64, 50)

	var pUUID [16]byte
	pUUID[0] = 7
	playerDead := true
	playerX, playerZ := 2.0, 0.0
	isDay := true

	ctx := mobs.AIContext{
		RNG:     rand.New(rand.NewSource(99)),
		SolidAt: func(x, y, z int) bool { return y < 64 },
		IsDay:   func() bool { return isDay },
		Players: func() []mobs.PlayerTarget {
			return []mobs.PlayerTarget{
				{UUID: pUUID, X: playerX, Y: 64, Z: playerZ, Gamemode: 0, Dead: playerDead},
			}
		},
	}

	// 1. Dead player must NOT be targeted or ignite creeper fuse.
	aisystems.Tick(store, ctx)
	for _, m := range store.All() {
		if m.EntityID == creeper.EntityID && (m.Target != ([16]byte{}) || m.FuseTicks > 0) {
			t.Fatalf("expected creeper to ignore dead player, got target=%v fuse=%d", m.Target, m.FuseTicks)
		}
	}

	// 2. Alive player within 3 blocks ignites creeper fuse; backing up to 5m (< 7m) keeps swelling; backing up to 8m (> 7m) cancels fuse.
	playerDead = false
	for i := 0; i < 4; i++ {
		aisystems.Tick(store, ctx)
	}
	var fuseAt2m int
	for _, m := range store.All() {
		if m.EntityID == creeper.EntityID {
			fuseAt2m = m.FuseTicks
		}
	}
	if fuseAt2m <= 0 {
		t.Fatalf("expected creeper fuse to start at 2m distance, got %d", fuseAt2m)
	}
	playerX = 5.0 // 5m is > 3m ignition radius, but <= 7m cancel radius
	aisystems.Tick(store, ctx)
	for _, m := range store.All() {
		if m.EntityID == creeper.EntityID && (m.State != mobs.StateFuse || m.FuseTicks >= fuseAt2m) {
			t.Fatalf("expected creeper fuse countdown to continue at 5m (< 7m), got state=%v fuse=%d (prev=%d)", m.State, m.FuseTicks, fuseAt2m)
		}
	}
	playerX = 8.5 // > 7m cancels fuse
	aisystems.Tick(store, ctx)
	for _, m := range store.All() {
		if m.EntityID == creeper.EntityID && (m.State == mobs.StateFuse || m.FuseTicks <= fuseAt2m) {
			t.Fatalf("expected creeper fuse to cancel outside 7m radius, got state=%v fuse=%d", m.State, m.FuseTicks)
		}
	}

	// 3. Spider is neutral in daylight unless provoked.
	playerX, playerZ = 52.0, 50.0
	aisystems.Tick(store, ctx)
	for _, m := range store.All() {
		if m.EntityID == spider.EntityID && m.Target != ([16]byte{}) {
			t.Fatalf("expected spider to remain neutral during daylight when unprovoked")
		}
	}
	store.HurtDirect(spider.EntityID, pUUID, 2.0)
	aisystems.Tick(store, ctx)
	for _, m := range store.All() {
		if m.EntityID == spider.EntityID && m.Target != pUUID {
			t.Fatalf("expected spider to retaliate when provoked in daylight")
		}
	}
}

func TestSnowBreakTicksAndPeacefulHungerAndCritAndDeadSnapshot(t *testing.T) {
	// 1. Snow break duration: bare hand vs shovel.
	snowID := world.StateID("minecraft:snow")
	if snowID != 0 {
		bareTicks := world.BreakTicks(snowID, "", false, true)
		shovelTicks := world.BreakTicks(snowID, "wooden_shovel", false, true)
		if bareTicks < 29.9 || bareTicks > 30.1 {
			t.Fatalf("expected bare-hand snow break to take 30 ticks (1.5s), got %v", bareTicks)
		}
		if shovelTicks >= bareTicks {
			t.Fatalf("expected shovel to break snow faster than bare hand, got shovel=%v bare=%v", shovelTicks, bareTicks)
		}
	}

	// 2. Peaceful difficulty: hunger never drains and regenerates food/HP.
	wm := world.NewManager()
	defer wm.Close()
	ht := wm.Hunger()
	id := uuid.New()
	var syncedFood int
	var healed float32
	ht.SetSyncCallback(func(_ uuid.UUID, food int, _, _ float32) {
		syncedFood = food
	})
	ht.SetHealCallback(func(_ uuid.UUID, amount float32) {
		healed += amount
	})
	ht.RegisterPlayer(id, 15, 5.0, "peaceful")
	ht.AddExhaustion(id, 40.0)
	for i := 0; i < 20; i++ {
		ht.Tick()
	}
	if syncedFood <= 15 {
		t.Fatalf("expected peaceful difficulty to never drain hunger and regenerate food > 15, got %d", syncedFood)
	}
	if healed <= 0 {
		t.Fatalf("expected peaceful difficulty to trigger passive healing, got %v", healed)
	}

	// 3. Authoritative FallDistance & CanCrit on descending jump + Dead snapshot flag.
	pm := player.NewManager()
	dw := wm.GetDefaultWorld()
	p := player.NewPlayer(id, "CritTester", player.EditionBedrock)
	p.World = dw
	p.Position = world.Position{X: 0.5, Y: 64.0, Z: 0.5}
	pm.AddPlayer(p)

	// Ascending jump: FallDistance must remain 0 and CanCrit must be false.
	pm.UpdatePosition(id, 0.5, 64.42, 0.5, 0, 0, false)
	if pm.GetPlayer(id).CanCrit() {
		t.Fatalf("expected ascending jump not to allow critical hits")
	}
	// Descending from apex while airborne: FallDistance > 0 and CanCrit must be true.
	pm.UpdatePosition(id, 0.5, 64.10, 0.5, 0, 0, false)
	if !pm.GetPlayer(id).CanCrit() {
		t.Fatalf("expected descending airborne player to allow critical hits")
	}

	// Killing the player marks Snapshot().Dead = true; RespawnPlayer clears it.
	pm.SetHealth(id, 0)
	if !pm.GetPlayer(id).Snapshot().Dead {
		t.Fatalf("expected dead player snapshot to have Dead=true")
	}
	snap := pm.RespawnPlayer(id, 0.5, 64.0, 0.5, 0, 0)
	if snap.Dead || pm.GetPlayer(id).Health != 20 {
		t.Fatalf("expected respawned player snapshot to have Dead=false and Health=20, got Dead=%v Health=%v", snap.Dead, pm.GetPlayer(id).Health)
	}
}




