package world

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// CrackState tracks a player's current block-breaking progress for cross-edition
// crack animation broadcasting and proper cleanup when switching blocks.
type CrackState struct {
	PlayerUUID uuid.UUID
	BlockPos   Position
	StartTime  time.Time
}

// CrackManager tracks all active block-breaking states across both editions.
type CrackManager struct {
	mu     sync.RWMutex
	states map[uuid.UUID]*CrackState // playerUUID -> current breaking state
}

func NewCrackManager() *CrackManager {
	return &CrackManager{
		states: make(map[uuid.UUID]*CrackState),
	}
}

// StartBreaking records that a player started breaking a block. Returns the
// previous block position if the player was already breaking a different block
// (caller should send stop-crack for the old position).
func (cm *CrackManager) StartBreaking(playerUUID uuid.UUID, x, y, z int) (hadPrevious bool, prevX, prevY, prevZ int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	prev := cm.states[playerUUID]
	if prev != nil && (int(prev.BlockPos.X) != x || int(prev.BlockPos.Y) != y || int(prev.BlockPos.Z) != z) {
		hadPrevious = true
		prevX, prevY, prevZ = int(prev.BlockPos.X), int(prev.BlockPos.Y), int(prev.BlockPos.Z)
	}

	cm.states[playerUUID] = &CrackState{
		PlayerUUID: playerUUID,
		BlockPos:   Position{X: float64(x), Y: float64(y), Z: float64(z)},
		StartTime:  time.Now(),
	}
	return
}

// StopBreaking clears a player's breaking state (on abort or finish).
func (cm *CrackManager) StopBreaking(playerUUID uuid.UUID) {
	cm.mu.Lock()
	delete(cm.states, playerUUID)
	cm.mu.Unlock()
}

// GetBreaking returns the current breaking state for a player, or nil if not breaking.
func (cm *CrackManager) GetBreaking(playerUUID uuid.UUID) *CrackState {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.states[playerUUID]
}

// GetAllBreaking returns all active breaking states (for broadcasting to new joiners).
func (cm *CrackManager) GetAllBreaking() []*CrackState {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	states := make([]*CrackState, 0, len(cm.states))
	for _, s := range cm.states {
		states = append(states, s)
	}
	return states
}
