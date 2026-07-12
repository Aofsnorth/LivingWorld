package main

// extract.go — Runs MC data extraction in a 3-phase pipeline:
//
//  1. Go host downloads server jar + language files (internet required)
//  2. Container extracts data from the jar (no internet needed)
//  3. Go host generates Go source from the extracted JSON
//
// Directory layout (relative to go-mc root):
//
//	tools/java/                     Java extractor sources (committed)
//	temp/cache/                     cached server jars + libs (gitignored)
//	temp/jsons/<version>/           output JSON files (gitignored)

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const jdkImage = "docker.io/library/eclipse-temurin:21-jdk"

// runExtract runs the MC data extraction pipeline.
// Phase 1: Download server jar + language files (Go host).
// Phase 2: Extract data inside container (no internet).
// Returns the path to the JSON output directory.
func runExtract(goMCRoot, version, runtime string, dryRun bool) (string, error) {
	cacheDir := filepath.Join(goMCRoot, "temp", "cache")
	jsonsDir := filepath.Join(goMCRoot, "temp", "jsons")
	javaDir := filepath.Join(goMCRoot, "tools", "java")

	// Ensure directories exist.
	for _, d := range []string{cacheDir, jsonsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", fmt.Errorf("creating directory %s: %v", d, err)
		}
	}

	// Check ExtractAll.java exists.
	entryFile := filepath.Join(javaDir, "ExtractAll.java")
	if _, err := os.Stat(entryFile); err != nil {
		return "", fmt.Errorf("ExtractAll.java not found at %s", entryFile)
	}

	// Phase 1: Download MC data on the Go host (needs internet).
	fmt.Fprintln(os.Stderr, "Phase 1: Downloading MC data (Go host)...")

	if _, err := downloadServerJar(cacheDir, goMCRoot, version); err != nil {
		return "", fmt.Errorf("downloading server jar: %v", err)
	}

	if err := downloadLangFiles(jsonsDir, version); err != nil {
		return "", fmt.Errorf("downloading language files: %v", err)
	}

	// Phase 2: Extract data inside container (no internet needed).
	fmt.Fprintln(os.Stderr)
	if runtime == "host" {
		fmt.Fprintln(os.Stderr, "Phase 2: Extracting data on host directly...")

		classDir := filepath.Join(cacheDir, "extract-classes")
		if err := os.MkdirAll(classDir, 0o755); err != nil {
			return "", fmt.Errorf("creating extract class dir: %w", err)
		}
		if dryRun {
			fmt.Fprintf(os.Stderr, "  command: javac -d %s %s && java -cp %s ExtractAll %s\n",
				classDir, filepath.Join(javaDir, "ExtractAll.java"), classDir, version)
			return filepath.Join(jsonsDir, version), nil
		}

		compile := exec.Command("javac", "-d", classDir, filepath.Join(javaDir, "ExtractAll.java"))
		compile.Stdout = os.Stdout
		compile.Stderr = os.Stderr
		if err := compile.Run(); err != nil {
			return "", fmt.Errorf("compiling ExtractAll.java: %w", err)
		}

		cmd := exec.Command("java", "-cp", classDir, "ExtractAll", version)
		cmd.Env = append(os.Environ(),
			"MC_CACHE_DIR="+cacheDir,
			"MC_JSONS_DIR="+jsonsDir,
			"MC_JAVA_DIR="+javaDir,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("host java extraction failed: %w", err)
		}
		outputDir := filepath.Join(jsonsDir, version)
		return outputDir, nil
	}

	fmt.Fprintln(os.Stderr, "Phase 2: Extracting data in container (no internet)...")

	containerArgs := buildContainerArgs(runtime, version, cacheDir, jsonsDir, javaDir)

	fmt.Fprintf(os.Stderr, "  version=%s runtime=%s\n", version, runtime)
	fmt.Fprintf(os.Stderr, "  cache:      %s\n", cacheDir)
	fmt.Fprintf(os.Stderr, "  jsons:      %s\n", jsonsDir)
	fmt.Fprintf(os.Stderr, "  java:       %s\n", javaDir)
	fmt.Fprintf(os.Stderr, "  image:      %s\n\n", jdkImage)

	if dryRun {
		fmt.Fprintf(os.Stderr, "  command: %s %s\n", runtime, strings.Join(containerArgs, " "))
		return filepath.Join(jsonsDir, version), nil
	}

	// Pull image if needed.
	fmt.Fprintf(os.Stderr, "Ensuring image %s is available...\n", jdkImage)
	pull := exec.Command(runtime, "pull", "--quiet", jdkImage)
	pull.Stdout = os.Stdout
	pull.Stderr = os.Stderr
	if err := pull.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: pull failed (may already be cached): %v\n", err)
	}

	// Run container.
	fmt.Fprintf(os.Stderr, "\nStarting extraction container...\n\n")
	cmd := exec.Command(runtime, containerArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("container failed: %v", err)
	}

	outputDir := filepath.Join(jsonsDir, version)

	// Report results.
	fmt.Fprintln(os.Stderr)
	reportExtractResults(outputDir)

	return outputDir, nil
}

func buildContainerArgs(runtime, version, cacheDir, jsonsDir, javaDir string) []string {
	args := []string{
		"run", "--rm",
		"--security-opt", "label=disable", // disable SELinux for volume access
		"-v", cacheDir + ":/cache",
		"-v", jsonsDir + ":/jsons",
		"-v", javaDir + ":/java:ro",
	}

	// Use current user's UID/GID so output files are owned by host user.
	if runtime != "podman" {
		// Docker: map container user to host user.
		uid := os.Getuid()
		gid := os.Getgid()
		if uid > 0 {
			args = append(args, "--user", fmt.Sprintf("%d:%d", uid, gid))
		}
	}

	args = append(args, jdkImage, "java", "--source", "21", "/java/ExtractAll.java", version)
	return args
}

func reportExtractResults(outputDir string) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  No output files found in %s\n", outputDir)
		return
	}

	fmt.Fprintf(os.Stderr, "Output files in %s:\n", outputDir)
	var totalSize int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		totalSize += info.Size()
		fmt.Fprintf(os.Stderr, "  %-30s %s\n", e.Name(), humanSize(info.Size()))
	}
	fmt.Fprintf(os.Stderr, "  %-30s %s\n", "TOTAL", humanSize(totalSize))
}

func detectRuntime() string {
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	return ""
}

// ExtractWorldgenData pulls worldgen JSON and NBT files from a Minecraft server
// jar into outDir. The jar may be the bundled distribution jar (which contains an
// inner server jar) or the inner server jar itself.
func ExtractWorldgenData(jar, outDir string) error {
	zr, err := zip.OpenReader(jar)
	if err != nil {
		return err
	}
	defer zr.Close()

	// If the supplied jar is a bundled distribution (1.21+), extract and process
	// the inner server jar.
	if !zipHasPrefix(&zr.Reader, "data/minecraft/") {
		inner, err := findBundledInnerJar(&zr.Reader)
		if err != nil {
			return err
		}
		data, err := readZipFile(inner)
		if err != nil {
			return err
		}
		br := bytes.NewReader(data)
		innerZR, err := zip.NewReader(br, int64(len(data)))
		if err != nil {
			return err
		}
		return extractWorldgenFromZipReader(innerZR, outDir)
	}
	return extractWorldgenFromZipReader(&zr.Reader, outDir)
}

func zipHasPrefix(zr *zip.Reader, prefix string) bool {
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, prefix) {
			return true
		}
	}
	return false
}

func findBundledInnerJar(zr *zip.Reader) (*zip.File, error) {
	var inner *zip.File
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "META-INF/versions/") && strings.HasSuffix(f.Name, ".jar") {
			if inner == nil || f.UncompressedSize64 > inner.UncompressedSize64 {
				inner = f
			}
		}
	}
	if inner == nil {
		return nil, fmt.Errorf("no inner server jar found in bundled jar")
	}
	return inner, nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func extractWorldgenFromZipReader(zr *zip.Reader, outDir string) error {
	prefixes := []string{
		"data/minecraft/worldgen/density_function/",
		"data/minecraft/worldgen/noise/",
		"data/minecraft/worldgen/noise_settings/",
		"data/minecraft/worldgen/world_preset/",
		"data/minecraft/structure/",
	}

	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		for _, p := range prefixes {
			if !strings.HasPrefix(f.Name, p) {
				continue
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(f.Name, "data/minecraft/"), "worldgen/")
			if strings.HasPrefix(rel, "structure/") {
				rel = "structures/" + strings.TrimPrefix(rel, "structure/")
			}
			dst := filepath.Join(outDir, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			data, err := readZipFile(f)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return err
			}
			break
		}
	}
	return nil
}
