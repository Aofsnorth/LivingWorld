// Package dfcompat lets unmodified dragonfly plugins run on LivingWorld. It
// adapts dragonfly's player.Handler onto LivingWorld's event bus and translates
// dragonfly block/item identities through the shared canonical registry.
//
// KNOWN GAP: dragonfly handlers receive the *player.Player subject via
// ctx.Val(); until the canonical player<->dragonfly adapter lands that subject
// is nil, so handlers that dereference the player do not work yet. Data carried
// in non-player arguments (chat text, positions, command args) and ctx.Cancel()
// bridge fully. Panics from handlers that touch the nil subject are contained by
// the plugin manager's panic isolation.
package dfcompat

import (
	"strings"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/cmd"
	"github.com/df-mc/dragonfly/server/event"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/player"

	"livingworld/plugin"
)

// Bridge invokes a dragonfly player.Handler in response to LivingWorld events.
type Bridge struct {
	h     player.Handler
	world plugin.World // world instance, resolved from the host on Register (nil if the host exposes none)
}

// New wraps an unmodified dragonfly handler so it can be registered on a manager.
func New(h player.Handler) *Bridge { return &Bridge{h: h} }

// Register wires the bridged dragonfly handler onto a LivingWorld plugin manager.
func (b *Bridge) Register(pm *plugin.PluginManager) {
	b.world = plugin.WorldOf(pm.Host()) // access the world instance via the host (DESIGN §7)
	pm.OnPlayerChat(b.onChat)
	pm.OnBlockBreak(b.onBlockBreak)
	pm.OnBlockPlace(b.onBlockPlace)
	pm.OnPlayerCommand(b.onCommand)
}

// World returns the world instance this bridge is bound to, or nil if the host
// exposes none. It is the dfcompat adapter's handle for resolving the block
// context dragonfly's handler surface needs (see the package KNOWN GAP note).
func (b *Bridge) World() plugin.World { return b.world }

func (b *Bridge) onChat(e *plugin.PlayerChatEvent) {
	ctx := event.C[*player.Player](nil)
	msg := e.Message
	b.h.HandleChat(ctx, &msg)
	if ctx.Cancelled() {
		e.Cancel()
		return
	}
	e.Message = msg // honour a handler rewriting the message
}

func (b *Bridge) onBlockBreak(e *plugin.BlockBreakEvent) {
	ctx := event.C[*player.Player](nil)
	var drops []item.Stack
	xp := 0
	b.h.HandleBlockBreak(ctx, cube.Pos{e.X, e.Y, e.Z}, &drops, &xp)
	if ctx.Cancelled() {
		e.Cancel()
	}
}

func (b *Bridge) onBlockPlace(e *plugin.BlockPlaceEvent) {
	ctx := event.C[*player.Player](nil)
	b.h.HandleBlockPlace(ctx, cube.Pos{e.X, e.Y, e.Z}, nil) // world.Block subject is a known gap
	if ctx.Cancelled() {
		e.Cancel()
	}
}

func (b *Bridge) onCommand(e *plugin.PlayerCommandEvent) {
	ctx := event.C[*player.Player](nil)
	args := strings.Fields(e.Command)
	if len(args) > 0 {
		args = args[1:] // dragonfly passes the args after the command name
	}
	b.h.HandleCommandExecution(ctx, cmd.Command{}, args)
	if ctx.Cancelled() {
		e.Cancel()
	}
}
