package world

import (
	"math/rand"
	"testing"
)

func TestGrassRandomTickSpreadsToNearbyDirt(t *testing.T) {
	m := NewManager()
	w := m.GetDefaultWorld()
	dirtID := StateID("minecraft:dirt")
	grassID := StateID("minecraft:grass_block")

	w.SetBlock(0, 64, 0, BlockByID(grassID))
	w.SetBlock(1, 64, 0, BlockByID(dirtID))
	w.SetBlock(-1, 64, 0, BlockByID(dirtID))
	w.SetBlock(0, 64, 1, BlockByID(dirtID))
	w.SetBlock(0, 64, -1, BlockByID(dirtID))
	w.Light().ProcessUpdates()

	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 200; i++ {
		m.randomTickGrassBlock(rng, w, 0, 64, 0, dirtID, grassID)
	}

	spread := 0
	for _, pos := range [][3]int{{1, 64, 0}, {-1, 64, 0}, {0, 64, 1}, {0, 64, -1}} {
		if w.GetBlock(pos[0], pos[1], pos[2]).ID() == grassID {
			spread++
		}
	}
	if spread == 0 {
		t.Fatalf("expected grass to spread to at least one neighbouring dirt block")
	}
}

func TestGrassRandomTickDecaysWhenCovered(t *testing.T) {
	m := NewManager()
	w := m.GetDefaultWorld()
	dirtID := StateID("minecraft:dirt")
	grassID := StateID("minecraft:grass_block")
	stoneID := StateID("minecraft:stone")

	w.SetBlock(0, 64, 0, BlockByID(grassID))
	w.SetBlock(0, 65, 0, BlockByID(stoneID))
	w.Light().ProcessUpdates()

	m.randomTickGrassBlock(rand.New(rand.NewSource(1)), w, 0, 64, 0, dirtID, grassID)

	if got := w.GetBlock(0, 64, 0).ID(); got != dirtID {
		t.Fatalf("covered grass should decay to dirt, got %s", StateName(got))
	}
}

func TestGrassRandomTickDecaysInLowLight(t *testing.T) {
	m := NewManager()
	w := m.GetDefaultWorld()
	dirtID := StateID("minecraft:dirt")
	grassID := StateID("minecraft:grass_block")

	w.SetBlock(0, 64, 0, BlockByID(grassID))
	w.Light().ProcessUpdates()
	w.SetSkyLight(0, 65, 0, 0)
	w.SetBlockLight(0, 65, 0, 0)

	m.randomTickGrassBlock(rand.New(rand.NewSource(1)), w, 0, 64, 0, dirtID, grassID)

	if got := w.GetBlock(0, 64, 0).ID(); got != dirtID {
		t.Fatalf("low-light grass should decay to dirt, got %s", StateName(got))
	}
}
