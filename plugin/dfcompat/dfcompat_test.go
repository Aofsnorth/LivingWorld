package dfcompat

import (
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/player"

	"livingworld/internal/registry"
	"livingworld/plugin"
)

// fakeItem is a minimal world.Item so tests avoid dragonfly's heavy block init.
type fakeItem struct{}

func (fakeItem) EncodeItem() (string, int16) { return "minecraft:test_item", 0 }

func TestItemBedrockRID(t *testing.T) {
	reg := registry.New()
	reg.RegisterItem("minecraft:test_item", 42)

	if id, ok := ItemBedrockRID(reg, item.NewStack(fakeItem{}, 1)); !ok || id != 42 {
		t.Fatalf("got (%d,%v), want (42,true)", id, ok)
	}
	if _, ok := ItemBedrockRID(reg, item.Stack{}); ok {
		t.Fatal("empty stack should not resolve")
	}
}

func TestBlockBedrockRID(t *testing.T) {
	reg := registry.New()
	reg.RegisterBlock(registry.BlockState(1), 7)

	if id, ok := BlockBedrockRID(reg, registry.BlockState(1)); !ok || id != 7 {
		t.Fatalf("got (%d,%v), want (7,true)", id, ok)
	}
	if _, ok := BlockBedrockRID(reg, registry.BlockState(999)); ok {
		t.Fatal("unmapped state should not resolve")
	}
}

// testHandler is an unmodified-style dragonfly handler embedding NopHandler.
type testHandler struct{ player.NopHandler }

func (testHandler) HandleBlockBreak(ctx *player.Context, _ cube.Pos, _ *[]item.Stack, _ *int) {
	ctx.Cancel()
}
func (testHandler) HandleChat(_ *player.Context, message *string) { *message = "hi" }

func TestBridgeBlockBreakCancel(t *testing.T) {
	pm := plugin.NewManager()
	New(testHandler{}).Register(pm)

	e := &plugin.BlockBreakEvent{BaseEvent: plugin.BaseEvent{Type_: plugin.EventBlockBreak}, X: 1, Y: 2, Z: 3}
	if !pm.EmitCancellable(e) {
		t.Fatal("expected dragonfly handler to cancel the break")
	}
}

func TestBridgeChatRewrite(t *testing.T) {
	pm := plugin.NewManager()
	New(testHandler{}).Register(pm)

	e := &plugin.PlayerChatEvent{BaseEvent: plugin.BaseEvent{Type_: plugin.EventPlayerChat}, Message: "x"}
	pm.Emit(e)
	if e.Message != "hi" || e.Cancelled() {
		t.Fatalf("message=%q cancelled=%v, want hi/false", e.Message, e.Cancelled())
	}
}


// fakeWorld is a minimal plugin.World for the bridge world-access test.
type fakeWorld struct{ blocks map[[3]int]int32 }

func (fakeWorld) Name() string                  { return "world" }
func (w fakeWorld) GetBlock(x, y, z int) int32  { return w.blocks[[3]int{x, y, z}] }
func (fakeWorld) SetBlock(int, int, int, int32) {}
func (fakeWorld) StateID(string) int32          { return 0 }

// fakeHost satisfies plugin.Host and plugin.WorldHost.
type fakeHost struct{ w plugin.World }

func (fakeHost) Broadcast(string)              {}
func (fakeHost) Message(string, string)        {}
func (fakeHost) Players() []string             { return nil }
func (fakeHost) PlayerCount() int              { return 0 }
func (fakeHost) GetBlock(int, int, int) int32  { return 0 }
func (fakeHost) SetBlock(int, int, int, int32) {}
func (fakeHost) StateID(string) int32          { return 0 }
func (fakeHost) Log(string, ...any)            {}
func (h fakeHost) World() plugin.World         { return h.w }

func TestBridgeWorldAccess(t *testing.T) {
	pm := plugin.NewManager()
	pm.SetHost(fakeHost{w: fakeWorld{blocks: map[[3]int]int32{{1, 2, 3}: 9}}})

	b := New(testHandler{})
	b.Register(pm)

	w := b.World()
	if w == nil {
		t.Fatal("bridge resolved no world from the host")
	}
	if got := w.GetBlock(1, 2, 3); got != 9 {
		t.Fatalf("world.GetBlock(1,2,3) = %d, want 9", got)
	}
}
