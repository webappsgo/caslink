package main

import (
	"os"
	"path/filepath"
	"testing"

	clientcfg "github.com/webappsgo/caslink/src/client/config"
)

func TestHasFlag(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		flags []string
		want  bool
	}{
		{"present short", []string{"-h"}, []string{"-h", "--help"}, true},
		{"present long", []string{"list", "--help"}, []string{"-h", "--help"}, true},
		{"absent", []string{"list"}, []string{"-h", "--help"}, false},
		{"empty args", nil, []string{"-h"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasFlag(tt.args, tt.flags...); got != tt.want {
				t.Errorf("hasFlag(%v, %v) = %v, want %v", tt.args, tt.flags, got, tt.want)
			}
		})
	}
}

func TestFlagValue(t *testing.T) {
	tests := []struct {
		name string
		args []string
		flag string
		want string
	}{
		{"space separated", []string{"--server", "https://x.com"}, "--server", "https://x.com"},
		{"equals separated", []string{"--server=https://x.com"}, "--server", "https://x.com"},
		{"missing", []string{"list"}, "--server", ""},
		{"flag is last token, no value", []string{"list", "--server"}, "--server", ""},
		{"empty args", nil, "--server", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := flagValue(tt.args, tt.flag); got != tt.want {
				t.Errorf("flagValue(%v, %q) = %q, want %q", tt.args, tt.flag, got, tt.want)
			}
		})
	}
}

func TestFlagIndex(t *testing.T) {
	tests := []struct {
		name string
		args []string
		flag string
		want int
	}{
		{"found", []string{"a", "--update", "yes"}, "--update", 1},
		{"not found", []string{"a", "b"}, "--update", -1},
		{"empty", nil, "--update", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := flagIndex(tt.args, tt.flag); got != tt.want {
				t.Errorf("flagIndex(%v, %q) = %d, want %d", tt.args, tt.flag, got, tt.want)
			}
		})
	}
}

func TestStripGlobalFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "removes valued flags and their values",
			args: []string{"--server", "https://x.com", "list"},
			want: []string{"list"},
		},
		{
			name: "removes equals-style valued flags",
			args: []string{"--output=json", "list"},
			want: []string{"list"},
		},
		{
			name: "removes bool flags without consuming next token",
			args: []string{"--debug", "list", "abc"},
			want: []string{"list", "abc"},
		},
		{
			name: "leaves subcommand args untouched",
			args: []string{"create", "https://example.com", "--code", "abc"},
			want: []string{"create", "https://example.com", "--code", "abc"},
		},
		{
			name: "empty input yields nil",
			args: nil,
			want: nil,
		},
		{
			name: "all flags stripped yields nil",
			args: []string{"--debug", "--server", "https://x.com"},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripGlobalFlags(tt.args)
			if len(got) != len(tt.want) {
				t.Fatalf("stripGlobalFlags(%v) = %v, want %v", tt.args, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("stripGlobalFlags(%v)[%d] = %q, want %q", tt.args, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFullVersionString(t *testing.T) {
	origV, origC, origB, origS := Version, CommitID, BuildDate, OfficialSite
	defer func() { Version, CommitID, BuildDate, OfficialSite = origV, origC, origB, origS }()

	Version = "1.2.3"
	CommitID = "unknown"
	BuildDate = "unknown"
	OfficialSite = ""
	if got, want := fullVersionString(), "caslink-cli 1.2.3"; got != want {
		t.Errorf("fullVersionString() = %q, want %q", got, want)
	}

	CommitID = "abc1234"
	BuildDate = "2026-01-01"
	OfficialSite = "https://caslink.casapps.us"
	got := fullVersionString()
	want := "caslink-cli 1.2.3 (abc1234) built 2026-01-01 — https://caslink.casapps.us"
	if got != want {
		t.Errorf("fullVersionString() = %q, want %q", got, want)
	}
}

func TestResolveToken_Priority(t *testing.T) {
	dir := t.TempDir()

	tokenFilePath := filepath.Join(dir, "tokenfile")
	if err := os.WriteFile(tokenFilePath, []byte("  from-token-file  \n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	t.Run("token-file flag wins first", func(t *testing.T) {
		cfg := &clientcfg.CLIConfig{Token: "from-config"}
		t.Setenv("CASLINK_TOKEN", "from-env")
		got := resolveToken(cfg, []string{"--token-file", tokenFilePath})
		if got != "from-token-file" {
			t.Errorf("resolveToken() = %q, want %q", got, "from-token-file")
		}
	})

	t.Run("env var wins over config", func(t *testing.T) {
		cfg := &clientcfg.CLIConfig{Token: "from-config"}
		t.Setenv("CASLINK_TOKEN", "from-env")
		got := resolveToken(cfg, nil)
		if got != "from-env" {
			t.Errorf("resolveToken() = %q, want %q", got, "from-env")
		}
	})

	t.Run("config token used when no flag or env", func(t *testing.T) {
		cfg := &clientcfg.CLIConfig{Token: "from-config"}
		t.Setenv("CASLINK_TOKEN", "")
		got := resolveToken(cfg, nil)
		if got != "from-config" {
			t.Errorf("resolveToken() = %q, want %q", got, "from-config")
		}
	})

	t.Run("empty when nothing configured", func(t *testing.T) {
		cfg := &clientcfg.CLIConfig{}
		t.Setenv("CASLINK_TOKEN", "")
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		got := resolveToken(cfg, nil)
		if got != "" {
			t.Errorf("resolveToken() = %q, want empty", got)
		}
	})

	t.Run("nonexistent token-file falls through to env", func(t *testing.T) {
		cfg := &clientcfg.CLIConfig{}
		t.Setenv("CASLINK_TOKEN", "from-env")
		got := resolveToken(cfg, []string{"--token-file", filepath.Join(dir, "does-not-exist")})
		if got != "from-env" {
			t.Errorf("resolveToken() = %q, want %q", got, "from-env")
		}
	})
}

func TestApplyFlagOverrides(t *testing.T) {
	cfg := &clientcfg.CLIConfig{Lang: "en", Color: "auto"}
	applyFlagOverrides(cfg, []string{"--lang", "fr", "--color", "no"})
	if cfg.Lang != "fr" {
		t.Errorf("Lang = %q, want fr", cfg.Lang)
	}
	if cfg.Color != "no" {
		t.Errorf("Color = %q, want no", cfg.Color)
	}
}

func TestApplyFlagOverrides_NoFlagsLeavesDefaults(t *testing.T) {
	cfg := &clientcfg.CLIConfig{Lang: "en", Color: "auto"}
	applyFlagOverrides(cfg, nil)
	if cfg.Lang != "en" {
		t.Errorf("Lang = %q, want en (unchanged)", cfg.Lang)
	}
	if cfg.Color != "auto" {
		t.Errorf("Color = %q, want auto (unchanged)", cfg.Color)
	}
}
