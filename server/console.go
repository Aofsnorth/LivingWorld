package server

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

// consoleHost is the narrow server capability surface the console needs. Keeping
// it small (instead of depending on *Server) decouples the console and makes it
// unit-testable.
type consoleHost interface {
	Broadcast(msg string)
	Players() []string
}

type consoleCmd struct {
	desc string
	run  func(args []string)
}

// console reads operator commands from stdin and dispatches them. It is the
// foundation for server-side commands, distinct from the in-game player commands
// in internal/command.
type console struct {
	host  consoleHost
	out   io.Writer
	stop  func()
	cmds  map[string]consoleCmd
	names []string
}

func newConsole(host consoleHost, out io.Writer, stop func()) *console {
	c := &console{host: host, out: out, stop: stop, cmds: map[string]consoleCmd{}}
	c.register("help", "list console commands", c.cmdHelp)
	c.register("list", "show online players", c.cmdList)
	c.register("say", "broadcast a message: say <text>", c.cmdSay)
	c.register("stop", "shut the server down", func([]string) { c.stop() })
	return c
}

func (c *console) register(name, desc string, run func([]string)) {
	c.cmds[name] = consoleCmd{desc: desc, run: run}
	c.names = append(c.names, name)
}

// run reads and dispatches lines until the reader hits EOF.
func (c *console) run(r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		c.dispatch(sc.Text())
	}
}

func (c *console) dispatch(line string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}
	cmd, ok := c.cmds[strings.ToLower(fields[0])]
	if !ok {
		fmt.Fprintf(c.out, "unknown command: %s (type 'help')\n", fields[0])
		return
	}
	cmd.run(fields[1:])
}

func (c *console) cmdHelp([]string) {
	sort.Strings(c.names)
	for _, n := range c.names {
		fmt.Fprintf(c.out, "  %-6s %s\n", n, c.cmds[n].desc)
	}
}

func (c *console) cmdList([]string) {
	p := c.host.Players()
	fmt.Fprintf(c.out, "%d player(s) online: %s\n", len(p), strings.Join(p, ", "))
}

func (c *console) cmdSay(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(c.out, "usage: say <text>")
		return
	}
	c.host.Broadcast("[Server] " + strings.Join(args, " "))
}
