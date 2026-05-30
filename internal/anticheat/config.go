package anticheat

// Config maps anticheat.yml (DESIGN §9): global toggle, decay, and per-check
// enable/weight/threshold settings.
type Config struct {
	Enabled      bool                   `yaml:"enabled"`
	DecayPerTick float64                `yaml:"decay_per_tick"` // retention factor in (0,1]
	Checks       map[string]CheckConfig `yaml:"checks"`
}

// CheckConfig tunes one check. The threshold fields are weighted-score cutoffs
// for each staged Action; a value <= 0 disables that stage. Any violation is
// always eligible for ActionLog.
type CheckConfig struct {
	Disabled bool    `yaml:"disabled"` // zero value keeps a check enabled by default
	Weight   float64 `yaml:"weight"`
	Warn     float64 `yaml:"warn"`
	Setback  float64 `yaml:"setback"`
	Kick     float64 `yaml:"kick"`
	Ban      float64 `yaml:"ban"`
}

func (c CheckConfig) disabled() bool { return c.Disabled }

func (c CheckConfig) weight() float64 {
	if c.Weight <= 0 {
		return 1
	}
	return c.Weight
}

func (c CheckConfig) action(score float64) Action {
	switch {
	case c.Ban > 0 && score >= c.Ban:
		return ActionBan
	case c.Kick > 0 && score >= c.Kick:
		return ActionKick
	case c.Setback > 0 && score >= c.Setback:
		return ActionSetback
	case c.Warn > 0 && score >= c.Warn:
		return ActionWarn
	default:
		return ActionLog
	}
}

// DefaultConfig returns conservative defaults: engine on, gentle decay, no
// checks registered yet (they land with the canonical-model foundation).
func DefaultConfig() Config {
	return Config{Enabled: true, DecayPerTick: 0.98, Checks: map[string]CheckConfig{}}
}
