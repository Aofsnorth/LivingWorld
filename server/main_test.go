package server

import "testing"

func TestParseFlagsApply(t *testing.T) {
	f, err := ParseFlags([]string{"-java-port", "30000", "-online", "-max-players", "50"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	bedrockBefore := cfg.Bedrock.Port
	f.Apply(cfg)

	if cfg.Java.Port != 30000 {
		t.Fatalf("java.port = %d", cfg.Java.Port)
	}
	if !cfg.Java.OnlineMode {
		t.Fatal("online not applied")
	}
	if cfg.Java.MaxPlayers != 50 || cfg.Bedrock.MaxPlayers != 50 {
		t.Fatalf("max-players not applied to both: %d/%d", cfg.Java.MaxPlayers, cfg.Bedrock.MaxPlayers)
	}
	if cfg.Bedrock.Port != bedrockBefore { // unset flag must not override
		t.Fatalf("bedrock port changed unexpectedly: %d", cfg.Bedrock.Port)
	}
}

func TestParseFlagsDefaults(t *testing.T) {
	f, err := ParseFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.ConfigPath != "config/config.yml" {
		t.Fatalf("config default = %q", f.ConfigPath)
	}
	if f.Whitelist {
		t.Fatal("whitelist should default off")
	}
}
