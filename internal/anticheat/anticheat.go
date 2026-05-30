// Package anticheat is LivingWorld's official, server-authoritative anticheat
// engine (R13, DESIGN §9): an edition-agnostic violation model + pure Check
// interface (this file), a per-player weighted Profile with decay (profile.go),
// the aggregation/staged-action Engine (engine.go), and anticheat.yml config
// (config.go). Concrete checks build on the canonical model (internal/registry)
// in checks.go.
package anticheat

import "livingworld/internal/registry"

// Mitigation is the corrective action a Check requests for a single event.
type Mitigation uint8

const (
	MitigateNone     Mitigation = iota // no correction
	MitigateCancel                     // cancel the offending action/event
	MitigateSetback                    // rewind the player to the last valid state
	MitigateVelocity                   // apply a corrective velocity
)

// Action is a staged enforcement step taken when a player's weighted violation
// score for a check crosses a configured threshold (log→warn→setback→kick→ban).
type Action uint8

const (
	ActionLog Action = iota
	ActionWarn
	ActionSetback
	ActionKick
	ActionBan
)

func (a Action) String() string {
	switch a {
	case ActionWarn:
		return "warn"
	case ActionSetback:
		return "setback"
	case ActionKick:
		return "kick"
	case ActionBan:
		return "ban"
	default:
		return "log"
	}
}

// Event is a gameplay event a Check inspects. It mirrors the plugin event
// surface (DESIGN §7) so the anticheat can run as a privileged plugin without
// coupling to that lane's package yet.
type Event interface {
	Name() string
	Cancellable() bool
	Cancel()
}

// PlayerCtx is a server-authoritative snapshot of the player under inspection,
// built on the canonical model (registry types) so checks read one source of
// truth across editions.
type PlayerCtx struct {
	UUID      string
	Name      string
	Pos       registry.Vec3 // server-authoritative position (feet)
	EyeHeight float64       // eye offset above Pos, for reach geometry
	Yaw       float64       // look yaw in degrees (Minecraft convention)
	Pitch     float64       // look pitch in degrees
	Sprinting bool
	OnGround  bool
	LatencyMS int     // round-trip latency, for lag compensation
	TPS       float64 // current server TPS, for tick-rate compensation
	Exempt    bool    // operator/admin exemption (/ac exempt)
}

// CheckResult is the verdict of a single Check for a single event. Checks are
// pure: they compute a violation weight and an optional mitigation but never
// mutate state.
type CheckResult struct {
	Vio      float64    // raw violation weight (0 = clean)
	Reason   string     // human-readable detail for logs/alerts
	Mitigate Mitigation // requested correction
}

// Check is one detection heuristic. Implementations MUST be pure (no side
// effects); the Engine owns aggregation, decay and enforcement.
type Check interface {
	Name() string
	Inspect(ctx *PlayerCtx, ev Event) CheckResult
}
