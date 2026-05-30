package anticheat

import (
	"testing"

	"livingworld/internal/registry"
)

func TestSpeedCheck(t *testing.T) {
	c := SpeedCheck{}
	// 7 b/s walking exceeds 4.3*1.3=5.59 -> flagged with setback.
	if r := c.Inspect(&PlayerCtx{}, NewMoveEvent(registry.Vec3{}, registry.Vec3{Z: 7}, 1)); r.Vio <= 0 || r.Mitigate != MitigateSetback {
		t.Fatalf("want speed violation, got %+v", r)
	}
	// 4 b/s walking is clean.
	if r := c.Inspect(&PlayerCtx{}, NewMoveEvent(registry.Vec3{}, registry.Vec3{Z: 4}, 1)); r.Vio != 0 {
		t.Fatalf("want clean walk, got %+v", r)
	}
	// sprinting raises the cap; 6 b/s is allowed.
	if r := c.Inspect(&PlayerCtx{Sprinting: true}, NewMoveEvent(registry.Vec3{}, registry.Vec3{Z: 6}, 1)); r.Vio != 0 {
		t.Fatalf("want sprint allowed, got %+v", r)
	}
}

func TestReachCheck(t *testing.T) {
	c, ctx := ReachCheck{}, &PlayerCtx{EyeHeight: 1.62}
	if r := c.Inspect(ctx, NewAttackEvent(registry.Vec3{X: 5, Y: 1.62})); r.Vio <= 0 || r.Mitigate != MitigateCancel {
		t.Fatalf("want reach violation, got %+v", r)
	}
	if r := c.Inspect(ctx, NewAttackEvent(registry.Vec3{X: 2, Y: 1.62})); r.Vio != 0 {
		t.Fatalf("want clean reach, got %+v", r)
	}
}

func TestEngineStagedAction(t *testing.T) {
	e := New(Config{Enabled: true, DecayPerTick: 1, Checks: map[string]CheckConfig{"Speed": {Setback: 1, Kick: 2}}})
	e.Register(SpeedCheck{})
	ctx := &PlayerCtx{UUID: "p1", TPS: 20} // compensation = 1.0
	move := NewMoveEvent(registry.Vec3{}, registry.Vec3{Z: 7}, 1) // ~1.41 overage/call

	if out := e.Handle(ctx, move); len(out) != 1 || out[0].Action != ActionSetback {
		t.Fatalf("call1 want Setback, got %+v", out)
	}
	if out := e.Handle(ctx, move); len(out) != 1 || out[0].Action != ActionKick {
		t.Fatalf("call2 want Kick, got %+v", out)
	}
}

func TestExemptAndDisabled(t *testing.T) {
	move := NewMoveEvent(registry.Vec3{}, registry.Vec3{Z: 99}, 1)

	e := New(Config{Enabled: true})
	e.Register(SpeedCheck{})
	if out := e.Handle(&PlayerCtx{UUID: "p", Exempt: true}, move); out != nil {
		t.Fatalf("exempt player must not be flagged, got %+v", out)
	}
	if out := New(Config{Enabled: false}).Handle(&PlayerCtx{UUID: "p"}, move); out != nil {
		t.Fatalf("disabled engine must no-op, got %+v", out)
	}
}


func TestTimerCheck(t *testing.T) {
	c := TimerCheck{}
	fast := NewMoveEvent(registry.Vec3{}, registry.Vec3{}, 0.05)
	fast.RealDT = 0.02 // client time runs 2.5x wall-clock
	if r := c.Inspect(&PlayerCtx{}, fast); r.Vio <= 0 {
		t.Fatalf("want timer violation, got %+v", r)
	}
	ok := NewMoveEvent(registry.Vec3{}, registry.Vec3{}, 0.05)
	ok.RealDT = 0.05
	if r := c.Inspect(&PlayerCtx{}, ok); r.Vio != 0 {
		t.Fatalf("want clean timer, got %+v", r)
	}
}

func TestAutoclickerCheck(t *testing.T) {
	c := AutoclickerCheck{}
	fast := NewAttackEvent(registry.Vec3{})
	fast.Interval = 0.04 // 25 CPS
	if r := c.Inspect(&PlayerCtx{}, fast); r.Vio <= 0 {
		t.Fatalf("want autoclicker violation, got %+v", r)
	}
	human := NewAttackEvent(registry.Vec3{})
	human.Interval = 0.1 // 10 CPS
	if r := c.Inspect(&PlayerCtx{}, human); r.Vio != 0 {
		t.Fatalf("want clean cps, got %+v", r)
	}
}

func TestKillAuraCheck(t *testing.T) {
	c := KillAuraCheck{}
	ctx := &PlayerCtx{EyeHeight: 1.62} // yaw=pitch=0 -> looking +Z
	if r := c.Inspect(ctx, NewAttackEvent(registry.Vec3{Y: 1.62, Z: -2})); r.Vio <= 0 || r.Mitigate != MitigateCancel {
		t.Fatalf("want killaura violation (target behind), got %+v", r)
	}
	if r := c.Inspect(ctx, NewAttackEvent(registry.Vec3{Y: 1.62, Z: 2})); r.Vio != 0 {
		t.Fatalf("want clean aim (target ahead), got %+v", r)
	}
}
