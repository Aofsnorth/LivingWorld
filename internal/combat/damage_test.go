package combat

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestAfterArmor(t *testing.T) {
	cases := []struct {
		name                  string
		dmg, armor, toughness float64
		want                  float64
	}{
		{"no armor", 10, 0, 0, 10},
		{"full diamond", 10, 20, 0, 4},      // 10*(1-15/25)
		{"full netherite", 10, 20, 12, 2.8}, // toughness reduces bypass
	}
	for _, c := range cases {
		if got := AfterArmor(c.dmg, c.armor, c.toughness); !approx(got, c.want) {
			t.Errorf("%s: AfterArmor(%v,%v,%v)=%v want %v", c.name, c.dmg, c.armor, c.toughness, got, c.want)
		}
	}
}

func TestAfterResistance(t *testing.T) {
	for _, c := range []struct {
		level int
		want  float64
	}{{0, 10}, {2, 6}, {5, 0}, {6, 0}} {
		if got := AfterResistance(10, c.level); !approx(got, c.want) {
			t.Errorf("level %d: got %v want %v", c.level, got, c.want)
		}
	}
}
