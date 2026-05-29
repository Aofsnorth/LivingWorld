package protocol

import (
	"testing"

	"livingworld/internal/player"
)

// TestEncodePlayerMetadataSkinPartsIndex guards against the MC 26.1 crash:
//
//	Invalid entity data item type for field 17 ... old=0.0(Float), new=127(Byte)
//
// In 26.1 the Avatar class shifted Displayed Skin Parts to index 16; index 17 is
// now the absorption Float. Sending skin parts (a Byte) at index 17 crashes the
// client. This test asserts skin parts are emitted at index 16 with the Byte type,
// and that index 17 never appears in the player metadata we send.
func TestEncodePlayerMetadataSkinPartsIndex(t *testing.T) {
	p := player.PlayerSnapshot{
		EntityRuntimeID: 42,
		SkinParts:       0x7F,
	}

	data := encodePlayerMetadata(p)

	// Skip the leading entityID VarInt (42 fits in one byte).
	fields := data[1:]

	got := scanMetadataFields(t, fields)

	skin, ok := got[MetaIndexSkinParts]
	if !ok {
		t.Fatalf("skin parts not emitted at index %d; fields=%v", MetaIndexSkinParts, got)
	}
	if skin.typ != MetaTypeByte {
		t.Errorf("skin parts type = %d, want Byte (%d)", skin.typ, MetaTypeByte)
	}
	if skin.firstByte != 0x7F {
		t.Errorf("skin parts value = %d, want 0x7F", skin.firstByte)
	}

	if _, bad := got[17]; bad {
		t.Errorf("index 17 must not be written for a player (it is the absorption Float in 26.1)")
	}
}

func TestEncodePlayerMetadataSneakFlags(t *testing.T) {
	p := player.PlayerSnapshot{EntityRuntimeID: 7, Sneaking: true}
	got := scanMetadataFields(t, encodePlayerMetadata(p)[1:])

	flags, ok := got[MetaIndexFlags]
	if !ok || flags.typ != MetaTypeByte {
		t.Fatalf("flags missing or wrong type: %+v", flags)
	}
	if flags.firstByte&EntityFlagSneaking == 0 {
		t.Errorf("sneaking flag not set; flags=0x%02x", flags.firstByte)
	}

	pose, ok := got[MetaIndexPose]
	if !ok || pose.typ != MetaTypePose {
		t.Fatalf("pose missing or wrong type: %+v", pose)
	}
	if pose.firstByte != PoseCrouching {
		t.Errorf("pose = %d, want crouching (%d)", pose.firstByte, PoseCrouching)
	}
}

type metaField struct {
	typ       int
	firstByte byte
}

// scanMetadataFields walks the entity-metadata wire format: repeating
// (index:UByte, type:VarInt, value...) until the 0xFF terminator. It records the
// type and first value byte per index. It only knows how to advance past the value
// types this codec emits (Byte and Pose/VarInt), which is sufficient for these tests.
func scanMetadataFields(t *testing.T, b []byte) map[byte]metaField {
	t.Helper()
	out := make(map[byte]metaField)
	i := 0
	for i < len(b) {
		index := b[i]
		i++
		if index == 0xFF {
			return out
		}
		typ, n := readVarInt(b[i:])
		i += n
		if i >= len(b) {
			t.Fatalf("metadata truncated reading value for index %d", index)
		}
		out[index] = metaField{typ: int(typ), firstByte: b[i]}
		switch typ {
		case MetaTypeByte:
			i++ // single byte value
		case MetaTypePose:
			_, vn := readVarInt(b[i:])
			i += vn
		default:
			t.Fatalf("scanner does not know how to skip value type %d at index %d", typ, index)
		}
	}
	t.Fatalf("metadata not terminated by 0xFF")
	return out
}

func readVarInt(b []byte) (int32, int) {
	var val int32
	var n int
	for {
		if n >= len(b) {
			return val, n
		}
		cur := b[n]
		val |= int32(cur&0x7F) << (7 * n)
		n++
		if cur&0x80 == 0 {
			break
		}
	}
	return val, n
}
