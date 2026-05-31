package command

import (
	"sort"
	"strings"
	"sync"

	"livingworld/internal/player"
	"livingworld/internal/shared/constants/chat"
	"livingworld/internal/world"
)

// Registry holds commands and dispatches input to them.
type Registry struct {
	mu    sync.RWMutex
	cmds  map[string]*Command // keyed by name AND each alias (lowercase)
	names []string            // canonical names, registration order

	pm  *player.Manager
	wm  *world.Manager
	ops OpController
}

// Ctx is passed to a command handler.
type Ctx struct {
	Sender Sender
	Args   []string
	PM     *player.Manager
	WM     *world.Manager
	Ops    OpController
}

var defaultRegistry = New(nil, nil)

// Default returns the process-wide registry.
func Default() *Registry { return defaultRegistry }

// Bind attaches the shared managers to the default registry (called once at
// startup so handlers can resolve players/world).
func Bind(pm *player.Manager, wm *world.Manager) {
	defaultRegistry.mu.Lock()
	defaultRegistry.pm = pm
	defaultRegistry.wm = wm
	defaultRegistry.mu.Unlock()
}

// BindOps wires the operator-list capability into the default registry so /op
// and /deop work. Called once at startup by the server.
func BindOps(o OpController) {
	defaultRegistry.mu.Lock()
	defaultRegistry.ops = o
	defaultRegistry.mu.Unlock()
}

func New(pm *player.Manager, wm *world.Manager) *Registry {
	return &Registry{cmds: make(map[string]*Command), pm: pm, wm: wm}
}

// Register adds a command (and its aliases). A duplicate name is ignored.
func (r *Registry) Register(c *Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := strings.ToLower(c.Name)
	if _, exists := r.cmds[key]; exists {
		return
	}
	r.cmds[key] = c
	r.names = append(r.names, c.Name)
	for _, a := range c.Aliases {
		r.cmds[strings.ToLower(a)] = c
	}
}

// Get resolves a command by name or alias.
func (r *Registry) Get(name string) (*Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.cmds[strings.ToLower(name)]
	return c, ok
}

// Commands returns the canonical command list, sorted by name.
func (r *Registry) Commands() []*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Command, 0, len(r.names))
	for _, n := range r.names {
		out = append(out, r.cmds[strings.ToLower(n)])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Dispatch parses raw input (WITHOUT the leading '/'), checks permission, and
// runs the command. Returns true if the input was a recognized command attempt
// (so the caller suppresses it from chat). Feedback is sent via Sender.Reply.
func (r *Registry) Dispatch(s Sender, raw string) (handled bool) {
	fields := tokenize(raw)
	if len(fields) == 0 {
		return false
	}
	cmd, ok := r.Get(fields[0])
	if !ok {
		s.Reply(chat.ColorRed + "Unknown command: /" + fields[0])
		return true
	}
	if cmd.Permission == PermOperator && !s.IsOp() {
		s.Reply(chat.ColorRed + "You don't have permission to use /" + cmd.Name)
		return true
	}
	args := fields[1:]
	if len(args) < cmd.MinArgs {
		usage := cmd.Usage
		if usage == "" {
			usage = cmd.Name
		}
		s.Reply(chat.ColorRed + "Usage: /" + usage)
		return true
	}
	r.mu.RLock()
	pm, wm, ops := r.pm, r.wm, r.ops
	r.mu.RUnlock()
	ctx := &Ctx{Sender: s, Args: args, PM: pm, WM: wm, Ops: ops}
	if err := cmd.Handler(ctx); err != nil {
		s.Reply(chat.ColorRed + "Error: " + err.Error())
	}
	return true
}

// tokenize splits on whitespace, honoring "double quotes" for multi-word args.
func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r == '"':
			inQuote = !inQuote
		case (r == ' ' || r == '\t') && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}
