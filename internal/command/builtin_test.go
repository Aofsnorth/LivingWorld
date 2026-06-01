package command

import (
	"strings"
	"testing"

	"livingworld/internal/player"

	"github.com/google/uuid"
)

// fakeSender records every Reply so the test can assert on the rendered
// version banner without standing up a protocol session.
type fakeSender struct {
	name    string
	ed      player.Edition
	isOp    bool
	replies []string
}

func (f *fakeSender) Name() string                              { return f.name }
func (f *fakeSender) UUID() uuid.UUID                           { return uuid.MustParse("00000000-0000-0000-0000-000000000000") }
func (f *fakeSender) IsOp() bool                                { return f.isOp }
func (f *fakeSender) Edition() player.Edition                   { return f.ed }
func (f *fakeSender) Reply(msg string)                          { f.replies = append(f.replies, msg) }
func (f *fakeSender) SetGameMode(int) error                     { return nil }
func (f *fakeSender) Teleport(float64, float64, float64) error { return nil }
func (f *fakeSender) GiveItem(string, int) error                { return nil }

// TestLWVersionBanner: the /lwversion command must surface the current
// version label and both protocol numbers, and it must be reachable by
// non-OPs (it's a diagnostic, not a mutation).
func TestLWVersionBanner(t *testing.T) {
	// Use the player package's Java sentinel directly; the test only
	// checks banner contents, not edition semantics.
	s := &fakeSender{name: "tester", ed: player.EditionJava, isOp: false}
	r := New(nil, nil)
	RegisterBuiltins(r)
	if _, ok := r.Get("lwversion"); !ok {
		t.Fatalf("lwversion command not registered")
	}
	if !r.Dispatch(s, "lwversion") {
		t.Fatalf("Dispatch(lwversion) returned handled=false")
	}
	joined := strings.Join(s.replies, "\n")
	// The banner should mention the current version label and both
	// protocol numbers. We don't pin the exact wording (the matrix can
	// change), only the data points a player needs to diagnose a
	// wrong-version client.
	for _, want := range []string{"26 (A)", "775", "975", "Java protocol:", "Bedrock protocol:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("/lwversion banner missing %q\nbanner:\n%s", want, joined)
		}
	}
}

// TestLWVersionNonOP: any player should be able to run /lwversion; gating it
// behind PermOperator would break the "version handshake" diagnosis flow
// where a player gets disconnected and wants to know why.
func TestLWVersionNonOP(t *testing.T) {
	s := &fakeSender{name: "tester", isOp: false}
	r := New(nil, nil)
	RegisterBuiltins(r)
	r.Dispatch(s, "lwversion")
	for _, line := range s.replies {
		if strings.Contains(line, "permission") {
			t.Errorf("non-OP got permission error: %s", line)
		}
	}
}
