package command

import (
	"fmt"
	"strconv"
	"strings"
)

// RegisterBuiltins adds the core commands to r. Java vs Bedrock differences (e.g.
// /time only works on Bedrock today) are handled inside the handlers via
// ctx.Sender.Edition().
func RegisterBuiltins(r *Registry) {
	r.Register(&Command{
		Name: "help", Description: "List commands", Usage: "help",
		Permission: PermAll, Handler: cmdHelp(r),
	})
	r.Register(&Command{
		Name: "gamemode", Aliases: []string{"gm"}, Description: "Change gamemode",
		Usage: "gamemode <survival|creative|adventure|spectator> [player]",
		Permission: PermOperator, MinArgs: 1, Handler: cmdGamemode,
	})
	r.Register(&Command{
		Name: "tp", Aliases: []string{"teleport"}, Description: "Teleport",
		Usage: "tp <x> <y> <z>  |  tp <player>",
		Permission: PermOperator, MinArgs: 1, Handler: cmdTp,
	})
	r.Register(&Command{
		Name: "give", Description: "Give an item", Usage: "give <item> [count]",
		Permission: PermOperator, MinArgs: 1, Handler: cmdGive,
	})
	r.Register(&Command{
		Name: "time", Description: "Set the time of day", Usage: "time set <day|night|noon|midnight|ticks>",
		Permission: PermOperator, MinArgs: 1, Handler: cmdTime,
	})
}

func cmdHelp(r *Registry) Handler {
	return func(ctx *Ctx) error {
		ctx.Sender.Reply("§e=== Commands ===")
		for _, c := range r.Commands() {
			if c.Permission == PermOperator && !ctx.Sender.IsOp() {
				continue
			}
			ctx.Sender.Reply(fmt.Sprintf("§7/%s§r - %s", c.Usage, c.Description))
		}
		return nil
	}
}

func parseGameMode(s string) (int, bool) {
	switch strings.ToLower(s) {
	case "survival", "s", "0":
		return 0, true
	case "creative", "c", "1":
		return 1, true
	case "adventure", "a", "2":
		return 2, true
	case "spectator", "sp", "3":
		return 3, true
	}
	return 0, false
}

func cmdGamemode(ctx *Ctx) error {
	mode, ok := parseGameMode(ctx.Args[0])
	if !ok {
		return fmt.Errorf("unknown gamemode %q", ctx.Args[0])
	}
	// Target defaults to the sender; a second arg targets another player, but
	// applying to others needs the target's live session — only self is wired
	// for now (cross-session gamemode is a follow-up).
	if len(ctx.Args) >= 2 && !strings.EqualFold(ctx.Args[1], ctx.Sender.Name()) {
		return fmt.Errorf("changing another player's gamemode is not supported yet")
	}
	if err := ctx.Sender.SetGameMode(mode); err != nil {
		return err
	}
	ctx.Sender.Reply("§aGamemode updated")
	return nil
}

func cmdTp(ctx *Ctx) error {
	// /tp <player>
	if len(ctx.Args) == 1 {
		if ctx.PM == nil {
			return fmt.Errorf("no player manager")
		}
		target := ctx.PM.GetPlayerByName(ctx.Args[0])
		if target == nil {
			return fmt.Errorf("player %q not found", ctx.Args[0])
		}
		s := target.Snapshot()
		if err := ctx.Sender.Teleport(s.Position.X, s.Position.Y, s.Position.Z); err != nil {
			return err
		}
		ctx.Sender.Reply("§aTeleported to " + target.Username)
		return nil
	}
	// /tp <x> <y> <z>
	if len(ctx.Args) < 3 {
		return fmt.Errorf("usage: /tp <x> <y> <z> or /tp <player>")
	}
	x, err1 := strconv.ParseFloat(ctx.Args[0], 64)
	y, err2 := strconv.ParseFloat(ctx.Args[1], 64)
	z, err3 := strconv.ParseFloat(ctx.Args[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return fmt.Errorf("coordinates must be numbers")
	}
	if err := ctx.Sender.Teleport(x, y, z); err != nil {
		return err
	}
	ctx.Sender.Reply(fmt.Sprintf("§aTeleported to %.1f %.1f %.1f", x, y, z))
	return nil
}

func cmdGive(ctx *Ctx) error {
	item := ctx.Args[0]
	count := 1
	if len(ctx.Args) >= 2 {
		if n, err := strconv.Atoi(ctx.Args[1]); err == nil && n > 0 {
			count = n
		}
	}
	if err := ctx.Sender.GiveItem(item, count); err != nil {
		return err
	}
	ctx.Sender.Reply(fmt.Sprintf("§aGave %d x %s", count, item))
	return nil
}

func parseTimeTicks(s string) (int64, bool) {
	switch strings.ToLower(s) {
	case "day":
		return 1000, true
	case "noon":
		return 6000, true
	case "night":
		return 13000, true
	case "midnight":
		return 18000, true
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n >= 0 {
		return n % 24000, true
	}
	return 0, false
}

func cmdTime(ctx *Ctx) error {
	// /time set <value>  (also accept "/time <value>")
	arg := ctx.Args[0]
	if strings.EqualFold(arg, "set") {
		if len(ctx.Args) < 2 {
			return fmt.Errorf("usage: /time set <day|night|noon|midnight|ticks>")
		}
		arg = ctx.Args[1]
	}
	ticks, ok := parseTimeTicks(arg)
	if !ok {
		return fmt.Errorf("unknown time %q", arg)
	}
	if ctx.WM == nil {
		return fmt.Errorf("no world manager")
	}
	ctx.WM.GetDefaultWorld().SetDayTime(ticks)
	ctx.Sender.Reply(fmt.Sprintf("§aSet time to %d", ticks))
	return nil
}
