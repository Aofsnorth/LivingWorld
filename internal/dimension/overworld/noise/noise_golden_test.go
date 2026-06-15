package noise

import (
	"math"
	"testing"
)

// These tests anchor the PRNG/noise layer's faithfulness.
//
// The Legacy backend is checked against java.util.Random's famous published
// outputs (a true external oracle). splitmix64 is checked against its canonical
// seed-0 vector. The xoroshiro128++ scrambler is cross-checked against an
// independent shift-based reference implementation in this file (catching
// formula / rotation-direction bugs). Bit-for-bit cross-checking of Mojang's
// XoroshiroRandomSource against a live Java instance is a follow-up once an
// oracle is available — the algorithm here is implemented to the documented
// spec (upgradeSeedTo128bit + xoroshiro128++).

// --- splitmix64 / Stafford ---------------------------------------------------

func TestSplitMix64KnownVector(t *testing.T) {
	// Canonical splitmix64(seed=0) first output.
	if got := SplitMix64(0); got != 0xE220A8397B1DCDAF {
		t.Fatalf("SplitMix64(0) = %#x, want 0xE220A8397B1DCDAF", got)
	}
}

func TestMixStafford13Consistency(t *testing.T) {
	for _, x := range []uint64{0, 1, 0xDEADBEEF, 0xFFFFFFFFFFFFFFFF} {
		if SplitMix64(x) != mixStafford13(x+goldenRatio64) {
			t.Fatalf("SplitMix64/mixStafford13 mismatch at %#x", x)
		}
	}
}

// --- xoroshiro128++ ----------------------------------------------------------

// referenceXoroshiroNext is an independent xoroshiro128++ step using explicit
// shift-based rotation, used to validate the production next().
func referenceXoroshiroNext(s *[2]uint64) uint64 {
	rotl := func(v uint64, k uint) uint64 { return (v << k) | (v >> (64 - k)) }
	s0, s1 := s[0], s[1]
	result := rotl(s0+s1, 17) + s0
	s1 ^= s0
	s[0] = rotl(s0, 49) ^ s1 ^ (s1 << 21)
	s[1] = rotl(s1, 28)
	return result
}

func TestXoroshiroNextMatchesReference(t *testing.T) {
	x := NewXoroshiroRaw(0x0123456789ABCDEF, 0xFEDCBA9876543210)
	ref := [2]uint64{0x0123456789ABCDEF, 0xFEDCBA9876543210}
	for i := 0; i < 64; i++ {
		want := referenceXoroshiroNext(&ref)
		if got := uint64(x.NextLong()); got != want {
			t.Fatalf("next #%d = %#x, want %#x", i, got, want)
		}
	}
}

func TestXoroshiroSeedingDeterministicAndGuarded(t *testing.T) {
	a, b := NewXoroshiro(12345), NewXoroshiro(12345)
	for i := 0; i < 16; i++ {
		if a.NextLong() != b.NextLong() {
			t.Fatal("same seed diverged")
		}
	}
	// all-zero state guard: (0,0) must not be a fixed point.
	z := NewXoroshiroRaw(0, 0)
	if z.NextLong() == 0 && z.NextLong() == 0 {
		t.Fatal("all-zero state not guarded")
	}
}

func TestXoroshiroDoubleRange(t *testing.T) {
	x := NewXoroshiro(99)
	for i := 0; i < 10000; i++ {
		if d := x.NextDouble(); d < 0 || d >= 1 {
			t.Fatalf("NextDouble out of range: %v", d)
		}
	}
}

// --- Legacy (java.util.Random) external vectors ------------------------------

func TestLegacyJavaUtilRandomVectors(t *testing.T) {
	r := NewLegacy(0)
	if v := r.NextInt(); v != -1155484576 {
		t.Fatalf("Legacy(0).NextInt()#1 = %d, want -1155484576", v)
	}
	if v := r.NextInt(); v != -723955400 {
		t.Fatalf("Legacy(0).NextInt()#2 = %d, want -723955400", v)
	}
	if v := NewLegacy(0).NextLong(); v != -4962768465676381896 {
		t.Fatalf("Legacy(0).NextLong() = %d, want -4962768465676381896", v)
	}
	if v := NewLegacy(0).NextDouble(); math.Abs(v-0.730967787376657) > 1e-15 {
		t.Fatalf("Legacy(0).NextDouble() = %v, want ~0.730967787376657", v)
	}
}

// --- Perlin / NormalNoise ----------------------------------------------------

func TestPerlinDeterministicAndBounded(t *testing.T) {
	p, q := NewPerlin(42), NewPerlin(42)
	for i := 0; i < 2000; i++ {
		x := float64(i) * 0.1
		a := p.Noise3D(x, x*0.5, x*0.25)
		if a != q.Noise3D(x, x*0.5, x*0.25) {
			t.Fatal("perlin nondeterministic")
		}
		if a < -1.6 || a > 1.6 {
			t.Fatalf("perlin out of range: %v", a)
		}
	}
}

func TestNormalNoiseRangeAndDeterminism(t *testing.T) {
	amps := []float64{1, 1, 1, 1, 1, 1, 1, 1}
	n := NewNormalNoiseFromParams(NewXoroshiro(7), -7, amps)
	m := NewNormalNoiseFromParams(NewXoroshiro(7), -7, amps)
	var maxAbs float64
	for i := 0; i < 5000; i++ {
		x := float64(i) * 1.7
		v := n.Sample(x, 0, x*0.3)
		if v != m.Sample(x, 0, x*0.3) {
			t.Fatal("normalnoise nondeterministic")
		}
		if a := math.Abs(v); a > maxAbs {
			maxAbs = a
		}
	}
	if maxAbs == 0 || maxAbs > 4 {
		t.Fatalf("normalnoise suspicious range, maxAbs=%v", maxAbs)
	}
}

func TestPositionalSeedingNameAndPositionSensitive(t *testing.T) {
	t1 := NewXoroshiro(1).ForkPositional().FromHashOf("minecraft:temperature").NextLong()
	t2 := NewXoroshiro(1).ForkPositional().FromHashOf("minecraft:temperature").NextLong()
	if t1 != t2 {
		t.Fatal("fromHashOf nondeterministic")
	}
	if e := NewXoroshiro(1).ForkPositional().FromHashOf("minecraft:erosion").NextLong(); e == t1 {
		t.Fatal("different noise names produced the same stream")
	}
	p1 := NewXoroshiro(2).ForkPositional().At(1, 2, 3).NextLong()
	p2 := NewXoroshiro(2).ForkPositional().At(4, 5, 6).NextLong()
	if p1 == p2 {
		t.Fatal("different positions produced the same stream")
	}
}
