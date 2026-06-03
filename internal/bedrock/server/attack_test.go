package server

import (
	"testing"

	"livingworld/config"
	"livingworld/internal/player"
	"livingworld/internal/skinbridge"
	"livingworld/internal/world"

	"github.com/google/uuid"
)

// newTestBedrockServer constructs a minimal Bedrock Server with
// the pieces M5's tests need: a world manager with the default
// world, a player manager, and an empty sessions map. It does
// NOT start any goroutines, register a listener, or open a
// network socket — it's a test fixture, not a runnable server.
func newTestBedrockServer(t *testing.T) *Server {
	t.Helper()
	cfg := &config.Config{
		Bedrock: config.BedrockConfig{
			Bind:       "127.0.0.1",
			Port:       0, // unused
			MaxPlayers: 8,
		},
		World: config.WorldConfig{Seed: 42},
	}
	wm := world.NewManager()
	_ = wm.GetDefaultWorld()
	pm := player.NewManager()
	// skinbridge.New() is a no-op constructor in v1.
	skins := skinbridge.New()
	return NewServer(cfg, pm, wm, skins)
}

// bedrockAttackerUUID returns a deterministic UUID for tests.
// The bytes are copied into the mob's hurtBy field on
// HurtDirect; any 16 bytes work.
func bedrockAttackerUUID() uuid.UUID {
	return uuid.MustParse("66666666-6666-6666-6666-666666666666")
}

// TestM5_BedrockRouteAttackToMob_AppliesDamage verifies the
// Bedrock routeBedrockAttack path: an attack on a mob entity
// id applies direct damage to that mob.
func TestM5_BedrockRouteAttackToMob_AppliesDamage(t *testing.T) {
	s := newTestBedrockServer(t)
	mobID := s.wm.Mobs().Spawn("minecraft:zombie", 0, 64, 0).EntityID
	before := s.wm.Mobs().Get(mobID)
	if before.HP != 20 {
		t.Fatalf("zombie starting HP: got %v want 20", before.HP)
	}
	s.routeBedrockAttack(bedrockAttackerUUID(), mobID)
	after := s.wm.Mobs().Get(mobID)
	if after.HP != 19 {
		t.Errorf("zombie HP after 1 swing: got %v want 19", after.HP)
	}
}

// TestM5_BedrockRouteAttackToMob_KillsAtZeroHP verifies the
// Bedrock path also sets Despawn when HP hits zero.
func TestM5_BedrockRouteAttackToMob_KillsAtZeroHP(t *testing.T) {
	s := newTestBedrockServer(t)
	mobID := s.wm.Mobs().Spawn("minecraft:skeleton", 0, 64, 0).EntityID
	for i := 0; i < 20; i++ {
		s.routeBedrockAttack(bedrockAttackerUUID(), mobID)
	}
	after := s.wm.Mobs().Get(mobID)
	if after.HP > 0 {
		t.Errorf("skeleton after 20 swings: HP=%v; want 0", after.HP)
	}
	pending := s.wm.Mobs().PendingDespawns()
	found := false
	for _, id := range pending {
		if id == mobID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("skeleton: not in PendingDespawns after 20 swings; got %v", pending)
	}
}

// TestM5_BedrockRouteAttack_UnknownTargetFallsThrough verifies
// the Bedrock path also rejects EntityID==0 mobs (a miss from
// Get returns zero Mob, which routeBedrockAttack must NOT
// damage).
func TestM5_BedrockRouteAttack_UnknownTargetFallsThrough(t *testing.T) {
	s := newTestBedrockServer(t)
	hpBefore := s.wm.Mobs().Spawn("minecraft:cow", 100, 64, 0)
	s.routeBedrockAttack(bedrockAttackerUUID(), 99999)
	hpAfter := s.wm.Mobs().Get(hpBefore.EntityID)
	if hpAfter.HP != hpBefore.HP {
		t.Errorf("cow HP changed by stray attack on id=99999: before=%v after=%v",
			hpBefore.HP, hpAfter.HP)
	}
}
