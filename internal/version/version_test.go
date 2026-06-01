// Package version tests: registry resolve + sort + capability bitset.
// These tests pin the contract Phase 2 of Master_Plan.md commits to:
// "exact match → serve; unsupported → disconnect".
package version

import "testing"

func TestResolveExactMatch(t *testing.T) {
	// Default registry carries 26 (A): Java 775, Bedrock 975.
	if v, ok := Resolve(Java, 775); !ok {
		t.Fatalf("Resolve(Java, 775): expected match, got ok=false")
	} else if v.Label != "26 (A)" {
		t.Errorf("Resolve(Java, 775).Label = %q, want %q", v.Label, "26 (A)")
	}
	if v, ok := Resolve(Bedrock, 975); !ok {
		t.Fatalf("Resolve(Bedrock, 975): expected match, got ok=false")
	} else if v.BedrockProtocol != 975 {
		t.Errorf("Resolve(Bedrock, 975).BedrockProtocol = %d, want 975", v.BedrockProtocol)
	}
}

func TestResolveUnsupported(t *testing.T) {
	// Master Plan §6 Phase 2 contract: unsupported protocol must
	// return ok=false so the login handler can disconnect cleanly.
	if _, ok := Resolve(Java, 999); ok {
		t.Fatalf("Resolve(Java, 999): expected ok=false (unsupported), got ok=true")
	}
	if _, ok := Resolve(Bedrock, 0); ok {
		t.Fatalf("Resolve(Bedrock, 0): expected ok=false, got ok=true")
	}
}

func TestEditionString(t *testing.T) {
	if got := Java.String(); got != "java" {
		t.Errorf("Java.String() = %q, want %q", got, "java")
	}
	if got := Bedrock.String(); got != "bedrock" {
		t.Errorf("Bedrock.String() = %q, want %q", got, "bedrock")
	}
	if got := Edition(255).String(); got != "edition(255)" {
		t.Errorf("Edition(255).String() = %q, want %q", got, "edition(255)")
	}
}

func TestSupportedNonEmptyAndCurrent(t *testing.T) {
	got := Supported()
	if len(got) == 0 {
		t.Fatalf("Supported(): empty registry")
	}
	cur, ok := Current()
	if !ok {
		t.Fatalf("Current(): expected ok=true, got ok=false")
	}
	if cur.Label != got[0].Label {
		t.Errorf("Current().Label = %q, want %q (first in registry)", cur.Label, got[0].Label)
	}
}

func TestRegisterAndResolve(t *testing.T) {
	// Register a synthetic version, resolve it, then unregister via
	// snapshot. The "fake" version is appended, so Resolve for its
	// protocol id returns ok=true.
	const fakeProto int32 = 999999
	fake := LWVersion{
		Label:           "TEST (X)",
		JavaProtocol:    fakeProto,
		JavaBuilds:      []string{"0.0.0-test"},
		BedrockProtocol: fakeProto,
		BedrockBuilds:   []string{"0.0.0-test"},
		Capabilities:    CapCustomPlayerModels,
		ChangelogURL:    "https://example.invalid/changelog",
	}
	RegisterVersion(fake)
	defer func() {
		// Drop the test entry so the rest of the suite is unaffected.
		// We can't delete from a slice; rebuild from a snapshot minus
		// the test entry.
		regMu.Lock()
		filtered := registry[:0]
		for _, v := range registry {
			if v.Label != fake.Label {
				filtered = append(filtered, v)
			}
		}
		registry = filtered
		regMu.Unlock()
	}()

	if v, ok := Resolve(Java, fakeProto); !ok || v.Label != fake.Label {
		t.Errorf("Resolve(Java, %d) after Register: got (%q,%v), want (%q,true)", fakeProto, v.Label, ok, fake.Label)
	}
}

func TestCapabilities(t *testing.T) {
	v, ok := Current()
	if !ok {
		t.Fatalf("Current(): expected ok=true")
	}
	// 26 (A) must carry both capability bits per DESIGN §5.
	if !v.Has(CapCustomPlayerModels) {
		t.Errorf("current version missing CapCustomPlayerModels")
	}
	if !v.Has(CapNetherUpdateNotes) {
		t.Errorf("current version missing CapNetherUpdateNotes")
	}
	// A capability bit we never set must read false.
	const capUnknown Capability = 1 << 30
	if v.Has(capUnknown) {
		t.Errorf("current version unexpectedly has capUnknown (1<<30)")
	}
}

func TestSortedBuilds(t *testing.T) {
	// SortedBuilds should return the current version's build list for
	// the given edition, sorted ascending.
	java := SortedBuilds(Java)
	if len(java) == 0 {
		t.Fatalf("SortedBuilds(Java): empty")
	}
	for i := 1; i < len(java); i++ {
		if java[i-1] > java[i] {
			t.Errorf("SortedBuilds(Java) not sorted: %v", java)
		}
	}
	bedrock := SortedBuilds(Bedrock)
	if len(bedrock) == 0 {
		t.Fatalf("SortedBuilds(Bedrock): empty")
	}
	for i := 1; i < len(bedrock); i++ {
		if bedrock[i-1] > bedrock[i] {
			t.Errorf("SortedBuilds(Bedrock) not sorted: %v", bedrock)
		}
	}
}
