package anticheat

import "sync"

// Outcome is the engine's decision for one triggered check on one event.
type Outcome struct {
	Check    string
	Score    float64
	Action   Action
	Mitigate Mitigation
	Reason   string
}

// Engine runs registered checks, aggregates weighted violations per player with
// decay, and maps scores to staged actions per config (DESIGN §9). It is
// independent of the canonical model so it builds standalone.
type Engine struct {
	cfg      Config
	checks   []Check
	mu       sync.Mutex
	profiles map[string]*Profile
}

// New builds an Engine from config.
func New(cfg Config) *Engine {
	return &Engine{cfg: cfg, profiles: map[string]*Profile{}}
}

// Register adds a check. Call during setup, before Handle.
func (e *Engine) Register(c Check) { e.checks = append(e.checks, c) }

// Handle runs every enabled check against the event and returns an Outcome for
// each check that produced a violation. A disabled engine or an exempt player
// short-circuits to nil.
func (e *Engine) Handle(ctx *PlayerCtx, ev Event) []Outcome {
	if !e.cfg.Enabled || ctx == nil || ctx.Exempt {
		return nil
	}
	comp := compensation(ctx)

	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.profileFor(ctx.UUID)

	var out []Outcome
	for _, c := range e.checks {
		cc := e.cfg.Checks[c.Name()]
		if cc.disabled() {
			continue
		}
		r := c.Inspect(ctx, ev)
		if r.Vio <= 0 {
			continue
		}
		score := p.add(c.Name(), r.Vio*cc.weight()*comp)
		out = append(out, Outcome{
			Check:    c.Name(),
			Score:    score,
			Action:   cc.action(score),
			Mitigate: r.Mitigate,
			Reason:   r.Reason,
		})
	}
	return out
}

// Decay advances violation decay for all tracked players by one step. Call from
// the world tick loop (or a fixed scheduler).
func (e *Engine) Decay() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, p := range e.profiles {
		if p.decayTick(); len(p.Score) == 0 {
			delete(e.profiles, id)
		}
	}
}

// Forget drops a player's profile (e.g. on disconnect).
func (e *Engine) Forget(uuid string) {
	e.mu.Lock()
	delete(e.profiles, uuid)
	e.mu.Unlock()
}

func (e *Engine) profileFor(uuid string) *Profile {
	p := e.profiles[uuid]
	if p == nil {
		p = newProfile(e.cfg.DecayPerTick)
		e.profiles[uuid] = p
	}
	return p
}

// compensation widens tolerance under latency / low TPS so legitimate laggy
// players are not falsely flagged (DESIGN §9). Returns a factor in [0.25, 1].
func compensation(ctx *PlayerCtx) float64 {
	f := 1.0
	if ctx.LatencyMS > 100 {
		f -= float64(ctx.LatencyMS-100) / 1000.0
	}
	if ctx.TPS > 0 && ctx.TPS < 20 {
		f *= ctx.TPS / 20
	}
	if f < 0.25 {
		f = 0.25
	}
	return f
}
