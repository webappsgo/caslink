package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	clientcfg "github.com/webappsgo/caslink/src/client/config"
)

// captureStdout redirects os.Stdout for the duration of f and returns
// everything written to it. Reading happens concurrently in a goroutine so
// output larger than the pipe buffer cannot deadlock the writer.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w
	outCh := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outCh <- buf.String()
	}()
	f()
	os.Stdout = orig
	_ = w.Close()
	out := <-outCh
	_ = r.Close()
	return out
}

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

func TestPrintHelp(t *testing.T) {
	out := captureStdout(t, printHelp)
	for _, want := range []string{
		"Usage:", "-h, --help", "-v, --version", "--server URL", "--shell bash|zsh|fish",
		"login", "logout", "list", "create <url>", "get <code>", "delete <code>",
		"qr <code>", "stats <code>", "version",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("printHelp() output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintVersion(t *testing.T) {
	origV := Version
	defer func() { Version = origV }()
	Version = "9.9.9"

	out := captureStdout(t, printVersion)
	want := "caslink-cli 9.9.9\n"
	if out != want {
		t.Errorf("printVersion() output = %q, want %q", out, want)
	}
}

func TestPrintCompletions_Bash(t *testing.T) {
	out := captureStdout(t, func() { printCompletions("bash") })
	if !strings.Contains(out, "bash completion for") {
		t.Errorf("printCompletions(bash) missing header:\n%s", out)
	}
	if !strings.Contains(out, "login logout list create get delete qr stats version") {
		t.Errorf("printCompletions(bash) missing commands list:\n%s", out)
	}
	if !strings.Contains(out, "complete -F _") {
		t.Errorf("printCompletions(bash) missing complete directive:\n%s", out)
	}
}

func TestPrintCompletions_Zsh(t *testing.T) {
	out := captureStdout(t, func() { printCompletions("zsh") })
	if !strings.Contains(out, "zsh completion for") {
		t.Errorf("printCompletions(zsh) missing header:\n%s", out)
	}
	if !strings.Contains(out, "compdef _") {
		t.Errorf("printCompletions(zsh) missing compdef directive:\n%s", out)
	}
	if !strings.Contains(out, "login:Authenticate") {
		t.Errorf("printCompletions(zsh) missing command description:\n%s", out)
	}
}

func TestPrintCompletions_Fish(t *testing.T) {
	out := captureStdout(t, func() { printCompletions("fish") })
	if !strings.Contains(out, "__fish_use_subcommand -a login") {
		t.Errorf("printCompletions(fish) missing login subcommand:\n%s", out)
	}
	if !strings.Contains(out, "-l output") {
		t.Errorf("printCompletions(fish) missing --output flag:\n%s", out)
	}
}

// runHelperProcess re-invokes the current test binary, selecting only the
// named test, with GO_TEST_HELPER=1 set so the target test can branch into
// its "run the exit-triggering code and return" half instead of asserting.
// This is the standard idiom for testing os.Exit paths in Go, since exiting
// the actual test process would abort the whole `go test` run.
func runHelperProcess(t *testing.T, testName string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$")
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER=1")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("runHelperProcess(%s) unexpected error type: %v", testName, err)
		}
	}
	return outBuf.String(), errBuf.String(), code
}

func TestPrintCompletions_UnknownShell(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") == "1" {
		printCompletions("powershell")
		return
	}
	_, stderr, code := runHelperProcess(t, "TestPrintCompletions_UnknownShell")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Unknown shell: powershell") {
		t.Errorf("stderr = %q, want it to mention unknown shell", stderr)
	}
}

func TestHandleUpdate_UnknownSubcommand(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") == "1" {
		handleUpdate([]string{"--update", "frobnicate"})
		return
	}
	_, stderr, code := runHelperProcess(t, "TestHandleUpdate_UnknownSubcommand")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Unknown --update subcommand: frobnicate") {
		t.Errorf("stderr = %q, want it to mention the unknown subcommand", stderr)
	}
}

func TestHandleUpdate_UnknownBranch(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") == "1" {
		handleUpdate([]string{"--update", "branch", "nightly"})
		return
	}
	_, stderr, code := runHelperProcess(t, "TestHandleUpdate_UnknownBranch")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Unknown branch: nightly") {
		t.Errorf("stderr = %q, want it to mention the unknown branch", stderr)
	}
}

// TestHandleUpdate_SubcommandParsing covers the "branch" case's channel
// argument parsing without touching the network: a flag-shaped channel
// value ("--not-a-channel") must fall into the "Unknown branch" default,
// proving args[2] is read as a plain value rather than another flag.
func TestHandleUpdate_SubcommandParsing(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") == "1" {
		handleUpdate([]string{"--update", "branch", "--not-a-channel"})
		return
	}
	_, stderr, code := runHelperProcess(t, "TestHandleUpdate_SubcommandParsing")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "Unknown branch: --not-a-channel") {
		t.Errorf("stderr = %q, want it to mention the unknown branch", stderr)
	}
}
