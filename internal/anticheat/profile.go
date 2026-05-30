package anticheat

// Profile holds a player's weighted violation scores per check, with decay so
// transient hits and isolated false positives fade over time (DESIGN §9).
type Profile struct {
	Score map[string]float64 // check name -> accumulated weighted score
	Decay float64            // per-tick retention factor in (0,1]; 1 = no decay
}

func newProfile(decay float64) *Profile {
	if decay <= 0 || decay > 1 {
		decay = 1
	}
	return &Profile{Score: map[string]float64{}, Decay: decay}
}

// add accumulates a weighted violation for a check and returns the new score.
func (p *Profile) add(check string, weighted float64) float64 {
	p.Score[check] += weighted
	return p.Score[check]
}

// decayTick applies one step of exponential decay to every score, clearing
// negligible entries. Call once per server tick (or on a fixed interval).
func (p *Profile) decayTick() {
	for k, v := range p.Score {
		if v *= p.Decay; v < 0.01 {
			delete(p.Score, k)
		} else {
			p.Score[k] = v
		}
	}
}
