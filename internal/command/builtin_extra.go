package command

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"livingworld/internal/shared/constants/chat"
	"livingworld/internal/world"
)

// registerExtended adds the broader vanilla-style command set on top of the
// core commands. Handlers use only existing infrastructure (world edit, player
// manager, ops capability) — commands needing unbuilt subsystems (/effect,
// /enchant, /xp, /gamerule, /ban, /whitelist) are intentionally absent.
func registerExtended(r *Registry) {
	r.Register(&Command{
		Name: "setblock", Description: "Set a block", Usage: "setblock <x> <y> <z> <block>",
		Permission: PermOperator, MinArgs: 4, Handler: cmdSetblock,
	})
	r.Register(&Command{
		Name: "fill", Description: "Fill a region with a block", Usage: "fill <x1> <y1> <z1> <x2> <y2> <z2> <block>",
		Permission: PermOperator, MinArgs: 7, Handler: cmdFill,
	})
	r.Register(&Command{
		Name: "op", Description: "Grant operator status", Usage: "op <player>",
		Permission: PermOperator, MinArgs: 1, Handler: cmdOp,
	})
	r.Register(&Command{
		Name: "deop", Description: "Revoke operator status", Usage: "deop <player>",
		Permission: PermOperator, MinArgs: 1, Handler: cmdDeop,
	})
	r.Register(&Command{
		Name: "kick", Description: "Kick a player", Usage: "kick <player> [reason]",
		Permission: PermOperator, MinArgs: 1, Handler: cmdKick,
	})
	r.Register(&Command{
		Name: "list", Description: "List online players", Usage: "list",
		Permission: PermAll, Handler: cmdListPlayers,
	})
	r.Register(&Command{
		Name: "msg", Aliases: []string{"tell", "w"}, Description: "Private message", Usage: "msg <player> <text>",
		Permission: PermAll, MinArgs: 2, Handler: cmdMsg,
	})
	r.Register(&Command{
		Name: "me", Description: "Emote in chat", Usage: "me <action>",
		Permission: PermAll, MinArgs: 1, Handler: cmdMe,
	})
}

// senderPos returns the sender's current position (for '~' relative coords).
func senderPos(ctx *Ctx) (x, y, z float64) {
	if ctx.PM != nil {
		if p := ctx.PM.GetPlayerByName(ctx.Sender.Name()); p != nil {
			s := p.Snapshot()
			return s.Position.X, s.Position.Y, s.Position.Z
		}
	}
	return 0, 0, 0
}

// coord parses an absolute ("64") or relative ("~", "~3") coordinate.
func coord(tok string, base float64) (int, error) {
	if strings.HasPrefix(tok, "~") {
		if tok == "~" {
			return int(math.Floor(base)), nil
		}
		d, err := strconv.ParseFloat(tok[1:], 64)
		if err != nil {
			return 0, fmt.Errorf("bad coordinate %q", tok)
		}
		return int(math.Floor(base + d)), nil
	}
	n, err := strconv.Atoi(tok)
	if err != nil {
		return 0, fmt.Errorf("bad coordinate %q", tok)
	}
	return n, nil
}

// resolveBlock turns a (possibly unqualified) block name into a state ID.
func resolveBlock(name string) (int32, error) {
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}
	id := world.StateID(name)
	if id == world.AirID && name != "minecraft:air" {
		return 0, fmt.Errorf("unknown block %q", name)
	}
	return id, nil
}

func cmdSetblock(ctx *Ctx) error {
	bx, by, bz := senderPos(ctx)
	x, e1 := coord(ctx.Args[0], bx)
	y, e2 := coord(ctx.Args[1], by)
	z, e3 := coord(ctx.Args[2], bz)
	if e1 != nil || e2 != nil || e3 != nil {
		return fmt.Errorf("coordinates must be numbers or ~")
	}
	id, err := resolveBlock(ctx.Args[3])
	if err != nil {
		return err
	}
	ctx.WM.SetBlockAndPublish(world.BlockUpdateSourceServer, x, y, z, world.BlockByID(id))
	ctx.Sender.Reply(chat.ColorGreen + fmt.Sprintf("Set block at %d %d %d", x, y, z))
	return nil
}

func cmdFill(ctx *Ctx) error {
	bx, by, bz := senderPos(ctx)
	bases := []float64{bx, by, bz, bx, by, bz}
	c := make([]int, 6)
	for i := 0; i < 6; i++ {
		v, err := coord(ctx.Args[i], bases[i])
		if err != nil {
			return err
		}
		c[i] = v
	}
	id, err := resolveBlock(ctx.Args[6])
	if err != nil {
		return err
	}
	x1, x2 := minMax(c[0], c[3])
	y1, y2 := minMax(c[1], c[4])
	z1, z2 := minMax(c[2], c[5])
	vol := (x2 - x1 + 1) * (y2 - y1 + 1) * (z2 - z1 + 1)
	if vol > 32768 {
		return fmt.Errorf("too many blocks (%d > 32768)", vol)
	}
	blk := world.BlockByID(id)
	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			for z := z1; z <= z2; z++ {
				ctx.WM.SetBlockAndPublish(world.BlockUpdateSourceServer, x, y, z, blk)
			}
		}
	}
	ctx.Sender.Reply(chat.ColorGreen + fmt.Sprintf("Filled %d block(s)", vol))
	return nil
}

func minMax(a, b int) (int, int) {
	if a > b {
		return b, a
	}
	return a, b
}

func cmdOp(ctx *Ctx) error {
	if ctx.Ops == nil {
		return fmt.Errorf("operator management is unavailable")
	}
	if ctx.Ops.SetOp(ctx.Args[0], true) {
		ctx.Sender.Reply(chat.ColorGreen + ctx.Args[0] + " is now an operator")
	} else {
		ctx.Sender.Reply(ctx.Args[0] + " is already an operator")
	}
	return nil
}

func cmdDeop(ctx *Ctx) error {
	if ctx.Ops == nil {
		return fmt.Errorf("operator management is unavailable")
	}
	if ctx.Ops.SetOp(ctx.Args[0], false) {
		ctx.Sender.Reply(chat.ColorGreen + ctx.Args[0] + " is no longer an operator")
	} else {
		ctx.Sender.Reply(ctx.Args[0] + " was not an operator")
	}
	return nil
}

func cmdKick(ctx *Ctx) error {
	if ctx.PM == nil {
		return fmt.Errorf("no player manager")
	}
	target := ctx.PM.GetPlayerByName(ctx.Args[0])
	if target == nil {
		return fmt.Errorf("player %q not found", ctx.Args[0])
	}
	reason := "Kicked by an operator"
	if len(ctx.Args) > 1 {
		reason = strings.Join(ctx.Args[1:], " ")
	}
	ctx.PM.Kick(target.UUID, reason)
	ctx.Sender.Reply(chat.ColorGreen + "Kicked " + target.Username)
	return nil
}

func cmdListPlayers(ctx *Ctx) error {
	if ctx.PM == nil {
		return fmt.Errorf("no player manager")
	}
	all := ctx.PM.GetAllPlayers()
	names := make([]string, 0, len(all))
	for _, p := range all {
		names = append(names, p.Username)
	}
	ctx.Sender.Reply(fmt.Sprintf(chat.ColorYellow+"%d player(s) online: "+chat.Reset+"%s", len(names), strings.Join(names, ", ")))
	return nil
}

func cmdMsg(ctx *Ctx) error {
	if ctx.PM == nil {
		return fmt.Errorf("no player manager")
	}
	target := ctx.PM.GetPlayerByName(ctx.Args[0])
	if target == nil {
		return fmt.Errorf("player %q not found", ctx.Args[0])
	}
	msg := strings.Join(ctx.Args[1:], " ")
	ctx.PM.Message(target.UUID, chat.ColorGray+"["+ctx.Sender.Name()+" -> you] "+chat.Reset+msg)
	ctx.Sender.Reply(chat.ColorGray + "[you -> " + target.Username + "] " + chat.Reset + msg)
	return nil
}

func cmdMe(ctx *Ctx) error {
	if ctx.PM == nil {
		return fmt.Errorf("no player manager")
	}
	ctx.PM.Broadcast(chat.ColorGray + "* " + ctx.Sender.Name() + " " + strings.Join(ctx.Args, " "))
	return nil
}
