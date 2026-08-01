package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// withConfigHome points XDG_CONFIG_HOME (or APPDATA on Windows) at a fresh
// temp directory for the duration of the test, restoring the previous value
// on cleanup. Uses t.TempDir() rather than a bare /tmp path per repo
// convention for test-scoped temp dirs.
func withConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	envVar := "XDG_CONFIG_HOME"
	if runtime.GOOS == "windows" {
		envVar = "APPDATA"
	}
	old, had := os.LookupEnv(envVar)
	if err := os.Setenv(envVar, dir); err != nil {
		t.Fatalf("setenv %s: %v", envVar, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(envVar, old)
		} else {
			_ = os.Unsetenv(envVar)
		}
	})
	return dir
}

func TestGetConfigDir(t *testing.T) {
	dir := withConfigHome(t)

	got, err := GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir() error = %v", err)
	}

	want := filepath.Join(dir, "webappsgo", "caslink")
	if runtime.GOOS == "windows" {
		want = filepath.Join(dir, "webappsgo", "caslink")
	}
	if got != want {
		t.Errorf("GetConfigDir() = %q, want %q", got, want)
	}
}

func TestGetConfigDir_MissingHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("APPDATA-based lookup tested separately on windows")
	}

	// No XDG_CONFIG_HOME and no HOME should fail cleanly rather than panic.
	oldXDG, hadXDG := os.LookupEnv("XDG_CONFIG_HOME")
	oldHome, hadHome := os.LookupEnv("HOME")
	_ = os.Unsetenv("XDG_CONFIG_HOME")
	_ = os.Unsetenv("HOME")
	t.Cleanup(func() {
		if hadXDG {
			_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
		}
		if hadHome {
			_ = os.Setenv("HOME", oldHome)
		}
	})

	_, err := GetConfigDir()
	if err == nil {
		t.Error("GetConfigDir() with no XDG_CONFIG_HOME/HOME expected an error, got nil")
	}
}

func TestGetTokenFile(t *testing.T) {
	dir := withConfigHome(t)

	got, err := GetTokenFile()
	if err != nil {
		t.Fatalf("GetTokenFile() error = %v", err)
	}
	want := filepath.Join(dir, "webappsgo", "caslink", "token")
	if got != want {
		t.Errorf("GetTokenFile() = %q, want %q", got, want)
	}
}

func TestLoadCLIConfig_MissingFileReturnsDefaults(t *testing.T) {
	withConfigHome(t)

	cfg, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() error = %v", err)
	}
	if cfg.Display.Mode != "auto" {
		t.Errorf("Display.Mode = %q, want %q", cfg.Display.Mode, "auto")
	}
	if cfg.Update.Auto != false {
		t.Errorf("Update.Auto = %v, want false", cfg.Update.Auto)
	}
	if cfg.Update.Channel != "stable" {
		t.Errorf("Update.Channel = %q, want %q", cfg.Update.Channel, "stable")
	}
	if cfg.Lang != "en" {
		t.Errorf("Lang = %q, want %q", cfg.Lang, "en")
	}
	if cfg.Color != "auto" {
		t.Errorf("Color = %q, want %q", cfg.Color, "auto")
	}
	if cfg.Server != "" {
		t.Errorf("Server = %q, want empty", cfg.Server)
	}
}

func TestSaveThenLoadCLIConfig_RoundTrip(t *testing.T) {
	withConfigHome(t)

	cfg := &CLIConfig{
		Server: "https://link.example.com",
		Token:  "adm_supersecret",
		Lang:   "fr",
		Color:  "yes",
	}
	cfg.Display.Mode = "cli"
	cfg.Update.Auto = true
	cfg.Update.Channel = "beta"
	cfg.Cluster = []string{"https://a.example.com", "https://b.example.com"}
	cfg.ClusterRefreshedAt = 123456

	if err := SaveCLIConfig(cfg); err != nil {
		t.Fatalf("SaveCLIConfig() error = %v", err)
	}

	got, err := LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() error = %v", err)
	}

	if got.Server != cfg.Server {
		t.Errorf("Server = %q, want %q", got.Server, cfg.Server)
	}
	if got.Token != cfg.Token {
		t.Errorf("Token = %q, want %q", got.Token, cfg.Token)
	}
	if got.Lang != cfg.Lang {
		t.Errorf("Lang = %q, want %q", got.Lang, cfg.Lang)
	}
	if got.Color != cfg.Color {
		t.Errorf("Color = %q, want %q", got.Color, cfg.Color)
	}
	if got.Display.Mode != cfg.Display.Mode {
		t.Errorf("Display.Mode = %q, want %q", got.Display.Mode, cfg.Display.Mode)
	}
	if got.Update.Auto != cfg.Update.Auto {
		t.Errorf("Update.Auto = %v, want %v", got.Update.Auto, cfg.Update.Auto)
	}
	if got.Update.Channel != cfg.Update.Channel {
		t.Errorf("Update.Channel = %q, want %q", got.Update.Channel, cfg.Update.Channel)
	}
	if len(got.Cluster) != 2 || got.Cluster[0] != cfg.Cluster[0] || got.Cluster[1] != cfg.Cluster[1] {
		t.Errorf("Cluster = %v, want %v", got.Cluster, cfg.Cluster)
	}
	if got.ClusterRefreshedAt != cfg.ClusterRefreshedAt {
		t.Errorf("ClusterRefreshedAt = %d, want %d", got.ClusterRefreshedAt, cfg.ClusterRefreshedAt)
	}
}

func TestSaveCLIConfig_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits not applicable on windows")
	}
	dir := withConfigHome(t)

	cfg := &CLIConfig{Server: "https://link.example.com"}
	if err := SaveCLIConfig(cfg); err != nil {
		t.Fatalf("SaveCLIConfig() error = %v", err)
	}

	path := filepath.Join(dir, "webappsgo", "caslink", "cli.yml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("cli.yml perm = %04o, want 0600", perm)
	}
}

func TestLoadCLIConfig_RejectsInsecurePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits not applicable on windows")
	}
	dir := withConfigHome(t)

	cfgDir := filepath.Join(dir, "webappsgo", "caslink")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(cfgDir, "cli.yml")
	if err := os.WriteFile(path, []byte("server: https://link.example.com\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadCLIConfig()
	if err == nil {
		t.Fatal("LoadCLIConfig() with world-readable file expected an error, got nil")
	}
}

func TestLoadCLIConfig_RejectsMalformedYAML(t *testing.T) {
	dir := withConfigHome(t)

	cfgDir := filepath.Join(dir, "webappsgo", "caslink")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(cfgDir, "cli.yml")
	if err := os.WriteFile(path, []byte("server: [this is not valid: yaml"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadCLIConfig()
	if err == nil {
		t.Fatal("LoadCLIConfig() with malformed YAML expected an error, got nil")
	}
}

func TestSaveCLIConfig_CreatesConfigDir(t *testing.T) {
	dir := withConfigHome(t)

	cfgDir := filepath.Join(dir, "webappsgo", "caslink")
	if _, err := os.Stat(cfgDir); !os.IsNotExist(err) {
		t.Fatalf("expected config dir to not exist yet, stat err = %v", err)
	}

	if err := SaveCLIConfig(&CLIConfig{Server: "https://link.example.com"}); err != nil {
		t.Fatalf("SaveCLIConfig() error = %v", err)
	}

	info, err := os.Stat(cfgDir)
	if err != nil {
		t.Fatalf("expected config dir to exist: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", cfgDir)
	}
}

func TestSaveCLIConfig_NoLeftoverTempFile(t *testing.T) {
	dir := withConfigHome(t)

	if err := SaveCLIConfig(&CLIConfig{Server: "https://link.example.com"}); err != nil {
		t.Fatalf("SaveCLIConfig() error = %v", err)
	}

	tmp := filepath.Join(dir, "webappsgo", "caslink", "cli.yml.tmp")
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("expected temp file %s to be renamed away, stat err = %v", tmp, err)
	}
}
