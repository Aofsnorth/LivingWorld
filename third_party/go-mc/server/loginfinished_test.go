package server

import (
	"bytes"
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/yggdrasil/user"

	"github.com/google/uuid"
)

// decodeLoginFinished26_3 decodes a clientbound Login Finished packet using the
// layout taken straight from the 26.3 server jar, so the test checks our
// encoder against an independently derived decoder rather than against itself.
//
//	ClientboundLoginFinishedPacket.STREAM_CODEC = StreamCodec.composite(
//	    ByteBufCodecs.GAME_PROFILE,  ...::gameProfile,
//	    UUIDUtil.STREAM_CODEC,       ...::sessionId, ...)
//
//	GAME_PROFILE = composite(UUID, PLAYER_NAME, GAME_PROFILE_PROPERTIES)
//	GAME_PROFILE_PROPERTIES = readCount, then per entry:
//	    Utf8String name, Utf8String value, readNullable signature
func decodeLoginFinished26_3(t *testing.T, data []byte) (id uuid.UUID, name string, props []user.Property, sessionID uuid.UUID, err error) {
	t.Helper()
	r := bytes.NewReader(data)

	var wID pk.UUID
	if _, err = wID.ReadFrom(r); err != nil {
		return
	}
	id = uuid.UUID(wID)

	var wName pk.String
	if _, err = wName.ReadFrom(r); err != nil {
		return
	}
	name = string(wName)

	var count pk.VarInt
	if _, err = count.ReadFrom(r); err != nil {
		return
	}
	if count < 0 {
		t.Fatalf("negative property count %d", count)
	}
	for i := int32(0); i < int32(count); i++ {
		var propName, propValue pk.String
		if _, err = (pk.Tuple{&propName, &propValue}).ReadFrom(r); err != nil {
			return
		}
		var sig pk.Option[pk.String, *pk.String]
		if _, err = sig.ReadFrom(r); err != nil {
			return
		}
		p := user.Property{Name: string(propName), Value: string(propValue)}
		if sig.Has {
			p.Signature = string(sig.Val)
		}
		props = append(props, p)
	}

	var wSession pk.UUID
	if _, err = wSession.ReadFrom(r); err != nil {
		return
	}
	sessionID = uuid.UUID(wSession)
	return
}

func TestLoginFinished26_3Layout(t *testing.T) {
	profileID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	session := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	props := []user.Property{
		{Name: "textures", Value: "dGV4dHVyZQ==", Signature: "sig1"},
		{Name: "unsigned", Value: "dmFsdWU="},
	}

	p := pk.Marshal(2, loginFinishedFields(profileID, "Steve", props, session, 777)...)
	if p.ID != 2 {
		t.Fatalf("packet id = %d, want 2 (login_finished)", p.ID)
	}

	gotID, gotName, gotProps, gotSession, err := decodeLoginFinished26_3(t, p.Data)
	if err != nil {
		t.Fatalf("26.3 decoder rejected our encoding: %v", err)
	}
	if gotID != profileID {
		t.Errorf("profile uuid = %v, want %v", gotID, profileID)
	}
	if gotName != "Steve" {
		t.Errorf("name = %q, want %q", gotName, "Steve")
	}
	if gotSession != session {
		t.Errorf("session id = %v, want %v", gotSession, session)
	}
	if len(gotProps) != len(props) {
		t.Fatalf("got %d properties, want %d", len(gotProps), len(props))
	}
	for i := range props {
		if gotProps[i].Name != props[i].Name || gotProps[i].Value != props[i].Value {
			t.Errorf("property %d = %+v, want %+v", i, gotProps[i], props[i])
		}
		if gotProps[i].Signature != props[i].Signature {
			t.Errorf("property %d signature = %q, want %q", i, gotProps[i].Signature, props[i].Signature)
		}
	}

	// The regression that produced "failed to decode packet clientbound login
	// finished": for 26.3 the packet must end with the 16-byte session UUID,
	// not the single-byte strictErrorHandling bool 26.2 used.
	if n := len(p.Data); n < 16 {
		t.Errorf("payload is %d bytes; too short to carry a 16-byte session uuid", n)
	}
	if tail := p.Data[len(p.Data)-16:]; !bytes.Equal(tail, session[:]) {
		t.Errorf("last 16 bytes = %x, want the session uuid %x", tail, session[:])
	}
}

// TestLoginFinishedLegacyBool guards the 26.2 branch: after the shared
// profile fields it must still append exactly one strictErrorHandling byte,
// so the version split stays honest.
func TestLoginFinishedLegacyBool(t *testing.T) {
	session := uuid.New()
	p := pk.Marshal(2, loginFinishedFields(uuid.New(), "Steve", nil, session, 776)...)

	// Skip the shared prefix: UUID (16) + String name + VarInt property count.
	r := bytes.NewReader(p.Data)
	var wID pk.UUID
	if _, err := wID.ReadFrom(r); err != nil {
		t.Fatalf("read uuid: %v", err)
	}
	var wName pk.String
	if _, err := wName.ReadFrom(r); err != nil {
		t.Fatalf("read name: %v", err)
	}
	var count pk.VarInt
	if _, err := count.ReadFrom(r); err != nil {
		t.Fatalf("read property count: %v", err)
	}

	if left := r.Len(); left != 1 {
		t.Errorf("%d trailing byte(s) after the profile, want 1 (the strictErrorHandling bool)", left)
	}
	var flag pk.Boolean
	if _, err := flag.ReadFrom(r); err != nil {
		t.Fatalf("read flag: %v", err)
	}
	if flag {
		t.Error("strictErrorHandling = true, want false")
	}
}
