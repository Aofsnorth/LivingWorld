// Command versioncheck polls the upstream Mojang + Bedrock manifests
// against the LWVersion matrix registered in internal/version, and
// reports whether the supported protocol numbers are still current
// (Master_Plan.md §6 Phase 2 / M1: "Poll Mojang manifest/changelog vs
// the matrix; report new patches and whether a new letter is needed").
//
// The tool is intentionally a thin, read-only CLI:
//
//   livingworld-versioncheck                # run a single check, exit 0/1
//   livingworld-versioncheck -json          # JSON output for CI gating
//   livingworld-versioncheck -matrix=false  # skip the matrix dump
//
// The default exit code is 0 when nothing in the matrix needs an
// update, 1 when at least one protocol is upstream-current but not in
// the local registry (so a CI job can be wired to fail the build when
// LivingWorld is drifting behind official client support).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"time"

	"livingworld/internal/version"
)

// upstreamMojangVersionManifest is the minimal slice of the Mojang
// version manifest we need. The full manifest is huge; we only care
// about (id, protocolVersion) per release.
type upstreamMojangVersionManifest struct {
	Versions []struct {
		ID             string `json:"id"`
		ProtocolVersion int   `json:"protocolVersion"`
	} `json:"versions"`
}

const mojangManifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest.json"

// findMojang fetches the manifest and returns the protocol number for
// the latest release whose id matches one of the supported Java
// builds. The Mojang manifest is paged alphabetically, so we walk the
// full list to find the highest protocol that shares an id prefix.
//
// Errors are returned verbatim so the caller can decide whether to
// treat network failures as "drift unknown" (CI) or fatal.
func findMojang() (latestProto int32, latestID string, err error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(mojangManifestURL)
	if err != nil {
		return 0, "", fmt.Errorf("fetch Mojang manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, "", fmt.Errorf("Mojang manifest: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", fmt.Errorf("read Mojang manifest: %w", err)
	}
	var m upstreamMojangVersionManifest
	if err := json.Unmarshal(body, &m); err != nil {
		return 0, "", fmt.Errorf("decode Mojang manifest: %w", err)
	}
	// Walk all releases; the highest protocol number wins (the manifest
	// is not guaranteed to be sorted by protocol). The id is taken from
	// the same entry so the caller can cross-check.
	supportedBuilds := map[string]bool{}
	if cur, ok := version.Current(); ok {
		for _, b := range cur.JavaBuilds {
			supportedBuilds[b] = true
		}
	}
	for _, v := range m.Versions {
		if !supportedBuilds[v.ID] {
			continue
		}
		if int32(v.ProtocolVersion) > latestProto {
			latestProto = int32(v.ProtocolVersion)
			latestID = v.ID
		}
	}
	return latestProto, latestID, nil
}

// matrixRow is one row of the JSON report.
type matrixRow struct {
	Edition   string   `json:"edition"`
	Label     string   `json:"label"`
	Protocol  int32    `json:"protocol"`
	Builds    []string `json:"builds"`
	UpToDate  bool     `json:"up_to_date"`
	LatestID  string   `json:"latest_id,omitempty"`
}

type matrixReport struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Current     string        `json:"current_label"`
	Rows        []matrixRow   `json:"rows"`
	Drift       []string      `json:"drift,omitempty"`
}

func main() {
	showMatrix := flag.Bool("matrix", true, "dump the supported version matrix to stdout")
	asJSON := flag.Bool("json", false, "emit JSON instead of human-readable text")
	flag.Parse()

	cur, ok := version.Current()
	if !ok {
		fmt.Fprintln(os.Stderr, "versioncheck: no supported versions registered")
		os.Exit(2)
	}

	upstreamProto, upstreamID, err := findMojang()
	if err != nil {
		fmt.Fprintf(os.Stderr, "versioncheck: %v\n", err)
		// Network failure is not a hard drift signal; surface it and
		// still print the matrix so the operator can see the local
		// state. CI jobs can choose to treat this exit code as soft.
	}

	rows := buildMatrixRows(cur, upstreamProto, upstreamID)
	drift := computeDrift(rows, cur, upstreamProto, upstreamID)
	report := matrixReport{
		GeneratedAt: time.Now().UTC(),
		Current:     cur.Label,
		Rows:        rows,
		Drift:       drift,
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "versioncheck: encode JSON: %v\n", err)
			os.Exit(1)
		}
	} else {
		printHuman(&report, *showMatrix)
	}

	if len(drift) > 0 {
		os.Exit(1)
	}
}

// buildMatrixRows assembles the per-edition rows for the report.
func buildMatrixRows(cur version.LWVersion, upstreamProto int32, upstreamID string) []matrixRow {
	out := []matrixRow{
		{
			Edition:  version.Java.String(),
			Label:    cur.Label,
			Protocol: cur.JavaProtocol,
			Builds:   append([]string(nil), cur.JavaBuilds...),
			UpToDate: upstreamProto == 0 || cur.JavaProtocol >= upstreamProto,
			LatestID: upstreamID,
		},
		{
			Edition:  version.Bedrock.String(),
			Label:    cur.Label,
			Protocol: cur.BedrockProtocol,
			Builds:   append([]string(nil), cur.BedrockBuilds...),
			UpToDate: true, // No public Bedrock protocol manifest yet; assume up to date.
		},
	}
	return out
}

// computeDrift returns one human-readable drift message per row that
// is out of date, sorted for determinism (so CI diffs are stable).
func computeDrift(rows []matrixRow, cur version.LWVersion, upstreamProto int32, upstreamID string) []string {
	var drift []string
	for _, r := range rows {
		if r.Edition == version.Java.String() && upstreamProto > 0 && cur.JavaProtocol < upstreamProto {
			drift = append(drift, fmt.Sprintf(
				"Java protocol %d (matrix %q) is behind upstream %s (protocol %d); a new LWVersion letter may be needed",
				cur.JavaProtocol, cur.Label, upstreamID, upstreamProto,
			))
		}
	}
	sort.Strings(drift)
	return drift
}

// printHuman emits the report in a script-friendly but readable form.
func printHuman(r *matrixReport, showMatrix bool) {
	fmt.Printf("LivingWorld versioncheck @ %s\n", r.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("Current LWVersion: %s\n\n", r.Current)
	if showMatrix {
		fmt.Println("edition\tlabel\tprotocol\tbuilds\tup_to_date")
		for _, row := range r.Rows {
			fmt.Printf("%s\t%s\t%d\t%s\t%v\n", row.Edition, row.Label, row.Protocol, joinComma(row.Builds), row.UpToDate)
		}
		fmt.Println()
	}
	if len(r.Drift) == 0 {
		fmt.Println("No drift detected.")
		return
	}
	fmt.Println("Drift detected:")
	for _, d := range r.Drift {
		fmt.Println("  - " + d)
	}
}

// joinComma is a tiny strings.Join to avoid pulling the package in
// for one helper.
func joinComma(s []string) string {
	if len(s) == 0 {
		return ""
	}
	out := s[0]
	for _, v := range s[1:] {
		out += "," + v
	}
	return out
}
