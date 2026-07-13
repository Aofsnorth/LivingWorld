package context

import "testing"

func TestNewBuildsContext(t *testing.T) {
	ctx := New(12345)
	if ctx == nil {
		t.Fatal("New returned nil")
	}
	if ctx.NoiseCtx.Seed != 12345 {
		t.Fatalf("seed mismatch: got %d", ctx.NoiseCtx.Seed)
	}
	if ctx.Final.Tree == nil {
		t.Fatal("Final.Tree is nil")
	}
	if len(ctx.NoiseCtx.Noises) == 0 {
		t.Fatal("Noises map empty")
	}
}
