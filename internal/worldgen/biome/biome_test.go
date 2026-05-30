package biome

import "testing"

func TestSelectDeterministic(t *testing.T) {
	for i := 0; i < 100; i++ {
		if Select(0.6, 0.3) != Select(0.6, 0.3) {
			t.Fatal("Select not deterministic for identical input")
		}
	}
}

func TestSelectClimateExtremes(t *testing.T) {
	cases := []struct {
		name        string
		temp, humid float64
		want        string
	}{
		{"hot-dry", 0.95, 0.05, Desert.Name},
		{"cold", 0.2, 0.35, Mountains.Name},
		{"temperate-dry", 0.8, 0.4, Plains.Name},
		{"warm-wet", 0.7, 0.8, Forest.Name},
		{"mid", 0.5, 0.5, Ocean.Name},
	}
	for _, c := range cases {
		if got := Select(c.temp, c.humid).Name; got != c.want {
			t.Errorf("%s: Select(%v,%v) = %q, want %q", c.name, c.temp, c.humid, got, c.want)
		}
	}
}

func TestAllSurfacesNamed(t *testing.T) {
	for _, b := range All() {
		if b.Surface == "" || b.Filler == "" || b.Name == "" {
			t.Errorf("biome %+v has an empty name/surface/filler", b)
		}
	}
}
