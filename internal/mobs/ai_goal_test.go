package mobs

import (
	"math"
	"math/rand"
	"testing"
)

// fakeGoal is a test double recording lifecycle calls. canUse/canCont are
// pointers so a test can flip them mid-run.
type fakeGoal struct {
	flags                GoalFlag
	canUse, canCont      bool
	interrupt            bool
	starts, stops, ticks *int
}

func (g *fakeGoal) Flags() GoalFlag                   { return g.flags }
func (g *fakeGoal) CanUse(*Mob, *AIContext) bool      { return g.canUse }
func (g *fakeGoal) CanContinue(*Mob, *AIContext) bool { return g.canCont }
func (g *fakeGoal) Interruptible() bool               { return g.interrupt }
func (g *fakeGoal) Start(*Mob, *AIContext)            { *g.starts++ }
func (g *fakeGoal) Stop(*Mob, *AIContext)             { *g.stops++ }
func (g *fakeGoal) Tick(*Mob, *AIContext)             { *g.ticks++ }

func counters() (s, st, t *int) {
	a, b, c := 0, 0, 0
	return &a, &b, &c
}

// TestGoalSelector_LowerPriorityWinsSameFlag verifies that when two idle goals
// both want FlagMove, only the lower-priority-number goal runs.
func TestGoalSelector_LowerPriorityWinsSameFlag(t *testing.T) {
	gs := newGoalSelector()
	hiS, hiSt, hiT := counters()
	loS, loSt, loT := counters()
	hi := &fakeGoal{flags: FlagMove, canUse: true, canCont: true, interrupt: true, starts: hiS, stops: hiSt, ticks: hiT}
	lo := &fakeGoal{flags: FlagMove, canUse: true, canCont: true, interrupt: true, starts: loS, stops: loSt, ticks: loT}
	gs.add(2, hi) // higher priority number = less important
	gs.add(0, lo) // priority 0 = most important

	gs.tick(&Mob{}, &AIContext{})

	if *loS != 1 || *loT != 1 {
		t.Errorf("priority-0 goal should run: starts=%d ticks=%d", *loS, *loT)
	}
	if *hiS != 0 {
		t.Errorf("priority-2 goal should be blocked by FlagMove conflict, started %d times", *hiS)
	}
}

// TestGoalSelector_DifferentFlagsCoexist verifies a FlagMove and a FlagLook
// goal run simultaneously.
func TestGoalSelector_DifferentFlagsCoexist(t *testing.T) {
	gs := newGoalSelector()
	mS, _, mT := counters()
	lS, _, lT := counters()
	mv := &fakeGoal{flags: FlagMove, canUse: true, canCont: true, starts: mS, stops: new(int), ticks: mT}
	lk := &fakeGoal{flags: FlagLook, canUse: true, canCont: true, starts: lS, stops: new(int), ticks: lT}
	gs.add(1, mv)
	gs.add(1, lk)

	gs.tick(&Mob{}, &AIContext{})

	if *mT != 1 || *lT != 1 {
		t.Errorf("move and look goals should both tick: move=%d look=%d", *mT, *lT)
	}
}

// TestGoalSelector_EvictInterruptibleHolder verifies a higher-priority goal
// evicts a running lower-priority interruptible goal that holds its flag.
func TestGoalSelector_EvictInterruptibleHolder(t *testing.T) {
	gs := newGoalSelector()
	m := &Mob{}
	ctx := &AIContext{}

	loS, loSt, loT := counters()
	lo := &fakeGoal{flags: FlagMove, canUse: true, canCont: true, interrupt: true, starts: loS, stops: loSt, ticks: loT}
	gs.add(5, lo)
	gs.tick(m, ctx) // lo starts
	if *loS != 1 {
		t.Fatalf("low-priority goal should have started")
	}

	// Introduce a higher-priority goal contending for FlagMove.
	hiS, _, _ := counters()
	hi := &fakeGoal{flags: FlagMove, canUse: true, canCont: true, interrupt: true, starts: hiS, stops: new(int), ticks: new(int)}
	gs.add(1, hi)
	gs.tick(m, ctx)

	if *hiS != 1 {
		t.Errorf("high-priority goal should start by eviction")
	}
	if *loSt != 1 {
		t.Errorf("low-priority holder should be stopped on eviction, stops=%d", *loSt)
	}
}

// TestGoalSelector_NonInterruptibleNotEvicted verifies a non-interruptible
// running goal keeps its flag against a higher-priority contender.
func TestGoalSelector_NonInterruptibleNotEvicted(t *testing.T) {
	gs := newGoalSelector()
	m := &Mob{}
	ctx := &AIContext{}

	loS, loSt, _ := counters()
	lo := &fakeGoal{flags: FlagMove, canUse: true, canCont: true, interrupt: false, starts: loS, stops: loSt, ticks: new(int)}
	gs.add(5, lo)
	gs.tick(m, ctx)

	hiS, _, _ := counters()
	hi := &fakeGoal{flags: FlagMove, canUse: true, canCont: true, interrupt: true, starts: hiS, stops: new(int), ticks: new(int)}
	gs.add(1, hi)
	gs.tick(m, ctx)

	if *hiS != 0 {
		t.Errorf("high-priority goal must not evict a non-interruptible holder")
	}
	if *loSt != 0 {
		t.Errorf("non-interruptible holder must not be stopped")
	}
}

// --- behaviour smoke tests (real goals via Store.Tick) --------------------

func aiTestCtx(players []PlayerTarget) AIContext {
	return AIContext{
		RNG:     rand.New(rand.NewSource(1)),
		SolidAt: func(x, y, z int) bool { return y < 64 }, // floor at y=64
		Players: func() []PlayerTarget { return players },
	}
}

// TestZombie_AcquiresAndChasesPlayer verifies the target selector locks the
// player and the melee goal moves the zombie toward them.
func TestZombie_AcquiresAndChasesPlayer(t *testing.T) {
	s := New()
	z := s.Spawn("minecraft:zombie", 0, 64, 0)
	player := PlayerTarget{UUID: [16]byte{1}, X: 5, Y: 64, Z: 0}
	ctx := aiTestCtx([]PlayerTarget{player})

	startX := s.Get(z.EntityID).X
	for i := 0; i < 10; i++ {
		s.Tick(ctx)
	}
	got := s.Get(z.EntityID)
	if got.target != player.UUID {
		t.Errorf("zombie did not acquire player target: %v", got.target)
	}
	if got.X <= startX {
		t.Errorf("zombie did not move toward player: startX=%.2f endX=%.2f", startX, got.X)
	}
}

// TestCow_PanicsWhenHurt verifies a passive mob flees its attacker after Hurt.
func TestCow_PanicsWhenHurt(t *testing.T) {
	s := New()
	c := s.Spawn("minecraft:cow", 5, 64, 0)
	attacker := PlayerTarget{UUID: [16]byte{2}, X: 0, Y: 64, Z: 0}
	ctx := aiTestCtx([]PlayerTarget{attacker})

	s.Tick(ctx) // establish aiTick clock
	s.Hurt(c.EntityID, attacker.UUID)

	startX := s.Get(c.EntityID).X
	for i := 0; i < 10; i++ {
		s.Tick(ctx)
	}
	got := s.Get(c.EntityID)
	if got.X <= startX {
		t.Errorf("cow should flee away from attacker (increasing X): startX=%.2f endX=%.2f", startX, got.X)
	}
}

// TestCreeper_SwellsAndExplodes verifies the swell goal arms the fuse in range
// and detonates via OnExplode.
func TestCreeper_SwellsAndExplodes(t *testing.T) {
	s := New()
	cr := s.Spawn("minecraft:creeper", 0, 64, 0)
	player := PlayerTarget{UUID: [16]byte{3}, X: 1, Y: 64, Z: 0}
	exploded := false
	ctx := aiTestCtx([]PlayerTarget{player})
	ctx.OnExplode = func(int64, float64, float64, float64, float64) { exploded = true }

	for i := 0; i < 60 && !exploded; i++ {
		s.Tick(ctx)
	}
	if !exploded {
		t.Errorf("creeper next to player should detonate within 60 ticks; fuse=%d", s.Get(cr.EntityID).fuseTicks)
	}
}

func TestLookAt_ClampsHeadYawToBody(t *testing.T) {
	m := &Mob{Type: "minecraft:zombie", X: 0, Y: 64, Z: 0, Yaw: 0, HeadYaw: 0}

	lookAt(m, 10, 65, -10, false)
	if diff := math.Abs(wrapDegrees(m.HeadYaw - m.Yaw)); diff > maxHeadBodyYaw {
		t.Fatalf("head yaw should stay within body clamp: diff=%.2f max=%.2f", diff, maxHeadBodyYaw)
	}

	lookAt(m, -10, 65, -10, false)
	if diff := math.Abs(wrapDegrees(m.HeadYaw - m.Yaw)); diff > maxHeadBodyYaw {
		t.Fatalf("head yaw should stay clamped after target crosses behind: diff=%.2f max=%.2f", diff, maxHeadBodyYaw)
	}
}

func TestSpawn_InitialHeadYawMatchesBodyYaw(t *testing.T) {
	s := New()
	z := s.Spawn("minecraft:zombie", 0, 64, 0)

	if z.HeadYaw != z.Yaw {
		t.Fatalf("spawn should initialise head yaw to body yaw: head=%.2f body=%.2f", z.HeadYaw, z.Yaw)
	}
}
