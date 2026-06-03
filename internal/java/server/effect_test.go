package server

import (
	"testing"
)

// TestM6_JavaUpdateMobEffectPacket_Layout pins the wire bytes for
// the add-effect packet.
//
// Wire layout (Java 1.21):
//   packet id 132   (VarInt: 0x84 0x01)
//   entity id 1     (VarInt: 0x01)
//   effect id 19    (VarInt: 0x13)  ← EffectPoison
//   amplifier 0     (VarInt: 0x00)
//   duration 200    (VarInt: 0xC8 0x01)
//   flags 0x06      (Byte: 0x06)
func TestM6_JavaUpdateMobEffectPacket_Layout(t *testing.T) {
	want := []byte{0x84, 0x01, 0x01, 0x13, 0x00, 0xC8, 0x01, 0x06}
	got, err := pkMarshal(javaUpdateMobEffectPacket(1, 19, 0, 200))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Errorf("wire bytes mismatch:\n got  %v\n want %v", got, want)
	}
}

// TestM6_JavaUpdateMobEffectPacket_HungerEffectId verifies the
// hunger id (17) — a single-byte VarInt in the wire. Amplifier
// 0 = level I. This catches a regression where the bridge uses
// the Bedrock EffectHunger (=17) on the wrong end of a swap.
func TestM6_JavaUpdateMobEffectPacket_HungerEffectId(t *testing.T) {
	want := []byte{0x84, 0x01, 0x01, 0x11, 0x00, 0xE8, 0x07, 0x06}
	got, err := pkMarshal(javaUpdateMobEffectPacket(1, 17, 0, 1000))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Errorf("wire bytes mismatch:\n got  %v\n want %v", got, want)
	}
}

// TestM6_JavaUpdateMobEffectPacket_MultiByteEffectId verifies
// the wire layout for a multi-byte VarInt effect id. The
// chosen id (128 = 0x80) needs 2 VarInt bytes. The amplifier
// and duration fields use single-byte VarInts; we verify the
// full layout doesn't lose a byte at the multi-byte boundary.
func TestM6_JavaUpdateMobEffectPacket_MultiByteEffectId(t *testing.T) {
	want := []byte{0x84, 0x01, 0x01, 0x80, 0x01, 0x00, 0x01, 0x06}
	got, err := pkMarshal(javaUpdateMobEffectPacket(1, 128, 0, 1))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Errorf("wire bytes mismatch:\n got  %v\n want %v", got, want)
	}
}

// TestM6_JavaRemoveMobEffectPacket_Layout pins the wire bytes for
// the remove-effect packet.
//
// Wire layout (Java 1.21):
//   packet id 78   (VarInt: 0x4E)
//   entity id 1    (VarInt: 0x01)
//   effect id 19   (VarInt: 0x13)
func TestM6_JavaRemoveMobEffectPacket_Layout(t *testing.T) {
	want := []byte{0x4E, 0x01, 0x13}
	got, err := pkMarshal(javaRemoveMobEffectPacket(1, 19))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Errorf("wire bytes mismatch:\n got  %v\n want %v", got, want)
	}
}
