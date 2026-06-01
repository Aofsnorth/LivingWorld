package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"livingworld/internal/infrastructure/logging"
	"livingworld/internal/version"
	"livingworld/server"

	"github.com/mattn/go-isatty"
)

var (
	Version   = "0.0.1"
	BuildDate = "2026-05-27"
)

// main is a thin entry point over the public livingworld/server API. Everything
// it does can be done from your own program by importing "livingworld/server".
func main() {
	logging.Setup()
	log.Printf("LivingWorld Server v%s (build: %s)", Version, BuildDate)

	configPath := flag.String("config", "config/config.yml", "path to YAML config")
	noTUI := flag.Bool("no-tui", false, "disable the terminal UI and use a plain console")
	debugChunks := flag.Bool("debug-chunks", false, "log chunk streaming (boundary crossings, forget/send lists) for diagnosing chunk render bugs")
	showVersion := flag.Bool("version", false, "print the version matrix and exit")
	flag.Parse()

	// --version is the user-facing, script-friendly variant of /lwversion.
	// It must short-circuit before any config load / listener bind so it
	// stays safe to run in CI smoke checks.
	if *showVersion {
		printVersionMatrix()
		return
	}

	cfg, err := server.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	cfg.Java.DebugChunks = *debugChunks

	srv := server.New(cfg)
	// Use the TUI by default on an interactive terminal; fall back to the plain
	// console when --no-tui is set or stdout is redirected (e.g. piped to a file).
	if !*noTUI && isatty.IsTerminal(os.Stdout.Fd()) {
		err = srv.RunTUI()
	} else {
		err = srv.Run()
	}
	if err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// printVersionMatrix emits the supported LWVersion list in a script-friendly
// format. Each line is "edition\tlabel\tprotocol\tbuild1,build2,..." so CI
// smoke tests can grep for the protocol numbers without parsing Markdown.
func printVersionMatrix() {
	cur, ok := version.Current()
	if !ok {
		fmt.Println("LivingWorld: no supported versions registered")
		return
	}
	fmt.Printf("LivingWorld v%s (build: %s)\n", Version, BuildDate)
	fmt.Printf("Current LWVersion: %s\n", cur.Label)
	fmt.Println("edition\tlabel\tprotocol\tbuilds")
	for _, e := range version.AllEditions {
		var proto int32
		var builds []string
		switch e {
		case version.Java:
			proto = cur.JavaProtocol
			builds = cur.JavaBuilds
		case version.Bedrock:
			proto = cur.BedrockProtocol
			builds = cur.BedrockBuilds
		}
		fmt.Printf("%s\t%s\t%d\t%s\n", e, cur.Label, proto, joinStrings(builds, ","))
	}
	for i, v := range version.Supported() {
		if i == 0 {
			continue // already shown as "current"
		}
		fmt.Printf("%s\t%s\t%d\t%s\n", version.Java, v.Label, v.JavaProtocol, joinStrings(v.JavaBuilds, ","))
		fmt.Printf("%s\t%s\t%d\t%s\n", version.Bedrock, v.Label, v.BedrockProtocol, joinStrings(v.BedrockBuilds, ","))
	}
}

// joinStrings is a tiny strings.Join replacement so the binary doesn't
// import "strings" just for this flag.
func joinStrings(s []string, sep string) string {
	if len(s) == 0 {
		return ""
	}
	out := s[0]
	for _, v := range s[1:] {
		out += sep + v
	}
	return out
}
