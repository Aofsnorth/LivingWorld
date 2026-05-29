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
	"fmt"
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
		if dryRun {
			fmt.Fprintf(os.Stderr, "  command: java --source 21 %s %s\n", filepath.Join(javaDir, "ExtractAll.java"), version)
			return filepath.Join(jsonsDir, version), nil
		}
		cmd := exec.Command("java", "--source", "21", filepath.Join(javaDir, "ExtractAll.java"), version)
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
