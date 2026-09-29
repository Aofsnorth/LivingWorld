// Command versioncheck polls the upstream Mojang manifest against the LWVersion
// matrix registered in internal/version, and reports whether the supported
// protocol numbers are still current (Master_Plan.md §6 Phase 2 / M1: "Poll
// Mojang manifest/changelog vs the matrix; report new patches and whether a new
// letter is needed").
//
// The tool is intentionally a thin, read-only CLI:
//
//	livingworld-versioncheck                # run a single check, exit 0/1
//	livingworld-versioncheck -json          # JSON output for CI gating
//	livingworld-versioncheck -matrix=false  # skip the matrix dump
//
// EXIT CODES
//
//	0  nothing to do
//	1  drift: upstream moved ahead of the local matrix, or the local Bedrock
//	   matrix disagrees with the gophertunnel we actually link against
//	2  internal error (no versions registered, flags unparsable)
//
// # JAVA SIDE
//
// Mojang removed "protocolVersion" from both version_manifest.json and
// version_manifest_v2.json, so the manifest can no longer be asked for a
// protocol number directly. What it still carries is latest.release, the id
// of the newest release. This tool therefore resolves that id through
// version.JavaProtocolFor's curated table. An id the table does not know is
// reported as drift with an explicit "add it" message rather than being
// silently treated as fine — the previous implementation did the latter, which
// is why LivingWorld stayed on 26.2/776 while 26.3/777 was already out.
//
// # BEDROCK SIDE
//
// There is no public Bedrock protocol manifest, so the previous version
// hardcoded UpToDate=true and could never fail. Instead we compare the matrix
// against gophertunnel's own compiled-in protocol.CurrentProtocol /
// CurrentVersion: that is exactly the wire version this binary will speak, so
// a disagreement is a real bug, not a guess. This check needs no network.
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

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

const mojangManifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest_v2.json"

// mojangManifest is the slice of the version manifest this tool needs: the
// id of the newest release. Note the absence of any protocol field — Mojang
// dropped it, hence version.JavaProtocolFor.
type mojangManifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
}

// fetchLatestRelease returns the id of the newest published release.
func fetchLatestRelease() (release string, snapshot string, err error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(mojangManifestURL)
	if err != nil {
		return "", "", fmt.Errorf("fetch Mojang manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", "", fmt.Errorf("Mojang manifest: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read Mojang manifest: %w", err)
	}
	var m mojangManifest
	if err := json.Unmarshal(body, &m); err != nil {
		return "", "", fmt.Errorf("decode Mojang manifest: %w", err)
	}
	if m.Latest.Release == "" {
		return "", "", fmt.Errorf("Mojang manifest has no latest.release")
	}
	return m.Latest.Release, m.Latest.Snapshot, nil
}

// matrixRow is one row of the JSON report.
type matrixRow struct {
	Edition  string   `json:"edition"`
	Label    string   `json:"label"`
	Protocol int32    `json:"protocol"`
	Builds   []string `json:"builds"`
	UpToDate bool     `json:"up_to_date"`
	Source   string   `json:"source,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

type matrixReport struct {
	GeneratedAt time.Time   `json:"generated_at"`
	Current     string      `json:"current_label"`
	LatestJava  string      `json:"latest_java_release,omitempty"`
	Rows        []matrixRow `json:"rows"`
	Drift       []string    `json:"drift,omitempty"`
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

	// Bedrock first: it is an offline comparison against what this binary can
	// actually speak, so it must not depend on the network.
	bedrockRow, bedrockDrift := bedrockCheck(cur)

	var javaRow matrixRow
	var javaDrift []string
	latestRelease, latestSnapshot, netErr := fetchLatestRelease()
	if netErr != nil {
		// Network failure is not a drift signal on its own. Say so plainly and
		// still report the local state, but do not exit non-zero for it.
		fmt.Fprintf(os.Stderr, "versioncheck: %v\n", netErr)
		fmt.Fprintln(os.Stderr, "versioncheck: skipping the Java upstream check")
		javaRow = matrixRow{
			Edition:  version.Java.String(),
			Label:    cur.Label,
			Protocol: cur.JavaProtocol,
			Builds:   append([]string(nil), cur.JavaBuilds...),
			UpToDate: true,
			Source:   "matrix (upstream check skipped)",
			Detail:   netErr.Error(),
		}
	} else {
		javaRow, javaDrift = javaCheck(cur, latestRelease, latestSnapshot)
	}

	rows := []matrixRow{javaRow, bedrockRow}
	drift := append(append([]string(nil), javaDrift...), bedrockDrift...)
	sort.Strings(drift)

	report := matrixReport{
		GeneratedAt: time.Now().UTC(),
		Current:     cur.Label,
		LatestJava:  latestRelease,
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

// javaCheck compares the matrix against the newest published Java release.
func javaCheck(cur version.LWVersion, latestRelease, latestSnapshot string) (matrixRow, []string) {
	row := matrixRow{
		Edition:  version.Java.String(),
		Label:    cur.Label,
		Protocol: cur.JavaProtocol,
		Builds:   append([]string(nil), cur.JavaBuilds...),
		Source:   "Mojang version_manifest_v2 latest.release",
		Detail:   "latest release " + latestRelease,
	}

	upstream, known := version.JavaProtocolFor(latestRelease)
	if !known {
		row.UpToDate = false
		row.Detail = "latest release " + latestRelease + " is not in version.JavaProtocolFor's table"
		return row, []string{fmt.Sprintf(
			"Mojang published %s but version.JavaProtocolFor has no entry for it; add its protocol "+
				"(known: %v) and bump the matrix if it is newer than %d",
			latestRelease, version.KnownJavaReleases(), cur.JavaProtocol,
		)}
	}

	row.Detail = fmt.Sprintf("latest release %s = protocol %d", latestRelease, upstream)
	if cur.JavaProtocol < upstream {
		row.UpToDate = false
		msg := fmt.Sprintf(
			"Java protocol %d (matrix %q) is behind upstream %s (protocol %d); a new LWVersion letter is needed",
			cur.JavaProtocol, cur.Label, latestRelease, upstream,
		)
		if latestSnapshot != "" {
			msg += fmt.Sprintf(" (a %s snapshot is also out, which is usually fine)", latestSnapshot)
		}
		return row, []string{msg}
	}
	row.UpToDate = true
	return row, nil
}

// bedrockCheck compares the matrix against gophertunnel's compiled-in protocol
// constants. A mismatch means the advertised Bedrock support is a lie: the
// binary would speak protocol.CurrentProtocol regardless of what we claim.
func bedrockCheck(cur version.LWVersion) (matrixRow, []string) {
	row := matrixRow{
		Edition:  version.Bedrock.String(),
		Label:    cur.Label,
		Protocol: cur.BedrockProtocol,
		Builds:   append([]string(nil), cur.BedrockBuilds...),
		Source:   "gophertunnel protocol.CurrentProtocol",
		Detail:   fmt.Sprintf("linked gophertunnel speaks %s (protocol %d)", protocol.CurrentVersion, protocol.CurrentProtocol),
	}
	if cur.BedrockProtocol != protocol.CurrentProtocol {
		row.UpToDate = false
		return row, []string{fmt.Sprintf(
			"Bedrock matrix claims protocol %d (builds %v) but the linked gophertunnel speaks protocol %d (%s); "+
				"update internal/version and pin the matching gophertunnel release",
			cur.BedrockProtocol, cur.BedrockBuilds, protocol.CurrentProtocol, protocol.CurrentVersion,
		)}
	}
	row.UpToDate = true
	return row, nil
}

// printHuman emits the report in a script-friendly but readable form.
func printHuman(r *matrixReport, showMatrix bool) {
	fmt.Printf("LivingWorld versioncheck @ %s\n", r.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("Current LWVersion: %s\n\n", r.Current)
	if showMatrix {
		fmt.Println("edition\tlabel\tprotocol\tbuilds\tup_to_date\tdetail")
		for _, row := range r.Rows {
			fmt.Printf("%s\t%s\t%d\t%s\t%v\t%s\n",
				row.Edition, row.Label, row.Protocol, joinComma(row.Builds), row.UpToDate, row.Detail)
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
