package anticheat

import "livingworld/internal/registry"

// event is the shared base for anticheat-inspected gameplay events. It mirrors
// the plugin event surface (Name/Cancellable/Cancel) without coupling to that
// lane's package; an adapter maps plugin events onto these when the anticheat
// runs as a privileged plugin.
type event struct {
	name        string
	cancellable bool
	cancelled   bool
}

func (e *event) Name() string      { return e.name }
func (e *event) Cancellable() bool { return e.cancellable }
func (e *event) Cancel()           { e.cancelled = true }

// Cancelled reports whether a mitigation cancelled the event.
func (e *event) Cancelled() bool { return e.cancelled }

// MoveEvent is one server-authoritative movement step.
type MoveEvent struct {
	event
	From, To registry.Vec3
	DT       float64 // client-claimed seconds for this step (~one tick)
	RealDT   float64 // server-measured wall-clock seconds since previous move (Timer)
}

func NewMoveEvent(from, to registry.Vec3, dt float64) *MoveEvent {
	return &MoveEvent{event: event{name: "move", cancellable: true}, From: from, To: to, DT: dt}
}

// AttackEvent is a melee attack; TargetPos is the closest hitbox point in
// canonical coordinates.
type AttackEvent struct {
	event
	TargetPos registry.Vec3
	Interval  float64 // server-measured seconds since this player's previous attack (Autoclicker)
}

func NewAttackEvent(target registry.Vec3) *AttackEvent {
	return &AttackEvent{event: event{name: "attack", cancellable: true}, TargetPos: target}
}
