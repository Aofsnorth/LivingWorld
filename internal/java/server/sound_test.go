package server

import (
	"livingworld/internal/mobs"
	"testing"
)

// TestM7_SoundEntity_SoundSourceInRange is a regression for
// the M0.7→M7 port bug where the SoundSource VarInt was
// written as 20 (Hostile) or 11 (Ambient) — values that
// matched an older protocol (1.8) and crash the 1.21 client
// with ArrayIndexOutOfBoundsException at
// FriendlyByteBuf.readEnum (line 475) because the modern
// SoundSource enum has only 11 entries (0-10).
//
// We assert: for every sound name the mob-sound emitter
// currently produces, the SoundSource VarInt written by
// buildSoundEntityPacket is in [0, 10]. The fix landed
// values 5 (HOSTILE) and 8 (AMBIENT) in soundCategoryFor.
func TestM7_SoundEntity_SoundSourceInRange(t *testing.T) {
	emits := []mobs.SoundEmit{
		{Sound: mobs.SoundMobZombieSay},
		{Sound: mobs.SoundMobSkeletonSay},
		{Sound: mobs.SoundMobCreeperSay},
		{Sound: mobs.SoundMobShoot},
		{Sound: mobs.SoundMobCowSay},
		{Sound: mobs.SoundMobPigSay},
		{Sound: mobs.SoundMobSheepSay},
		{Sound: mobs.SoundMobChickenSay},
		{Sound: mobs.SoundMobHurt},
		{Sound: mobs.SoundMobDeath},
	}
	for _, e := range emits {
		src := soundCategoryFor(e)
		if src < 0 || src > 10 {
			t.Errorf("sound %q: SoundSource=%d, out of [0,10] (1.21 enum)", e.Sound, src)
		}
	}
}

// TestM7_SoundEntity_HostileSendsFive asserts a hostile mob
// sound's packet contains a SoundSource VarInt of 5 (the
// 1.21 HOSTILE entry). This is the actual value the client
// reads; a wrong value triggers a disconnect. We decode the
// packet and find the SoundSource VarInt at its known offset.
func TestM7_SoundEntity_HostileSendsFive(t *testing.T) {
	p := buildSoundEntityPacket(mobs.SoundEmit{
		EntityID: 1, Sound: mobs.SoundMobZombieSay, Volume: 1.0, Pitch: 1.0,
	})
	got, err := pkMarshal(p)
	if err != nil {
		t.Fatalf("pkMarshal: %v", err)
	}
	// Wire format: VarInt(id=116) | SoundEvent{Type, Name, FixedRange}
	//             | VarInt(SoundSource) | VarInt(EntityID) | Float(Vol) | Float(Pitch) | Long(Seed)
	// The SoundEvent's Identifier VarInt length-prefix + bytes
	// makes byte-precise layout fragile across go-mc versions,
	// so we walk the packet: after the header (1 byte id 0x74),
	// the next bytes are the SoundEvent's Type VarInt=0x00,
	// then the Identifier (length-prefixed string), then the
	// Optional<Float> FixedRange (presence=0x00), then the
	// SoundSource VarInt.
	//
	// We don't lock byte positions; instead, find the entity id
	// VarInt (1) at the end of the VarInt section and read the
	// SoundSource VarInt immediately before it. VarInt(1) is
	// the single byte 0x01.
	idx := findByteReverse(got, 0x01)
	if idx < 0 {
		t.Fatalf("VarInt(entity=1) byte not found in packet %x", got)
	}
	// The SoundSource VarInt is the byte at idx-1. For a value
	// of 5, that byte is 0x05.
	srcByte := got[idx-1]
	if srcByte != 0x05 {
		t.Errorf("SoundSource byte = 0x%02x, want 0x05 (HOSTILE); full=%x", srcByte, got)
	}
}

// TestM7_SoundEntity_AmbientSendsEight asserts a passive mob
// sound's packet contains a SoundSource VarInt of 8 (the 1.21
// AMBIENT entry). Mirror of HostileSendsFive.
func TestM7_SoundEntity_AmbientSendsEight(t *testing.T) {
	p := buildSoundEntityPacket(mobs.SoundEmit{
		EntityID: 1, Sound: mobs.SoundMobCowSay, Volume: 1.0, Pitch: 1.0,
	})
	got, err := pkMarshal(p)
	if err != nil {
		t.Fatalf("pkMarshal: %v", err)
	}
	idx := findByteReverse(got, 0x01)
	if idx < 0 {
		t.Fatalf("VarInt(entity=1) byte not found in packet %x", got)
	}
	srcByte := got[idx-1]
	if srcByte != 0x08 {
		t.Errorf("SoundSource byte = 0x%02x, want 0x08 (AMBIENT); full=%x", srcByte, got)
	}
}

// findByteReverse returns the highest index of `b` in `s`, or
// -1 if absent. Used by the SoundSource decoder test to walk
// backward from the entity-id VarInt and grab the immediately
// preceding byte.
func findByteReverse(s []byte, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}
