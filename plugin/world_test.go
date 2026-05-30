package plugin

import "testing"

// fakeWorld is a minimal in-memory World handle for tests.
type fakeWorld struct{ blocks map[[3]int]int32 }

func (fakeWorld) Name() string { return "world" }
func (w *fakeWorld) GetBlock(x, y, z int) int32 { return w.blocks[[3]int{x, y, z}] }
func (w *fakeWorld) SetBlock(x, y, z int, id int32) {
	if w.blocks == nil {
		w.blocks = map[[3]int]int32{}
	}
	w.blocks[[3]int{x, y, z}] = id
}
func (fakeWorld) StateID(name string) int32 {
	if name == "minecraft:stone" {
		return 1
	}
	return 0
}

// worldlessHost satisfies Host but not WorldHost.
type worldlessHost struct{}

func (worldlessHost) Broadcast(string)              {}
func (worldlessHost) Message(string, string)        {}
func (worldlessHost) Players() []string             { return nil }
func (worldlessHost) PlayerCount() int              { return 0 }
func (worldlessHost) GetBlock(int, int, int) int32  { return 0 }
func (worldlessHost) SetBlock(int, int, int, int32) {}
func (worldlessHost) StateID(string) int32          { return 0 }
func (worldlessHost) Log(string, ...any)            {}

// worldfulHost adds World(), so it also satisfies WorldHost.
type worldfulHost struct {
	worldlessHost
	w World
}

func (h worldfulHost) World() World { return h.w }

func TestWorldOf(t *testing.T) {
	if got := WorldOf(worldlessHost{}); got != nil {
		t.Fatalf("WorldOf(non-WorldHost) = %v, want nil", got)
	}

	w := &fakeWorld{}
	got := WorldOf(worldfulHost{w: w})
	if got == nil {
		t.Fatal("WorldOf(WorldHost) = nil, want a world handle")
	}
	got.SetBlock(1, 2, 3, 5)
	if got.GetBlock(1, 2, 3) != 5 {
		t.Fatalf("GetBlock(1,2,3) = %d, want 5", got.GetBlock(1, 2, 3))
	}
	if got.Name() != "world" || got.StateID("minecraft:stone") != 1 {
		t.Fatalf("Name/StateID mismatch: %q / %d", got.Name(), got.StateID("minecraft:stone"))
	}
}
