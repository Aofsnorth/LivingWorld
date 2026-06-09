package mobs

// Goal-selector engine — a faithful port of vanilla Minecraft's
// net.minecraft.world.entity.ai.goal.GoalSelector. A mob's behaviour is no
// longer a single AIState switch (see the pre-rombak ai.go); instead every
// behaviour is a Goal with a numeric priority and a set of control Flags.
// Each tick the selector decides which goals may run based on priority and
// flag-mutual-exclusion, then ticks the running set.
//
// Two selectors run per mob, mirroring vanilla:
//   - goalSel   : the behaviour selector (move / look / attack / wander).
//   - targetSel : the target-acquisition selector (who to attack).
// They are independent so a mob can re-evaluate "who is my target" (FlagTarget)
// while a move goal keeps running (FlagMove). The split is exactly how vanilla
// keeps Mob.goalSelector and Mob.targetSelector apart.
//
// Lower priority number = more important (vanilla convention). A goal with
// priority 0 outranks priority 8. When two idle goals both want to start and
// contend for the same flag, the lower-priority-number goal wins; a running
// goal can be evicted only if it is Interruptible and is outranked.

// GoalFlag is a control channel a goal occupies while running. Two goals that
// share a flag cannot run simultaneously — the higher-priority one wins. This
// is vanilla's Goal.Flag (MOVE / LOOK / JUMP / TARGET).
type GoalFlag uint8

const (
	// FlagMove — the goal drives the mob's body position (pathing, fleeing,
	// strolling). Only one move goal runs at a time.
	FlagMove GoalFlag = 1 << iota
	// FlagLook — the goal drives the head yaw/pitch (watch player, look
	// around). Decoupled from FlagMove so a mob can stroll AND watch.
	FlagLook
	// FlagJump — the goal drives discrete jumps (leap-at-target).
	FlagJump
	// FlagTarget — the goal selects/clears m.target. Lives on the target
	// selector; kept distinct so targeting never blocks movement.
	FlagTarget
)

// allFlags is the iteration set used by the selector when locking/releasing.
var allFlags = [...]GoalFlag{FlagMove, FlagLook, FlagJump, FlagTarget}

// Goal is one unit of mob behaviour. Implementations are small structs (often
// zero-sized) holding only per-type configuration; all per-mob mutable state
// lives on *Mob so a single Goal value can be shared across mobs of a type.
//
// Lifecycle per vanilla:
//   CanUse      → may this goal start this tick?
//   CanContinue → may a *running* goal keep going? (default: CanUse)
//   Start       → called once when the goal transitions idle→running.
//   Tick        → called every tick while running.
//   Stop        → called once when the goal transitions running→idle.
//   Flags       → the control channels this goal occupies while running.
type Goal interface {
	CanUse(m *Mob, ctx *AIContext) bool
	CanContinue(m *Mob, ctx *AIContext) bool
	Start(m *Mob, ctx *AIContext)
	Stop(m *Mob, ctx *AIContext)
	Tick(m *Mob, ctx *AIContext)
	Flags() GoalFlag
}

// interruptibleGoal is an optional interface. A goal that returns false from
// Interruptible() cannot be evicted by a higher-priority goal mid-run (vanilla
// Goal.isInterruptable defaults true). Goals that don't implement it are
// interruptible.
type interruptibleGoal interface {
	Interruptible() bool
}

func goalInterruptible(g Goal) bool {
	if ig, ok := g.(interruptibleGoal); ok {
		return ig.Interruptible()
	}
	return true
}

// baseGoal is embedded by most goals to default CanContinue→CanUse and to
// supply no-op Start/Stop. A goal overrides only what it needs.
type baseGoal struct{}

func (baseGoal) Start(*Mob, *AIContext) {}
func (baseGoal) Stop(*Mob, *AIContext)  {}

// wrappedGoal pairs a goal with its priority and running state. The `running`
// flag is the per-mob bit (one wrappedGoal per mob, built by buildAI), so this
// struct is NOT shared between mobs.
type wrappedGoal struct {
	priority int
	goal     Goal
	running  bool
}

// goalSelector holds a mob's prioritised goal list and the flag-lock table.
// One instance per mob per selector kind (goalSel + targetSel).
type goalSelector struct {
	goals   []*wrappedGoal
	locked  map[GoalFlag]*wrappedGoal // flag → the running goal that owns it
	scratch []*wrappedGoal            // reused by byPriority to avoid per-tick allocs
}

func newGoalSelector() *goalSelector {
	return &goalSelector{locked: make(map[GoalFlag]*wrappedGoal, 4)}
}

// add registers a goal at the given priority. Insertion order doesn't matter;
// tick() always scans by priority.
func (gs *goalSelector) add(priority int, g Goal) {
	gs.goals = append(gs.goals, &wrappedGoal{priority: priority, goal: g})
}

// empty reports whether the selector has no goals (lets aiStep skip the scan).
func (gs *goalSelector) empty() bool { return gs == nil || len(gs.goals) == 0 }

// flagsFree reports whether every flag in `want` is either unlocked or locked
// by a goal that `cand` is allowed to evict (lower importance AND
// interruptible). When evictable, the holders are returned so the caller can
// stop them.
func (gs *goalSelector) flagsAvailable(cand *wrappedGoal, want GoalFlag) (ok bool, evict []*wrappedGoal) {
	for _, f := range allFlags {
		if want&f == 0 {
			continue
		}
		holder, taken := gs.locked[f]
		if !taken {
			continue
		}
		if holder == cand {
			continue
		}
		// Occupied by another goal. We may take it only if we outrank the
		// holder (smaller priority number) and the holder is interruptible.
		if cand.priority < holder.priority && goalInterruptible(holder.goal) {
			evict = append(evict, holder)
			continue
		}
		return false, nil
	}
	return true, evict
}

// lock marks every flag in `want` as owned by `wg`.
func (gs *goalSelector) lock(wg *wrappedGoal, want GoalFlag) {
	for _, f := range allFlags {
		if want&f != 0 {
			gs.locked[f] = wg
		}
	}
}

// release frees every flag currently owned by `wg`.
func (gs *goalSelector) release(wg *wrappedGoal) {
	for _, f := range allFlags {
		if gs.locked[f] == wg {
			delete(gs.locked, f)
		}
	}
}

// stopGoal transitions a running goal to idle, firing Stop and freeing flags.
func (gs *goalSelector) stopGoal(wg *wrappedGoal, m *Mob, ctx *AIContext) {
	if !wg.running {
		return
	}
	wg.running = false
	gs.release(wg)
	wg.goal.Stop(m, ctx)
}

// tick runs one selection + execution pass, mirroring vanilla GoalSelector.tick:
//  1. Stop running goals that can no longer continue (or lost a flag).
//  2. For each idle goal in priority order, start it if its flags are free
//     (evicting lower-priority interruptible holders).
//  3. Tick every running goal.
func (gs *goalSelector) tick(m *Mob, ctx *AIContext) {
	if gs == nil {
		return
	}
	// Phase 1: stop goals that should no longer run.
	for _, wg := range gs.goals {
		if wg.running && !wg.goal.CanContinue(m, ctx) {
			gs.stopGoal(wg, m, ctx)
		}
	}
	// Phase 2: try to start idle goals, lowest priority number first.
	for _, wg := range gs.byPriority() {
		if wg.running {
			continue
		}
		if !wg.goal.CanUse(m, ctx) {
			continue
		}
		want := wg.goal.Flags()
		avail, evict := gs.flagsAvailable(wg, want)
		if !avail {
			continue
		}
		for _, h := range evict {
			gs.stopGoal(h, m, ctx)
		}
		wg.running = true
		gs.lock(wg, want)
		wg.goal.Start(m, ctx)
	}
	// Phase 3: tick running goals.
	for _, wg := range gs.goals {
		if wg.running {
			wg.goal.Tick(m, ctx)
		}
	}
}

// byPriority returns the goals sorted ascending by priority. The list is tiny
// (≤ ~10 goals per mob) so an insertion sort into a scratch slice is cheaper
// and allocation-lighter than sort.Slice with a closure.
func (gs *goalSelector) byPriority() []*wrappedGoal {
	out := append(gs.scratch[:0], gs.goals...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].priority < out[j-1].priority; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	gs.scratch = out
	return out
}

// stopAll force-stops every running goal (used when the mob dies/despawns or
// the selector is rebuilt). Currently unused by the hot path but kept for the
// brain/anger transitions in Milestone C.
func (gs *goalSelector) stopAll(m *Mob, ctx *AIContext) {
	if gs == nil {
		return
	}
	for _, wg := range gs.goals {
		gs.stopGoal(wg, m, ctx)
	}
}
