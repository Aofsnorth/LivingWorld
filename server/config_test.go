package server

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadConfigYAML(t *testing.T) {
	p := writeTemp(t, "config.yml", "serverName: YamlSrv\njava:\n  port: 25600\n")
	cfg, err := LoadConfigFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerName != "YamlSrv" {
		t.Fatalf("serverName = %q", cfg.ServerName)
	}
	if cfg.Java.Port != 25600 {
		t.Fatalf("java.port = %d", cfg.Java.Port)
	}
	if cfg.Bedrock.Port != 19132 { // default preserved for unspecified field
		t.Fatalf("bedrock default lost: %d", cfg.Bedrock.Port)
	}
}

func TestLoadConfigTOML(t *testing.T) {
	doc := "serverName = \"TomlSrv\"\n\n[world]\ntype = \"superflat\"\nseed = 99\n\n[java]\nport = 12345\nonlineMode = true\n"
	p := writeTemp(t, "server.toml", doc)
	cfg, err := LoadConfigFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerName != "TomlSrv" {
		t.Fatalf("serverName = %q", cfg.ServerName)
	}
	if cfg.Java.Port != 12345 || !cfg.Java.OnlineMode {
		t.Fatalf("java = %+v", cfg.Java)
	}
	if cfg.World.Seed != 99 {
		t.Fatalf("world.seed = %d", cfg.World.Seed)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	cfg, err := LoadConfigFile(filepath.Join(t.TempDir(), "nope.yml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if cfg.ServerName != "LivingWorld Server" {
		t.Fatalf("expected defaults, got %q", cfg.ServerName)
	}
}
