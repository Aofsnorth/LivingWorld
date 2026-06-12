// Package aiselector is LivingWorld's faithful port of vanilla
// net.minecraft.world.entity.ai.goal.GoalSelector. A mob's behaviour
// is no longer a single AIState switch; instead every behaviour is a
// Goal with a numeric priority and a set of control Flags. Each tick
// the selector decides which goals may run based on priority and
// flag-mutual-exclusion, then ticks the running set.
//
// Two selectors run per mob, mirroring vanilla:
//
//   - GoalSel   : the behaviour selector (move / look / attack / wander).
//   - TargetSel : the target-acquisition selector (who to attack).
//
// They are independent so a mob can re-evaluate "who is my target"
// (FlagTarget) while a move goal keeps running (FlagMove). The split
// is exactly how vanilla keeps Mob.goalSelector and Mob.targetSelector
// apart.
//
// Lower priority number = more important (vanilla convention). A goal
// with priority 0 outranks priority 8. When two idle goals both want
// to start and contend for the same flag, the lower-priority-number
// goal wins; a running goal can be evicted only if it is Interruptible
// and is outranked.
package aiselector

import (
	// Alias the package to `context` so the Goal interface and
	// method signatures read as `*context.AIContext` (matching the
	// rest of the ai subpackages). The stdlib `context` is not
	// used in this file.
	context "livingworld/internal/mobs/ai/context"
)

// Flag is a control channel a goal occupies while running.
type Flag uint8

const (
	FlagMove Flag = 1 << iota
	FlagLook
	FlagJump
	FlagTarget
)

var allFlags = [...]Flag{FlagMove, FlagLook, FlagJump, FlagTarget}

// Goal is one unit of mob behaviour. The body is passed as `any`
// to keep the package Mob-free; the goal implementations in the
// per-kind subpackages type-assert to *mobs.Mob at the top of
// each method.
type Goal interface {
	CanUse(body any, ctx *context.AIContext) bool
	CanContinue(body any, ctx *context.AIContext) bool
	Start(body any, ctx *context.AIContext)
	Stop(body any, ctx *context.AIContext)
	Tick(body any, ctx *context.AIContext)
	Flags() Flag
}

// Interruptible is an optional interface.
type Interruptible interface {
	IsInterruptible() bool
}

// IsInterruptible returns true unless the goal explicitly opts out
// via the Interruptible interface. Mirrors the vanilla
// Goal.isInterruptable default.
func IsInterruptible(g Goal) bool {
	if ig, ok := g.(Interruptible); ok {
		return ig.IsInterruptible()
	}
	return true
}

// BaseGoal is embedded by most goals to default CanContinue→CanUse
// and to supply no-op Start/Stop.
type BaseGoal struct{}

func (BaseGoal) Start(any, *context.AIContext)    {}
func (BaseGoal) Stop(any, *context.AIContext)     {}
func (BaseGoal) CanContinue(any, *context.AIContext) bool {
	return true
}

// WrappedGoal pairs a goal with its priority and running state.
type WrappedGoal struct {
	Priority int
	Goal     Goal
	Running  bool
}

// GoalSelector holds a mob's prioritised goal list and the flag-lock
// table.
type GoalSelector struct {
	Goals   []*WrappedGoal
	Locked  map[Flag]*WrappedGoal
	Scratch []*WrappedGoal
}

// NewGoalSelector returns a fresh selector.
func NewGoalSelector() *GoalSelector {
	return &GoalSelector{Locked: make(map[Flag]*WrappedGoal, 4)}
}

// Add registers a goal at the given priority. Insertion order
// doesn't matter; Tick scans by priority.
func (gs *GoalSelector) Add(priority int, g Goal) {
	gs.Goals = append(gs.Goals, &WrappedGoal{Priority: priority, Goal: g})
}

// Empty reports whether the selector has no goals.
func (gs *GoalSelector) Empty() bool { return gs == nil || len(gs.Goals) == 0 }

// FlagsAvailable reports whether every flag in `want` is either
// unlocked or held by a goal that `cand` is allowed to evict.
func (gs *GoalSelector) FlagsAvailable(cand *WrappedGoal, want Flag) (ok bool, evict []*WrappedGoal) {
	for _, f := range allFlags {
		if want&f == 0 {
			continue
		}
		holder, taken := gs.Locked[f]
		if !taken {
			continue
		}
		if holder == cand {
			continue
		}
		if cand.Priority < holder.Priority && IsInterruptible(holder.Goal) {
			evict = append(evict, holder)
			continue
		}
		return false, nil
	}
	return true, nil
}

func (gs *GoalSelector) lock(wg *WrappedGoal, want Flag) {
	for _, f := range allFlags {
		if want&f != 0 {
			gs.Locked[f] = wg
		}
	}
}

func (gs *GoalSelector) release(wg *WrappedGoal) {
	for _, f := range allFlags {
		if gs.Locked[f] == wg {
			delete(gs.Locked, f)
		}
	}
}

func (gs *GoalSelector) stopGoal(wg *WrappedGoal, body any, ctx *context.AIContext) {
	if !wg.Running {
		return
	}
	wg.Running = false
	gs.release(wg)
	wg.Goal.Stop(body, ctx)
}

// Tick runs one selection + execution pass.
func (gs *GoalSelector) Tick(body any, ctx *context.AIContext) {
	if gs == nil {
		return
	}
	for _, wg := range gs.Goals {
		if wg.Running && !wg.Goal.CanContinue(body, ctx) {
			gs.stopGoal(wg, body, ctx)
		}
	}
	for _, wg := range gs.byPriority() {
		if wg.Running {
			continue
		}
		if !wg.Goal.CanUse(body, ctx) {
			continue
		}
		want := wg.Goal.Flags()
		avail, evict := gs.FlagsAvailable(wg, want)
		if !avail {
			continue
		}
		for _, h := range evict {
			gs.stopGoal(h, body, ctx)
		}
		wg.Running = true
		gs.lock(wg, want)
		wg.Goal.Start(body, ctx)
	}
	for _, wg := range gs.Goals {
		if wg.Running {
			wg.Goal.Tick(body, ctx)
		}
	}
}

func (gs *GoalSelector) byPriority() []*WrappedGoal {
	out := append(gs.Scratch[:0], gs.Goals...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Priority < out[j-1].Priority; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	gs.Scratch = out
	return out
}

// StopAll force-stops every running goal.
func (gs *GoalSelector) StopAll(body any, ctx *context.AIContext) {
	if gs == nil {
		return
	}
	for _, wg := range gs.Goals {
		gs.stopGoal(wg, body, ctx)
	}
}

// LookGoalActive reports whether a FlagLook goal is currently
// running on the selector. Used by bodyRotationSystem to avoid
// overwriting head yaw/pitch a look goal is actively controlling.
func (gs *GoalSelector) LookGoalActive() bool {
	if gs == nil {
		return false
	}
	_, ok := gs.Locked[FlagLook]
	return ok
}
