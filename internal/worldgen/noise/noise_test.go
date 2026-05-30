package noise

import (
	"math"
	"testing"
)

func TestRNGDeterminism(t *testing.T) {
	a, b := New(42), New(42)
	for i := 0; i < 1000; i++ {
		if x, y := a.Uint64(), b.Uint64(); x != y {
			t.Fatalf("same seed diverged at %d: %d != %d", i, x, y)
		}
	}
	if New(1).Uint64() == New(2).Uint64() {
		t.Fatal("different seeds produced identical first value")
	}
}

func TestRNGFloatRange(t *testing.T) {
	r := New(7)
	for i := 0; i < 10000; i++ {
		if f := r.Float64(); f < 0 || f >= 1 {
			t.Fatalf("Float64 out of [0,1): %v", f)
		}
	}
}

func TestDeriveDecorrelated(t *testing.T) {
	if Derive(99, 1).Uint64() == Derive(99, 2).Uint64() {
		t.Fatal("different salts produced identical stream")
	}
	if x, y := Derive(99, 5).Uint64(), Derive(99, 5).Uint64(); x != y {
		t.Fatalf("same (seed,salt) not deterministic: %d != %d", x, y)
	}
}

func TestPerlinDeterminism(t *testing.T) {
	p, q := NewPerlin(123), NewPerlin(123)
	if p.Noise3D(1.5, 2.5, 3.5) != q.Noise3D(1.5, 2.5, 3.5) {
		t.Fatal("same seed gave different noise")
	}
	if NewPerlin(1).Noise2D(0.3, 0.7) == NewPerlin(2).Noise2D(0.3, 0.7) {
		t.Fatal("different seeds gave identical noise at a sample point")
	}
}

func TestPerlinRangeAndVariation(t *testing.T) {
	p := NewPerlin(2024)
	var sawPos, sawNeg bool
	for i := 0; i < 64; i++ {
		for j := 0; j < 64; j++ {
			v := p.Noise3D(float64(i)*0.13, float64(j)*0.13, 0.42)
			if math.Abs(v) > 1.0+1e-9 {
				t.Fatalf("noise out of [-1,1]: %v", v)
			}
			if v > 0 {
				sawPos = true
			} else if v < 0 {
				sawNeg = true
			}
		}
	}
	if !sawPos || !sawNeg {
		t.Fatal("noise field lacks sign variation (constant?)")
	}
}

func TestOctavesBoundedAndDeterministic(t *testing.T) {
	p := NewPerlin(5)
	v1 := p.Octaves2D(3.3, 4.4, 4, 0.5, 2.0)
	v2 := p.Octaves2D(3.3, 4.4, 4, 0.5, 2.0)
	if v1 != v2 {
		t.Fatalf("octaves not deterministic: %v != %v", v1, v2)
	}
	if math.Abs(v1) > 1.0+1e-9 {
		t.Fatalf("octaves out of [-1,1]: %v", v1)
	}
}
