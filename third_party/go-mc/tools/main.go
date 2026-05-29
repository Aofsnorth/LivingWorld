// tools generates all Go source files for go-mc from MC data JSONs.
//
// Usage:
//
//	go run ./tools/ --version 1.21.11                 # extract (if needed) + generate
//	go run ./tools/ --version 1.21.11 --extract       # force re-extraction + generate
//	go run ./tools/ --version 1.21.11 --gen-only      # extract only, skip generation
//	go run ./tools/ --version 1.21.11 --dry-run       # show what would run
//	go run ./tools/ <json-dir>                        # generate from explicit directory
//
// With --version, the tool checks temp/jsons/<version>/ for cached JSONs.
// If the cache is missing (or --extract is given), it runs a Docker/Podman
// container to download the MC server jar and extract data.
//
// The go-mc root is auto-detected by walking up from the working directory.
// Output files are written to the standard locations within go-mc.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type generator struct {
	name string
	fn   func(jsonDir, goMCRoot string) error
}

var generators = []generator{
	{"packetid", genPacketID},
	{"soundid", genSoundID},
	{"item", genItem},
	{"blocks", genBlocks},
	{"entity", genEntity},
	{"component", genComponent},
	{"blockentities", genBlockEntities},
	{"registryid", genRegistryID},
	{"biome", genBiome},
	{"lang", genLang},
}

func main() {
	args := os.Args[1:]

	forceExtract := flagBool(args, "--extract")
	version := flagValue(args, "--version")
	runtime := flagValue(args, "--runtime")
	dryRun := flagBool(args, "--dry-run")
	genOnly := flagBool(args, "--gen-only")

	goMCRoot, err := detectGoMCRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tools: %v\n", err)
		os.Exit(1)
	}

	var jsonDir string

	if version != "" {
		// --version given: use cached JSONs or extract.
		jsonDir = filepath.Join(goMCRoot, "temp", "jsons", version)
		needExtract := forceExtract

		if !needExtract {
			if _, err := os.Stat(jsonDir); err != nil {
				needExtract = true
			}
		}

		if needExtract {
			// Pre-26.x versions need an unobfuscated server jar entry.
			if !isNativelyUnobfuscated(version) {
				if err := checkUnobfuscatedEntry(goMCRoot, version); err != nil {
					fmt.Fprintf(os.Stderr, "tools: %v\n", err)
					os.Exit(1)
				}
			}
			if runtime == "" {
				runtime = detectRuntime()
				if runtime == "" {
					fmt.Fprintln(os.Stderr, "tools: neither podman nor docker found in PATH. Falling back to running Java on host.")
					runtime = "host"
				}
			}

			dir, err := runExtract(goMCRoot, version, runtime, dryRun)
			if err != nil {
				fmt.Fprintf(os.Stderr, "tools: extract: %v\n", err)
				os.Exit(1)
			}
			jsonDir = dir

			if dryRun {
				return
			}
		}

		if genOnly {
			return
		}
	} else {
		// Positional arg: generate from explicit directory.
		jsonDir = positionalArg(args)
		if jsonDir == "" {
			printUsage()
			os.Exit(1)
		}

		if _, err := os.Stat(jsonDir); err != nil {
			fmt.Fprintf(os.Stderr, "tools: JSON directory not found: %s\n", jsonDir)
			os.Exit(1)
		}
	}

	jsonVersion = filepath.Base(jsonDir)
	fmt.Fprintf(os.Stderr, "tools: go-mc=%s json-dir=%s\n\n", goMCRoot, jsonDir)

	for _, g := range generators {
		start := time.Now()
		if err := g.fn(jsonDir, goMCRoot); err != nil {
			fmt.Fprintf(os.Stderr, "tools: %s: %v\n", g.name, err)
			os.Exit(1)
		}
		elapsed := time.Since(start).Round(time.Millisecond)
		fmt.Fprintf(os.Stderr, "  %s: done (%s)\n", g.name, elapsed)
	}
	fmt.Fprintf(os.Stderr, "\ntools: all %d generators done!\n", len(generators))
}

// detectGoMCRoot finds the go-mc repository root by walking up from the
// working directory looking for go.mod with the go-mc module path.
func detectGoMCRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %v", err)
	}
	for d := wd; d != "/" && d != "."; d = filepath.Dir(d) {
		data, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "module github.com/Tnze/go-mc\n") {
			return d, nil
		}
	}
	return "", fmt.Errorf("cannot find go-mc root from %s (need go.mod with module github.com/Tnze/go-mc)", wd)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  go run ./tools/ --version 1.21.11              # extract (if needed) + generate")
	fmt.Fprintln(os.Stderr, "  go run ./tools/ --version 1.21.11 --extract    # force re-extraction + generate")
	fmt.Fprintln(os.Stderr, "  go run ./tools/ --version 1.21.11 --gen-only   # extract only, skip generation")
	fmt.Fprintln(os.Stderr, "  go run ./tools/ <json-dir>                     # generate from explicit directory")
}

// positionalArg returns the first arg that doesn't look like a flag.
func positionalArg(args []string) string {
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == "--version" || a == "--runtime" {
			skip = true
			continue
		}
		if strings.HasPrefix(a, "--") {
			continue
		}
		return a
	}
	return ""
}

// isNativelyUnobfuscated returns true for versions 26.x+ which ship unobfuscated
// in the standard Mojang manifest and don't need a separate unobfuscated entry.
func isNativelyUnobfuscated(version string) bool {
	parts := strings.SplitN(version, ".", 2)
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	return major >= 26
}

// checkUnobfuscatedEntry verifies that tools/unobfuscated_versions.json has an
// entry for the given version. Pre-26.x versions need unobfuscated server jars
// for custom Java extractors to work. Returns an error with instructions if missing.
func checkUnobfuscatedEntry(goMCRoot, version string) error {
	jsonPath := filepath.Join(goMCRoot, "tools", "unobfuscated_versions.json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("unobfuscated_versions.json not found at %s\n"+
			"  See docs/dev/tools.md for how to add unobfuscated version entries", jsonPath)
	}

	var versions map[string]json.RawMessage
	if err := json.Unmarshal(data, &versions); err != nil {
		return fmt.Errorf("failed to parse %s: %v", jsonPath, err)
	}

	if _, ok := versions[version]; !ok {
		return fmt.Errorf("version %s not found in %s\n"+
			"  Pre-26.x versions need an unobfuscated server jar for custom extractors.\n"+
			"  See docs/dev/tools.md for how to find and add the unobfuscated entry", version, jsonPath)
	}
	return nil
}
