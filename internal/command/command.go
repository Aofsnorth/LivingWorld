// Package command is LivingWorld's protocol-free command/"cheat" system. Both
// the Java and Bedrock bridges parse a player's "/..." input and dispatch it
// here; handlers act through the Sender interface so the package never imports
// any protocol code (mirroring player.Controller's design).
package command

import (
	"github.com/google/uuid"

	"livingworld/internal/player"
)

// PermissionLevel gates who may run a command.
type PermissionLevel int

const (
	PermAll      PermissionLevel = iota // anyone
	PermOperator                        // OP / cheats only
)

// Sender is the player a command acts on/through. Each bridge implements it on
// its session. It is protocol-free, like player.Controller.
type Sender interface {
	Name() string
	UUID() uuid.UUID
	IsOp() bool
	Edition() player.Edition
	// Reply delivers command feedback (system chat in each edition's format).
	Reply(msg string)
	// Authoritative state mutators — the bridge sends the correct per-edition
	// packets so gameplay state stays server-authoritative.
	SetGameMode(mode int) error // 0 survival, 1 creative, 2 adventure, 3 spectator
	Teleport(x, y, z float64) error
	GiveItem(itemName string, count int) error
}

// Handler runs a command. ctx carries the parsed args + the Sender.
type Handler func(ctx *Ctx) error

// OpController lets the /op and /deop commands mutate the server operator list.
// The server implements it and command.BindOps wires it in (this package can't
// import server). It is nil until bound.
type OpController interface {
	SetOp(name string, op bool) bool
}

// Command is a registered command.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Usage       string // shown on wrong usage, without leading '/'
	Permission  PermissionLevel
	MinArgs     int
	Handler     Handler
}
