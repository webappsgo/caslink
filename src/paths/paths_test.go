package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetUnixPathsRoot verifies the system-wide (privileged) path layout per
// AI.md PART 2/4: config under /etc, data under /var/lib, cache under
// /var/cache, backups nested under data, logs under /var/log, and a
// dedicated PID path under /var/run.
func TestGetUnixPathsRoot(t *testing.T) {
	p := getUnixPaths("webappsgo", "caslink", true)

	want := &Paths{
		Config: "/etc/webappsgo/caslink",
		Data:   "/var/lib/webappsgo/caslink",
		Cache:  "/var/cache/webappsgo/caslink",
		Backup: "/var/lib/webappsgo/caslink/backups",
		Log:    "/var/log/webappsgo/caslink",
		PID:    "/var/run/webappsgo/caslink.pid",
	}

	assertPathsEqual(t, p, want)
}

// TestGetUnixPathsUserDefaults verifies XDG Base Directory defaults are used
// when none of XDG_CONFIG_HOME/XDG_DATA_HOME/XDG_CACHE_HOME are set.
func TestGetUnixPathsUserDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")

	p := getUnixPaths("webappsgo", "caslink", false)

	base := filepath.Join(home, ".local", "share", "webappsgo", "caslink")
	want := &Paths{
		Config: filepath.Join(home, ".config", "webappsgo", "caslink"),
		Data:   base,
		Cache:  filepath.Join(home, ".cache", "webappsgo", "caslink"),
		Backup: filepath.Join(base, "backups"),
		Log:    filepath.Join(base, "logs"),
		PID:    filepath.Join(base, "caslink.pid"),
	}

	assertPathsEqual(t, p, want)
}

// TestGetUnixPathsUserRespectsXDGOverrides verifies explicit
// XDG_CONFIG_HOME/XDG_DATA_HOME/XDG_CACHE_HOME environment variables take
// priority over the ~/.config, ~/.local/share, ~/.cache defaults.
func TestGetUnixPathsUserRespectsXDGOverrides(t *testing.T) {
	home := t.TempDir()
	xdgConfig := filepath.Join(home, "custom-config")
	xdgData := filepath.Join(home, "custom-data")
	xdgCache := filepath.Join(home, "custom-cache")

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("XDG_DATA_HOME", xdgData)
	t.Setenv("XDG_CACHE_HOME", xdgCache)

	p := getUnixPaths("webappsgo", "caslink", false)

	base := filepath.Join(xdgData, "webappsgo", "caslink")
	if p.Config != filepath.Join(xdgConfig, "webappsgo", "caslink") {
		t.Errorf("Config = %q, want XDG_CONFIG_HOME-based path", p.Config)
	}
	if p.Data != base {
		t.Errorf("Data = %q, want %q", p.Data, base)
	}
	if p.Cache != filepath.Join(xdgCache, "webappsgo", "caslink") {
		t.Errorf("Cache = %q, want XDG_CACHE_HOME-based path", p.Cache)
	}
}

// TestGetDarwinPaths verifies both privileged and user macOS layouts.
func TestGetDarwinPaths(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		p := getDarwinPaths("webappsgo", "caslink", true)
		base := "/Library/Application Support/webappsgo/caslink"
		want := &Paths{
			Config: base,
			Data:   base,
			Cache:  "/Library/Caches/webappsgo/caslink",
			Backup: filepath.Join(base, "backups"),
			Log:    "/Library/Logs/webappsgo/caslink",
			PID:    "/var/run/webappsgo/caslink.pid",
		}
		assertPathsEqual(t, p, want)
	})

	t.Run("user", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		p := getDarwinPaths("webappsgo", "caslink", false)
		base := filepath.Join(home, "Library/Application Support", "webappsgo", "caslink")
		want := &Paths{
			Config: base,
			Data:   base,
			Cache:  filepath.Join(home, "Library/Caches", "webappsgo", "caslink"),
			Backup: filepath.Join(base, "backups"),
			Log:    filepath.Join(home, "Library/Logs", "webappsgo", "caslink"),
			PID:    filepath.Join(base, "caslink.pid"),
		}
		assertPathsEqual(t, p, want)
	})
}

// TestGetWindowsPaths verifies both privileged (ProgramData) and user
// (%APPDATA%/%LOCALAPPDATA%) Windows layouts.
func TestGetWindowsPaths(t *testing.T) {
	t.Run("root uses ProgramData", func(t *testing.T) {
		programData := filepath.Join(t.TempDir(), "ProgramData")
		t.Setenv("ProgramData", programData)

		p := getWindowsPaths("webappsgo", "caslink", true)
		base := filepath.Join(programData, "webappsgo", "caslink")
		want := &Paths{
			Config: base,
			Data:   base,
			Cache:  filepath.Join(base, "cache"),
			Backup: filepath.Join(base, "backups"),
			Log:    filepath.Join(base, "logs"),
			PID:    filepath.Join(base, "caslink.pid"),
		}
		assertPathsEqual(t, p, want)
	})

	t.Run("root falls back when ProgramData unset", func(t *testing.T) {
		t.Setenv("ProgramData", "")
		p := getWindowsPaths("webappsgo", "caslink", true)
		want := filepath.Join("C:\\ProgramData", "webappsgo", "caslink")
		if p.Config != want {
			t.Errorf("Config = %q, want %q", p.Config, want)
		}
	})

	t.Run("user uses APPDATA/LOCALAPPDATA", func(t *testing.T) {
		appData := filepath.Join(t.TempDir(), "Roaming")
		localAppData := filepath.Join(t.TempDir(), "Local")
		t.Setenv("APPDATA", appData)
		t.Setenv("LOCALAPPDATA", localAppData)

		p := getWindowsPaths("webappsgo", "caslink", false)
		base := filepath.Join(appData, "webappsgo", "caslink")
		want := &Paths{
			Config: base,
			Data:   base,
			Cache:  filepath.Join(localAppData, "webappsgo", "caslink", "cache"),
			Backup: filepath.Join(base, "backups"),
			Log:    filepath.Join(base, "logs"),
			PID:    filepath.Join(base, "caslink.pid"),
		}
		assertPathsEqual(t, p, want)
	})
}

// TestIsRunningAsRoot cross-checks against os.Geteuid on non-Windows, which
// is what the actual test runner's GOOS resolves to in this repo's Docker
// toolchain.
func TestIsRunningAsRoot(t *testing.T) {
	want := os.Geteuid() == 0
	if got := isRunningAsRoot(); got != want {
		t.Errorf("isRunningAsRoot() = %v, want %v (os.Geteuid()==0)", got, want)
	}
}

// TestGetHomeDir verifies the $HOME environment variable takes priority.
func TestGetHomeDir(t *testing.T) {
	custom := "/custom/home/dir"
	t.Setenv("HOME", custom)
	if got := getHomeDir(); got != custom {
		t.Errorf("getHomeDir() = %q, want %q", got, custom)
	}
}

// TestGetHomeDirFallsBackWhenHOMEUnset verifies a non-empty value is still
// produced (via os/user.Current() or the /tmp last resort) when $HOME is
// empty, so downstream path construction never operates on an empty base.
func TestGetHomeDirFallsBackWhenHOMEUnset(t *testing.T) {
	t.Setenv("HOME", "")
	if got := getHomeDir(); got == "" {
		t.Errorf("getHomeDir() = %q, want a non-empty fallback", got)
	}
}

// TestGetEnvOrDefault covers both the set and unset/empty branches.
func TestGetEnvOrDefault(t *testing.T) {
	t.Run("set value wins", func(t *testing.T) {
		t.Setenv("CASLINK_TEST_VAR", "explicit-value")
		if got := getEnvOrDefault("CASLINK_TEST_VAR", "fallback"); got != "explicit-value" {
			t.Errorf("getEnvOrDefault() = %q, want %q", got, "explicit-value")
		}
	})

	t.Run("unset uses default", func(t *testing.T) {
		t.Setenv("CASLINK_TEST_VAR", "")
		if got := getEnvOrDefault("CASLINK_TEST_VAR", "fallback"); got != "fallback" {
			t.Errorf("getEnvOrDefault() = %q, want %q", got, "fallback")
		}
	})
}

// TestExpandPath covers tilde expansion (bare "~", "~/sub"), environment
// variable expansion, and plain paths left untouched.
func TestExpandPath(t *testing.T) {
	home := "/home/testuser"
	t.Setenv("HOME", home)
	t.Setenv("CASLINK_TEST_EXPAND", "expanded")

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare tilde", "~", home},
		{"tilde with subpath", "~/config/server.yml", filepath.Join(home, "config/server.yml")},
		{"env var expansion", "/data/$CASLINK_TEST_EXPAND/dir", "/data/expanded/dir"},
		{"plain path untouched", "/etc/webappsgo/caslink", "/etc/webappsgo/caslink"},
		{"empty path", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpandPath(tt.in); got != tt.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestEnsureDirCreatesAndIsIdempotent verifies EnsureDir creates a directory
// that is writable, and that calling it again on an already-existing
// directory does not error (called on every startup, per PART 7).
func TestEnsureDirCreatesAndIsIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "sub", "dir")

	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir (create): %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", dir, err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", dir)
	}

	if err := EnsureDir(dir); err != nil {
		t.Errorf("EnsureDir (idempotent second call): %v", err)
	}
}

// TestEnsureDirFailsWhenPathComponentIsAFile verifies EnsureDir surfaces the
// underlying error rather than silently succeeding when a path component
// that must be a directory is actually a regular file.
func TestEnsureDirFailsWhenPathComponentIsAFile(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := EnsureDir(filepath.Join(blocker, "subdir")); err == nil {
		t.Errorf("EnsureDir() under a file path component: expected an error, got nil")
	}
}

// TestEnsurePIDFileCreatesParentDir verifies EnsurePIDFile creates the
// directory portion of a PID file path, not the PID file itself.
func TestEnsurePIDFileCreatesParentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rundir")
	pidPath := filepath.Join(dir, "caslink.pid")

	if err := EnsurePIDFile(pidPath); err != nil {
		t.Fatalf("EnsurePIDFile: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected parent dir %s to exist: %v", dir, err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", dir)
	}
	if _, err := os.Stat(pidPath); err == nil {
		t.Errorf("EnsurePIDFile should not create the PID file itself, but %s exists", pidPath)
	}
}

// TestGetDefaultPaths is a smoke test for the OS dispatch switch — the
// per-OS/per-privilege logic is covered exhaustively above via the
// unexported getUnixPaths/getDarwinPaths/getWindowsPaths helpers, which take
// isRoot explicitly and so are deterministic regardless of how the test
// binary itself is invoked.
func TestGetDefaultPaths(t *testing.T) {
	p := GetDefaultPaths("webappsgo", "caslink")
	if p == nil {
		t.Fatal("GetDefaultPaths() returned nil")
	}
	if p.Config == "" || p.Data == "" || p.Cache == "" || p.Backup == "" || p.Log == "" || p.PID == "" {
		t.Errorf("GetDefaultPaths() has an empty field: %+v", p)
	}
}

func assertPathsEqual(t *testing.T, got, want *Paths) {
	t.Helper()
	if got.Config != want.Config {
		t.Errorf("Config = %q, want %q", got.Config, want.Config)
	}
	if got.Data != want.Data {
		t.Errorf("Data = %q, want %q", got.Data, want.Data)
	}
	if got.Cache != want.Cache {
		t.Errorf("Cache = %q, want %q", got.Cache, want.Cache)
	}
	if got.Backup != want.Backup {
		t.Errorf("Backup = %q, want %q", got.Backup, want.Backup)
	}
	if got.Log != want.Log {
		t.Errorf("Log = %q, want %q", got.Log, want.Log)
	}
	if got.PID != want.PID {
		t.Errorf("PID = %q, want %q", got.PID, want.PID)
	}
}
