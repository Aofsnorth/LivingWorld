package main

import (
	"strings"
	"testing"

	"livingworld/internal/version"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

func TestJavaCheck(t *testing.T) {
	tests := []struct {
		name       string
		registry   version.LWVersion
		latest     string
		wantDrift  bool
		wantDetail string
	}{
		{
			name:      "current release",
			registry:  version.LWVersion{Label: "26 (B)", JavaProtocol: 776},
			latest:    "26.2",
			wantDrift: false,
		},
		{
			name:       "upstream ahead",
			registry:   version.LWVersion{Label: "26 (B)", JavaProtocol: 776},
			latest:     "26.3",
			wantDrift:  true,
			wantDetail: "behind upstream 26.3",
		},
		{
			name:      "matrix ahead of published",
			registry:  version.LWVersion{Label: "26 (B)", JavaProtocol: 776},
			latest:    "26.1",
			wantDrift: false,
		},
		{
			name:       "unknown release is drift, not silence",
			registry:   version.LWVersion{Label: "26 (B)", JavaProtocol: 776},
			latest:     "26.9",
			wantDrift:  true,
			wantDetail: "no entry for it",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row, drift := javaCheck(tc.registry, tc.latest, "")
			if tc.wantDrift && len(drift) == 0 {
				t.Fatalf("expected drift, got none (row up_to_date=%v detail=%q)", row.UpToDate, row.Detail)
			}
			if !tc.wantDrift && len(drift) != 0 {
				t.Fatalf("expected no drift, got %v", drift)
			}
			if tc.wantDrift && !strings.Contains(drift[0], tc.wantDetail) {
				t.Errorf("drift %q does not mention %q", drift[0], tc.wantDetail)
			}
		})
	}
}

// The previous implementation hardcoded Bedrock UpToDate=true and could never
// fail. Pin that a mismatch against the linked gophertunnel is now reported.
func TestBedrockCheckDetectsMismatch(t *testing.T) {
	ok, drift := bedrockCheck(version.LWVersion{Label: "26 (B)", BedrockProtocol: protocol.CurrentProtocol})
	if len(drift) != 0 {
		t.Fatalf("matrix matching gophertunnel reported drift: %v", drift)
	}
	if !ok.UpToDate {
		t.Error("matching protocol reported not up to date")
	}

	bad, drift := bedrockCheck(version.LWVersion{Label: "26 (B)", BedrockProtocol: protocol.CurrentProtocol - 1})
	if len(drift) == 0 {
		t.Fatal("mismatched protocol reported no drift")
	}
	if bad.UpToDate {
		t.Error("mismatched protocol reported up to date")
	}
	if !strings.Contains(drift[0], "gophertunnel") {
		t.Errorf("drift %q should name gophertunnel as the source of truth", drift[0])
	}
}

// The table must cover the release we claim to support, otherwise versioncheck
// would report drift on a correctly-pinned build.
func TestJavaProtocolForCoversCurrentMatrix(t *testing.T) {
	cur, ok := version.Current()
	if !ok {
		t.Fatal("no registered versions")
	}
	found := false
	for _, b := range cur.JavaBuilds {
		if _, ok := version.JavaProtocolFor(b); ok {
			found = true
		}
	}
	if !found {
		t.Errorf("none of the matrix Java builds %v are in the JavaProtocolFor table; "+
			"versioncheck would report false drift", cur.JavaBuilds)
	}
}
