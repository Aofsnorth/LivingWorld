package server

import (
	"flag"
	"fmt"
)

// Flags are the command-line startup options. Only flags explicitly passed
// override the loaded config (tracked in set); see Apply.
type Flags struct {
	ConfigPath    string
	OpsPath       string
	WhitelistPath string
	Whitelist     bool
	OnlineMode    bool
	JavaPort      int
	BedrockPort   int
	MaxPlayers    int
	WorldDir      string

	set map[string]bool
}

// ParseFlags parses args (typically os.Args[1:]) into Flags. It uses
// ContinueOnError, so a parse failure returns an error instead of exiting.
func ParseFlags(args []string) (*Flags, error) {
	f := &Flags{set: map[string]bool{}}
	fs := flag.NewFlagSet("livingworld", flag.ContinueOnError)
	fs.StringVar(&f.ConfigPath, "config", "config/config.yml", "config file (.yml/.yaml/.toml)")
	fs.StringVar(&f.OpsPath, "ops", "config/ops.txt", "operator list file")
	fs.StringVar(&f.WhitelistPath, "whitelist-file", "config/whitelist.txt", "whitelist file")
	fs.BoolVar(&f.Whitelist, "whitelist", false, "only admit whitelisted players")
	fs.BoolVar(&f.OnlineMode, "online", false, "enable Java online-mode authentication")
	fs.IntVar(&f.JavaPort, "java-port", 0, "override Java port")
	fs.IntVar(&f.BedrockPort, "bedrock-port", 0, "override Bedrock port")
	fs.IntVar(&f.MaxPlayers, "max-players", 0, "override max players (both editions)")
	fs.StringVar(&f.WorldDir, "world-dir", "", "override world save directory")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	fs.Visit(func(fl *flag.Flag) { f.set[fl.Name] = true })
	return f, nil
}

// Apply overrides cfg with the flags that were explicitly set on the command
// line, leaving file/default values intact for the rest.
func (f *Flags) Apply(cfg *Config) {
	if f.set["java-port"] {
		cfg.Java.Port = f.JavaPort
	}
	if f.set["bedrock-port"] {
		cfg.Bedrock.Port = f.BedrockPort
	}
	if f.set["online"] {
		cfg.Java.OnlineMode = f.OnlineMode
	}
	if f.set["max-players"] {
		cfg.Java.MaxPlayers = f.MaxPlayers
		cfg.Bedrock.MaxPlayers = f.MaxPlayers
	}
	if f.set["world-dir"] {
		cfg.World.Directory = f.WorldDir
	}
}

// Main is the server entry point: parse flags, load config + ops + whitelist,
// build the server, and run until interrupted. A binary is then a one-liner:
//
//	func main() { log.Fatal(server.Main(os.Args[1:])) }
func Main(args []string) error {
	f, err := ParseFlags(args)
	if err != nil {
		return err
	}

	cfg, err := LoadConfigFile(f.ConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	f.Apply(cfg)

	ops, err := LoadOps(f.OpsPath)
	if err != nil {
		return fmt.Errorf("load ops: %w", err)
	}
	cfg.Ops = ops.List() // feed the login op-check (config.IsOp)

	wl, err := LoadWhitelist(f.WhitelistPath)
	if err != nil {
		return fmt.Errorf("load whitelist: %w", err)
	}
	wl.SetEnabled(f.Whitelist)

	srv := New(cfg)
	srv.ops = ops
	srv.whitelist = wl
	return srv.Run()
}
