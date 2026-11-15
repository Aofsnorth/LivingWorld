package experience

import "math"

// Package experience implements Minecraft 1.20+ edition-agnostic Experience (XP)
// progression and XP orb splitting rebuilt from scratch using GoCraft's core/experience
// architectural principles.

var orbThresholds = [...]int32{
	2477,
	1237,
	617,
	307,
	149,
	73,
	37,
	17,
	7,
	3,
	1,
}

// RoundToOrbSize returns the largest vanilla Minecraft XP orb value <= value.
func RoundToOrbSize(value int32) int32 {
	for _, t := range orbThresholds {
		if value >= t {
			return t
		}
	}
	return 1
}

// SplitIntoOrbs decomposes a total XP amount into a slice of canonical vanilla XP orb values.
func SplitIntoOrbs(total int32) []int32 {
	if total <= 0 {
		return nil
	}
	orbs := make([]int32, 0, 8)
	rem := total
	for rem > 0 {
		size := RoundToOrbSize(rem)
		orbs = append(orbs, size)
		rem -= size
	}
	return orbs
}

// XPToNextLevel returns the XP required to advance from `level` to `level+1`
// according to the official Minecraft 1.20+ experience curve.
func XPToNextLevel(level int32) int32 {
	if level < 0 {
		return 7
	}
	switch {
	case level <= 15:
		return 2*level + 7
	case level <= 30:
		return 5*level - 38
	default:
		return 9*level - 158
	}
}

// TotalXPForLevel computes the cumulative XP needed to reach the start of `level` from level 0.
func TotalXPForLevel(level int32) int32 {
	if level <= 0 {
		return 0
	}
	switch {
	case level <= 16:
		return level*level + 6*level
	case level <= 31:
		return int32(math.Round(2.5*float64(level*level) - 40.5*float64(level) + 360.0))
	default:
		return int32(math.Round(4.5*float64(level*level) - 162.5*float64(level) + 2220.0))
	}
}

type State struct {
	TotalXP  int32
	Level    int32
	Progress float32
}

func StateFromTotalXP(total int32) State {
	if total <= 0 {
		return State{TotalXP: 0, Level: 0, Progress: 0}
	}
	lvl := int32(0)
	for TotalXPForLevel(lvl+1) <= total {
		lvl++
	}
	intoLevel := total - TotalXPForLevel(lvl)
	needed := XPToNextLevel(lvl)
	prog := float32(0)
	if needed > 0 {
		prog = float32(intoLevel) / float32(needed)
		if prog < 0 {
			prog = 0
		} else if prog >= 1 {
			prog = 0.9999
		}
	}
	return State{
		TotalXP:  total,
		Level:    lvl,
		Progress: prog,
	}
}

func (s State) AddPoints(delta int32) State {
	next := s.TotalXP + delta
	if next < 0 {
		next = 0
	}
	return StateFromTotalXP(next)
}

func (s State) AddLevels(deltaLevels int32) State {
	targetLevel := s.Level + deltaLevels
	if targetLevel <= 0 {
		return State{TotalXP: 0, Level: 0, Progress: 0}
	}
	base := TotalXPForLevel(targetLevel)
	extra := int32(math.Round(float64(s.Progress) * float64(XPToNextLevel(targetLevel))))
	return StateFromTotalXP(base + extra)
}
