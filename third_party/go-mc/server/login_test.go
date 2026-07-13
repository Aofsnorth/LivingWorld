package server

import (
	"bytes"
	"testing"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/yggdrasil/user"
)

func makeLoginConn(t *testing.T, protocol int32) (*bytes.Buffer, *net.Conn) {
	t.Helper()
	input := new(bytes.Buffer)
	write := func(p pk.Packet) {
		if err := p.Pack(input, -1); err != nil {
			t.Fatalf("pack setup packet: %v", err)
		}
	}
	write(pk.Marshal(
		packetid.ServerboundLoginHello,
		pk.String("TestPlayer"),
		pk.UUID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
	))
	write(pk.Marshal(
		packetid.ServerboundLoginLoginAcknowledged,
	))

	output := new(bytes.Buffer)
	conn := &net.Conn{
		Reader: input,
		Writer: output,
	}
	conn.SetThreshold(-1)

	handler := &MojangLoginHandler{
		OnlineMode: false,
		Threshold:  -1,
	}

	name, _, _, _, err := handler.AcceptLogin(conn, protocol)
	if err != nil {
		t.Fatalf("AcceptLogin failed: %v", err)
	}
	if name != "TestPlayer" {
		t.Fatalf("unexpected player name: %s", name)
	}
	return output, conn
}

func TestLoginSuccessIncludesStrictErrorHandling(t *testing.T) {
	output, _ := makeLoginConn(t, 776)

	var p pk.Packet
	if err := p.UnPack(output, -1); err != nil {
		t.Fatalf("UnPack LoginSuccess: %v", err)
	}

	var uid pk.UUID
	var uname pk.String
	var props []user.Property
	var strict pk.Boolean
	if err := p.Scan(
		&uid,
		&uname,
		pk.Ary[pk.VarInt]{Ary: &props},
		&strict,
	); err != nil {
		t.Fatalf("Scan LoginSuccess: %v", err)
	}

	if uname != "TestPlayer" {
		t.Fatalf("unexpected username in LoginSuccess: %s", uname)
	}
	// The critical field added for 1.21.5+.
	if strict != false {
		t.Fatalf("expected strict error handling to be false, got %v", strict)
	}
}

func TestLoginSuccessOmitsStrictErrorHandlingForOldProtocol(t *testing.T) {
	output, _ := makeLoginConn(t, 770)

	var p pk.Packet
	if err := p.UnPack(output, -1); err != nil {
		t.Fatalf("UnPack LoginSuccess: %v", err)
	}

	var uid pk.UUID
	var uname pk.String
	var props []user.Property
	var strict pk.Boolean
	if err := p.Scan(
		&uid,
		&uname,
		pk.Ary[pk.VarInt]{Ary: &props},
	); err != nil {
		t.Fatalf("Scan LoginSuccess: %v", err)
	}

	if uname != "TestPlayer" {
		t.Fatalf("unexpected username in LoginSuccess: %s", uname)
	}

	// A pre-776 client must not receive the strictErrorHandling field.
	if err := p.Scan(&uid, &uname, pk.Ary[pk.VarInt]{Ary: &props}, &strict); err == nil {
		t.Fatalf("LoginSuccess for protocol 770 should not contain strictErrorHandling field")
	}
}
