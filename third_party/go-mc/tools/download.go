package main

// download.go — Downloads MC server jar and language files before container extraction.
//
// This is Phase 1 of the extraction pipeline: the Go host downloads all
// data that needs internet access so the container runs network-free.
//
// Downloads:
//   - Server jar: from unobfuscated_versions.json (pre-26.x) or Mojang manifest (26.x+)
//   - Language files: all ~137 languages from Mojang asset index CDN

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const manifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest_v2.json"
const assetCDNBase = "https://resources.download.minecraft.net"

// --- Mojang API JSON types ---

type mojangManifest struct {
	Versions []mojangManifestVersion `json:"versions"`
}

type mojangManifestVersion struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type mojangVersionDetail struct {
	Downloads struct {
		Server struct {
			URL  string `json:"url"`
			SHA1 string `json:"sha1"`
			Size int64  `json:"size"`
		} `json:"server"`
	} `json:"downloads"`
	AssetIndex struct {
		URL string `json:"url"`
	} `json:"assetIndex"`
}

type mojangAssetIndex struct {
	Objects map[string]struct {
		Hash string `json:"hash"`
		Size int64  `json:"size"`
	} `json:"objects"`
}

type unobfuscatedEntry struct {
	ServerSHA1 string `json:"server_sha1"`
	ServerURL  string `json:"server_url"`
}

// --- Server jar download ---

// downloadServerJar downloads the MC server jar to cacheDir/<version>-server.jar.
// For pre-26.x versions, uses tools/unobfuscated_versions.json.
// For 26.x+ versions, uses the standard Mojang version manifest.
// Returns the cached jar path.
func downloadServerJar(cacheDir, goMCRoot, version string) (string, error) {
	jarPath := filepath.Join(cacheDir, version+"-server.jar")

	if fi, err := os.Stat(jarPath); err == nil && fi.Size() > 0 {
		logf("  Server jar cached: %s (%s)", jarPath, humanSize(fi.Size()))
		return jarPath, nil
	}

	url, err := resolveServerJarURL(goMCRoot, version)
	if err != nil {
		return "", err
	}

	logf("Downloading server jar for %s...", version)
	if err := httpDownloadFile(url, jarPath); err != nil {
		return "", fmt.Errorf("downloading server jar: %v", err)
	}

	return jarPath, nil
}

// resolveServerJarURL returns the download URL for the MC server jar.
func resolveServerJarURL(goMCRoot, version string) (string, error) {
	// Check unobfuscated_versions.json for hardcoded entries (pre-26.x).
	jsonFile := filepath.Join(goMCRoot, "tools", "unobfuscated_versions.json")
	if data, err := os.ReadFile(jsonFile); err == nil {
		var versions map[string]unobfuscatedEntry
		if err := json.Unmarshal(data, &versions); err == nil {
			if entry, ok := versions[version]; ok {
				logf("  Using unobfuscated URL from unobfuscated_versions.json")
				return entry.ServerURL, nil
			}
		}
	}

	// Fall back to standard Mojang version manifest.
	logf("  Fetching Mojang version manifest...")
	detail, err := fetchVersionDetail(version)
	if err != nil {
		return "", err
	}

	if detail.Downloads.Server.URL == "" {
		return "", fmt.Errorf("no server download URL for version %s", version)
	}

	return detail.Downloads.Server.URL, nil
}

// --- Language file download ---

// downloadLangFiles downloads all Minecraft language files from the Mojang
// asset index CDN to jsonsDir/<version>/lang/.
func downloadLangFiles(jsonsDir, version string) error {
	langDir := filepath.Join(jsonsDir, version, "lang")
	if err := os.MkdirAll(langDir, 0o755); err != nil {
		return fmt.Errorf("creating lang dir: %v", err)
	}

	detail, err := fetchVersionDetail(version)
	if err != nil {
		return fmt.Errorf("fetching version detail for lang download: %v", err)
	}

	if detail.AssetIndex.URL == "" {
		return fmt.Errorf("no asset index URL for version %s", version)
	}

	var assetIndex mojangAssetIndex
	if err := httpGetJSON(detail.AssetIndex.URL, &assetIndex); err != nil {
		return fmt.Errorf("fetching asset index: %v", err)
	}

	// Find all minecraft/lang/*.json entries.
	type langFileEntry struct {
		name string
		hash string
	}
	var langs []langFileEntry
	for key, obj := range assetIndex.Objects {
		if strings.HasPrefix(key, "minecraft/lang/") && strings.HasSuffix(key, ".json") {
			name := key[len("minecraft/lang/"):]
			langs = append(langs, langFileEntry{name, obj.Hash})
		}
	}

	if len(langs) == 0 {
		logf("  WARNING: no language files found in asset index")
		return nil
	}

	var downloaded, skipped int
	for _, l := range langs {
		dest := filepath.Join(langDir, l.name)
		if fi, err := os.Stat(dest); err == nil && fi.Size() > 0 {
			skipped++
			continue
		}

		url := assetCDNBase + "/" + l.hash[:2] + "/" + l.hash
		if err := httpDownloadFile(url, dest); err != nil {
			logf("  WARNING: failed to download %s: %v", l.name, err)
			continue
		}
		downloaded++
	}

	// Count total size.
	var totalSize int64
	entries, _ := os.ReadDir(langDir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			totalSize += info.Size()
		}
	}

	logf("  Languages: %d files (%s total), %d downloaded, %d cached",
		len(langs), humanSize(totalSize), downloaded, skipped)
	return nil
}

// --- Mojang API helpers ---

// fetchVersionDetail fetches the Mojang version manifest and returns the
// detail JSON for the given version.
func fetchVersionDetail(version string) (*mojangVersionDetail, error) {
	var manifest mojangManifest
	if err := httpGetJSON(manifestURL, &manifest); err != nil {
		return nil, fmt.Errorf("fetching version manifest: %v", err)
	}

	var versionURL string
	for _, v := range manifest.Versions {
		if v.ID == version {
			versionURL = v.URL
			break
		}
	}
	if versionURL == "" {
		return nil, fmt.Errorf("version %s not found in Mojang manifest", version)
	}

	var detail mojangVersionDetail
	if err := httpGetJSON(versionURL, &detail); err != nil {
		return nil, fmt.Errorf("fetching version detail for %s: %v", version, err)
	}

	return &detail, nil
}

// --- HTTP helpers ---

func httpGetJSON(url string, v any) error {
	data, err := httpGet(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func httpGet(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}

func httpDownloadFile(url, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	start := time.Now()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		os.Remove(destPath)
		return err
	}

	elapsed := time.Since(start).Round(time.Millisecond)
	logf("  Downloaded %s in %s → %s", humanSize(n), elapsed, destPath)
	return nil
}
