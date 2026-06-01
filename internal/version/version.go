// Package version is LivingWorld's version identity, edition-aware
// negotiation, and capability surface (Master Plan §3.1, §6 Phase 2 / M1).
//
// It is the single home for:
//   - the Edition enum (Java, Bedrock),
//   - the LWVersion registry (year/letter label, protocol numbers,
//     capability bitset, supported client builds),
//   - the Resolve(edition, protocol) function used by both edges'
//     login paths to accept or reject a connecting client,
//   - the Supported() enumeration used by /lwversion and the version
//     matrix in docs/VERSION_MATRIX.md.
//
// Phase 1 of the Master Plan salvages the Edition enum out of the
// dead internal/network bridge (now removed; see Master_Plan.md §3
// audit row "internal/network"). The rest of the registry + negotiation
// surface is the Phase 2 deliverable.
//
// DESIGN §5 establishes one canonical LWVersion group ("26 (A)") per
// supported year/letter cycle. Within a group, all patches share the
// same wire protocol; capability bitset captures per-patch
// differences so downstream code (multiprotocol, command tree shaping)
// can degrade gracefully.
package version

import (
	"fmt"
	"sort"
	"sync"
)

// Edition identifies a client's wire-protocol family. It is the
// canonical cross-cut used by every package that needs to make an
// edition-specific decision at a translation boundary; gameplay
// packages must NOT branch on Edition.
type Edition uint8

const (
	// Java is the Java Edition wire family (TCP, go-mc, 1.21.x family).
	Java Edition = iota
	// Bedrock is the Bedrock Edition wire family (UDP/RakNet, gophertunnel,
	// 1.26.x family).
	Bedrock
)

// String returns the lowercase, human-readable name of the edition.
func (e Edition) String() string {
	switch e {
	case Java:
		return "java"
	case Bedrock:
		return "bedrock"
	default:
		return fmt.Sprintf("edition(%d)", uint8(e))
	}
}

// AllEditions lists every edition Supported() enumerates. Useful for
// tests and version-matrix generators.
var AllEditions = []Edition{Java, Bedrock}

// Capability is a single feature flag a given LWVersion may or may not
// have. Bitset layout is stable per LWVersion group; new bits must be
// added at the next unused index so consumers can do bitwise tests
// without re-basing.
type Capability uint32

const (
	// CapCustomPlayerModels marks versions that understand the
	// Java 1.21.2+ custom_model_data dimension split and the
	// Bedrock 1.21+ item component overlay. Multiprotocol chains
	// (Phase 7b) consult this to choose the right translator stage.
	CapCustomPlayerModels Capability = 1 << iota
	// CapNetherUpdateNotes marks versions where the nether
	// coordinate scale (1:8) is enforced client-side. Server-side
	// enforcement is unaffected.
	CapNetherUpdateNotes
)

// LWVersion is a single named, year+letter version group supported by
// LivingWorld (DESIGN §5 / Master Plan §6 Phase 2). All clients within
// one group share the same wire protocol for each edition; capability
// bitset + per-patch build list capture the deltas.
type LWVersion struct {
	// Label is the user-facing version label, e.g. "26 (A)". Year is
	// the Minecraft release year, Letter is the in-year group
	// (A=first supported cycle of the year).
	Label string
	// JavaProtocol is the canonical Java Edition protocol number
	// (e.g. 775 for 1.21.1). All Java clients within the group use
	// this protocol; sub-patches are matched by JavaBuilds.
	JavaProtocol int32
	// JavaBuilds is the set of accepted human-readable Java build
	// identifiers within the group, e.g. {"1.21.1", "1.21.2"}.
	JavaBuilds []string
	// BedrockProtocol is the canonical Bedrock Edition protocol
	// number (e.g. 975 for 1.26.1.x). All Bedrock clients within
	// the group use this protocol.
	BedrockProtocol int32
	// BedrockBuilds is the set of accepted Bedrock build identifiers
	// within the group, e.g. {"1.26.10", "1.26.20", "1.26.23"}.
	BedrockBuilds []string
	// Capabilities is the version group's capability bitset.
	Capabilities Capability
	// ChangelogURL is a human-readable changelog the /lwversion
	// command and --version flag link to.
	ChangelogURL string
}

// Has reports whether the LWVersion exposes the given capability bit.
func (v LWVersion) Has(c Capability) bool { return v.Capabilities&c != 0 }

// registry is the package-private, append-once list of supported
// versions. Phase 2's contract is "all consumers read from Supported()"
// so a future bump is a one-file change.
var (
	regMu  sync.RWMutex
	registry = []LWVersion{
		{
			Label:           "26 (A)",
			JavaProtocol:    775,
			JavaBuilds:      []string{"1.21.1", "1.21.2"},
			BedrockProtocol: 975,
			BedrockBuilds:   []string{"1.26.10", "1.26.20", "1.26.23"},
			Capabilities:    CapCustomPlayerModels | CapNetherUpdateNotes,
			ChangelogURL:    "https://github.com/Aofsnorth/LivingWorld/releases/tag/v26-A",
		},
	}
)

// Supported returns a copy of the registered version list in
// registration order. Callers MUST NOT mutate the result.
func Supported() []LWVersion {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]LWVersion, len(registry))
	copy(out, registry)
	return out
}

// RegisterVersion appends a version to the registry. Intended for
// tests and for cmd/versioncheck which may inject a freshly
// discovered version. Production code reads via Supported() / Resolve.
func RegisterVersion(v LWVersion) {
	regMu.Lock()
	registry = append(registry, v)
	regMu.Unlock()
}

// Current returns the highest-priority (first-registered) supported
// version. Phase 7 multiprotocol chains negotiate against every
// supported version, but the simplest case ("/lwversion", --version,
// version banners) reports the current one.
func Current() (LWVersion, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	if len(registry) == 0 {
		return LWVersion{}, false
	}
	return registry[0], true
}

// Resolve looks up the LWVersion that owns the given (edition,
// protocol) pair, returning ok=false if the protocol is not in any
// supported group. The match is exact: the audit explicitly requires
// "exact match → serve; unsupported → disconnect" (Master Plan §6
// Phase 2), so partial or fuzzy matching is intentionally absent.
//
// Callers are the Java/Bedrock login handlers; they translate the
// wire protocol id from the handshake packet and ask Resolve. The
// returned LWVersion is then used to look up capabilities and
// supported client builds, and to build the human-readable
// disconnect message when ok=false.
func Resolve(edition Edition, protocol int32) (LWVersion, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	for _, v := range registry {
		switch edition {
		case Java:
			if v.JavaProtocol == protocol {
				return v, true
			}
		case Bedrock:
			if v.BedrockProtocol == protocol {
				return v, true
			}
		}
	}
	return LWVersion{}, false
}

// SortedBuilds returns a sorted copy of the build list for a given
// edition in the current LWVersion. Useful for emitting deterministic
// banners and the /lwversion command output.
func SortedBuilds(e Edition) []string {
	v, ok := Current()
	if !ok {
		return nil
	}
	var src []string
	switch e {
	case Java:
		src = v.JavaBuilds
	case Bedrock:
		src = v.BedrockBuilds
	default:
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	sort.Strings(out)
	return out
}
