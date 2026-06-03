package server

import (
	"testing"

	"livingworld/internal/world"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// TestM6_BedrockMobEffectPacket_Add pins the field values of a
// packet.MobEffect with Operation=MobEffectAdd. v1 effects always
// set Particles=true (so the client renders the swirly overlay)
// and Ambient=false (v1 effects come from mob hits, never
// beacons / conduits).
func TestM6_BedrockMobEffectPacket_Add(t *testing.T) {
	ev := world.WorldEffectEvent{
		Kind:     world.EffectStatus,
		EffectID: 19, // poison
		Data:     0,  // amplifier 0 = level I
		Aux:      200, // 200 ticks = 10 s
	}
	pk := buildMobEffectPacket(42, ev, packet.MobEffectAdd)
	if pk.EntityRuntimeID != 42 {
		t.Errorf("EntityRuntimeID = %d, want 42", pk.EntityRuntimeID)
	}
	if pk.Operation != packet.MobEffectAdd {
		t.Errorf("Operation = %d, want %d", pk.Operation, packet.MobEffectAdd)
	}
	if pk.EffectType != 19 {
		t.Errorf("EffectType = %d, want 19", pk.EffectType)
	}
	if pk.Amplifier != 0 {
		t.Errorf("Amplifier = %d, want 0", pk.Amplifier)
	}
	if pk.Duration != 10 {
		t.Errorf("Duration = %d, want 10 (200 ticks → 10 s)", pk.Duration)
	}
	if !pk.Particles {
		t.Errorf("Particles = false, want true")
	}
	if pk.Ambient {
		t.Errorf("Ambient = true, want false (v1 effects are mob-sourced)")
	}
	if pk.Tick != 0 {
		t.Errorf("Tick = %d, want 0", pk.Tick)
	}
}

// TestM6_BedrockMobEffectPacket_Add_HungerEffectId verifies the
// hunger id (17) flows through with the same wire semantics as
// the Bedrock EffectHunger constant. The bridge does not import
// the EffectHunger symbol; the lookup is by EffectID alone.
func TestM6_BedrockMobEffectPacket_Add_HungerEffectId(t *testing.T) {
	ev := world.WorldEffectEvent{
		Kind:     world.EffectStatus,
		EffectID: 17, // hunger
		Data:     0,
		Aux:      1000,
	}
	pk := buildMobEffectPacket(1, ev, packet.MobEffectAdd)
	if pk.EffectType != 17 {
		t.Errorf("EffectType = %d, want 17", pk.EffectType)
	}
	if pk.Duration != 50 {
		t.Errorf("Duration = %d, want 50 (1000 ticks → 50 s)", pk.Duration)
	}
}

// TestM6_BedrockMobEffectPacket_Remove verifies the remove path:
// Duration, Amplifier, and Particles are all zero (the wire
// fields are unused on a remove), but EffectType and runtime
// id must still be set so the client knows which effect to drop.
func TestM6_BedrockMobEffectPacket_Remove(t *testing.T) {
	ev := world.WorldEffectEvent{
		Kind:     world.EffectStatusRemove,
		EffectID: 19,
	}
	pk := buildMobEffectPacket(7, ev, packet.MobEffectRemove)
	if pk.EntityRuntimeID != 7 {
		t.Errorf("EntityRuntimeID = %d, want 7", pk.EntityRuntimeID)
	}
	if pk.Operation != packet.MobEffectRemove {
		t.Errorf("Operation = %d, want %d", pk.Operation, packet.MobEffectRemove)
	}
	if pk.EffectType != 19 {
		t.Errorf("EffectType = %d, want 19", pk.EffectType)
	}
	if pk.Duration != 0 {
		t.Errorf("Duration = %d, want 0 (remove ignores duration)", pk.Duration)
	}
	if pk.Amplifier != 0 {
		t.Errorf("Amplifier = %d, want 0 (remove ignores amplifier)", pk.Amplifier)
	}
	if pk.Particles {
		t.Errorf("Particles = true, want false (remove ignores particles)")
	}
	if pk.Ambient {
		t.Errorf("Ambient = true, want false")
	}
}

// TestM6_BedrockMobEffectPacket_ConstantsMatchJava verifies the
// gophertunnel packet.MobEffect operation constants are
// 1/2/3 (Add/Modify/Remove). This catches a regression if
// gophertunnel ever renumbers them.
func TestM6_BedrockMobEffectPacket_ConstantsMatchJava(t *testing.T) {
	if packet.MobEffectAdd != 1 {
		t.Errorf("packet.MobEffectAdd = %d, want 1", packet.MobEffectAdd)
	}
	if packet.MobEffectModify != 2 {
		t.Errorf("packet.MobEffectModify = %d, want 2", packet.MobEffectModify)
	}
	if packet.MobEffectRemove != 3 {
		t.Errorf("packet.MobEffectRemove = %d, want 3", packet.MobEffectRemove)
	}
}
