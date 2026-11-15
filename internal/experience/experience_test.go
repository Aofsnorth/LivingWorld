package experience

import "testing"

func TestRoundToOrbSizeAndSplit(t *testing.T) {
	if got := RoundToOrbSize(5000); got != 2477 {
		t.Fatalf("expected 2477, got %d", got)
	}
	if got := RoundToOrbSize(20); got != 17 {
		t.Fatalf("expected 17, got %d", got)
	}
	if got := RoundToOrbSize(0); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}

	orbs := SplitIntoOrbs(25)
	want := []int32{17, 7, 1}
	if len(orbs) != len(want) {
		t.Fatalf("expected %v, got %v", want, orbs)
	}
	for i := range want {
		if orbs[i] != want[i] {
			t.Fatalf("orb[%d] = %d, want %d", i, orbs[i], want[i])
		}
	}
}

func TestXPLevelCurves(t *testing.T) {
	cases := []struct {
		level int32
		total int32
	}{
		{0, 0},
		{1, 7},
		{16, 352},
		{30, 1395},
		{31, 1507},
		{32, 1628},
	}
	for _, tc := range cases {
		got := TotalXPForLevel(tc.level)
		if got != tc.total {
			t.Fatalf("TotalXPForLevel(%d) = %d, want %d", tc.level, got, tc.total)
		}
		state := StateFromTotalXP(tc.total)
		if state.Level != tc.level {
			t.Fatalf("StateFromTotalXP(%d).Level = %d, want %d", tc.total, state.Level, tc.level)
		}
	}
}
