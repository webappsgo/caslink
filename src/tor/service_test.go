package tor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/caslink/src/config"
)

// TestFindTorBinaryConfiguredExists verifies that an explicitly configured
// path wins immediately when it exists on disk, without touching PATH or the
// hardcoded candidate list.
func TestFindTorBinaryConfiguredExists(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "tor-fake")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	if got := findTorBinary(fake); got != fake {
		t.Errorf("findTorBinary(%q) = %q, want %q", fake, got, fake)
	}
}

// TestFindTorBinaryConfiguredMissingFallsBackToPath verifies that a
// configured path which does not exist is ignored, and PATH is consulted
// next by isolating $PATH to a directory containing a fake "tor" executable.
func TestFindTorBinaryConfiguredMissingFallsBackToPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH-based executable lookup semantics differ on windows")
	}

	dir := t.TempDir()
	fake := filepath.Join(dir, "tor")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	t.Setenv("PATH", dir)

	got := findTorBinary(filepath.Join(dir, "does-not-exist"))
	if got != fake {
		t.Errorf("findTorBinary() = %q, want %q (from PATH)", got, fake)
	}
}

// TestFindTorBinaryNotFound verifies that when neither the configured path
// nor PATH resolves a tor binary, and none of the hardcoded OS candidate
// paths exist on this machine, an empty string is returned rather than a
// panic or a guessed path.
func TestFindTorBinaryNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardcoded candidate paths are OS-specific; this test targets unix defaults")
	}

	for _, c := range []string{"/usr/bin/tor", "/usr/local/bin/tor"} {
		if _, err := os.Stat(c); err == nil {
			t.Skipf("skipping: %s exists on this system, would make the test non-deterministic", c)
		}
	}

	dir := t.TempDir()
	t.Setenv("PATH", dir)

	if got := findTorBinary(""); got != "" {
		t.Errorf("findTorBinary() = %q, want empty string", got)
	}
}

// TestEnsureTorDirsCreatesAllSubdirs verifies all three required directories
// (config, data/tor/data, data/tor/site) are created with 0700 permissions.
func TestEnsureTorDirsCreatesAllSubdirs(t *testing.T) {
	base := t.TempDir()
	configDir := filepath.Join(base, "config")
	dataDir := filepath.Join(base, "data")

	if err := ensureTorDirs(configDir, dataDir); err != nil {
		t.Fatalf("ensureTorDirs: %v", err)
	}

	for _, d := range []string{
		configDir,
		filepath.Join(dataDir, "tor", "data"),
		filepath.Join(dataDir, "tor", "site"),
	} {
		info, err := os.Stat(d)
		if err != nil {
			t.Fatalf("expected directory %s to exist: %v", d, err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}

// TestEnsureTorDirsIdempotent verifies calling it twice on an existing tree
// does not error (startup calls this on every Start()).
func TestEnsureTorDirsIdempotent(t *testing.T) {
	base := t.TempDir()
	configDir := filepath.Join(base, "config")
	dataDir := filepath.Join(base, "data")

	if err := ensureTorDirs(configDir, dataDir); err != nil {
		t.Fatalf("first ensureTorDirs: %v", err)
	}
	if err := ensureTorDirs(configDir, dataDir); err != nil {
		t.Fatalf("second ensureTorDirs: %v", err)
	}
}

// TestEnsureTorrcCreatesOnce verifies that ensureTorrc creates the file on
// first call (created=true) and reports created=false without overwriting
// the file on a subsequent call, even with different content.
func TestEnsureTorrcCreatesOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "torrc")

	created, err := ensureTorrc(path, []byte("first\n"))
	if err != nil {
		t.Fatalf("ensureTorrc (first): %v", err)
	}
	if !created {
		t.Errorf("ensureTorrc (first) created = false, want true")
	}

	created, err = ensureTorrc(path, []byte("second\n"))
	if err != nil {
		t.Fatalf("ensureTorrc (second): %v", err)
	}
	if created {
		t.Errorf("ensureTorrc (second) created = true, want false")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading torrc: %v", err)
	}
	if string(data) != "first\n" {
		t.Errorf("torrc content = %q, want %q (should not be overwritten)", data, "first\n")
	}
}

// TestUpdateTorrcAlwaysOverwrites verifies updateTorrc unconditionally
// replaces existing content — the counterpart to ensureTorrc's create-once
// behavior, used when config has changed between restarts.
func TestUpdateTorrcAlwaysOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "torrc")

	if err := os.WriteFile(path, []byte("old\n"), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := updateTorrc(path, []byte("new\n")); err != nil {
		t.Fatalf("updateTorrc: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading torrc: %v", err)
	}
	if string(data) != "new\n" {
		t.Errorf("torrc content = %q, want %q", data, "new\n")
	}
}

// TestGetTorConfig checks every conditional branch of torrc generation
// against a representative config, per AI.md PART 32 (dedicated non-default
// SocksPort/ControlPort, no exit relay, safe logging, bandwidth caps).
func TestGetTorConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.TorConfig
		want    []string
		mustNot []string
	}{
		{
			name: "network disabled, no bandwidth caps",
			cfg: &config.TorConfig{
				UseNetwork:  false,
				SafeLogging: true,
			},
			want: []string{
				"SocksPort 0\n",
				"ControlPort 127.0.0.1:auto\n",
				"SafeLogging 1\n",
				"ExitRelay 0\n",
				"ExitPolicy reject *:*\n",
				"ORPort 0\n",
				"DirPort 0\n",
			},
			mustNot: []string{"SocksPort auto", "BandwidthRate", "AccountingStart"},
		},
		{
			name: "network enabled, safe logging off, bandwidth set",
			cfg: &config.TorConfig{
				UseNetwork:          true,
				SafeLogging:         false,
				BandwidthRate:       "1 MB",
				BandwidthBurst:      "2 MB",
				MaxMonthlyBandwidth: "100 GB",
			},
			want: []string{
				"SocksPort auto\n",
				"SafeLogging 0\n",
				"BandwidthRate 1 MB\n",
				"BandwidthBurst 2 MB\n",
				"AccountingStart month 1 00:00\n",
				"AccountingMax 100 GB\n",
			},
			mustNot: []string{"SocksPort 0\n"},
		},
		{
			name: "unlimited bandwidth omits accounting",
			cfg: &config.TorConfig{
				MaxMonthlyBandwidth: "unlimited",
			},
			mustNot: []string{"AccountingStart", "AccountingMax"},
		},
		{
			name: "empty bandwidth strings omitted",
			cfg:  &config.TorConfig{},
			mustNot: []string{
				"BandwidthRate \n",
				"BandwidthBurst \n",
				"AccountingStart",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTorConfig(tt.cfg)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("getTorConfig() missing %q\nfull output:\n%s", w, got)
				}
			}
			for _, nw := range tt.mustNot {
				if strings.Contains(got, nw) {
					t.Errorf("getTorConfig() unexpectedly contains %q\nfull output:\n%s", nw, got)
				}
			}
		})
	}
}

// TestGetTorConfigNeverExitRelay is a regression-style check on the
// non-negotiable AI.md PART 32 rule: this app's Tor instance must never
// become an exit relay, regardless of any other config combination.
func TestGetTorConfigNeverExitRelay(t *testing.T) {
	cfg := &config.TorConfig{UseNetwork: true, MaxMonthlyBandwidth: "unlimited"}
	got := getTorConfig(cfg)
	for _, want := range []string{"ExitRelay 0\n", "ExitPolicy reject *:*\n", "ORPort 0\n", "DirPort 0\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("getTorConfig() missing mandatory non-exit-relay directive %q", want)
		}
	}
}

// TestParseDuration covers valid, invalid, and empty inputs.
func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		fallback time.Duration
		want     time.Duration
	}{
		{"valid duration", "45s", time.Minute, 45 * time.Second},
		{"valid minutes", "3m", time.Second, 3 * time.Minute},
		{"empty string uses fallback", "", 90 * time.Second, 90 * time.Second},
		{"unparseable uses fallback", "not-a-duration", 30 * time.Second, 30 * time.Second},
		{"negative sign valid", "-5s", time.Minute, -5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseDuration(tt.in, tt.fallback); got != tt.want {
				t.Errorf("parseDuration(%q, %v) = %v, want %v", tt.in, tt.fallback, got, tt.want)
			}
		})
	}
}

// TestTorManagerZeroValueBehaviorWithoutStart verifies the manager's public
// accessors are safe to call before Start() ever succeeds (e.g. Start()
// returned nil because no tor binary was found — the common CI case). We
// deliberately do not call Start()/Restart() in this package's tests since
// they require a real tor binary and OS-level process control, which is out
// of scope for pure unit tests (per task instructions).
func TestTorManagerZeroValueBehaviorWithoutStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tm := NewTorManager(ctx, 8080, &config.TorConfig{}, t.TempDir(), t.TempDir())

	if tm.IsRunning() {
		t.Errorf("IsRunning() = true before Start(), want false")
	}
	if addr := tm.OnionAddress(); addr != "" {
		t.Errorf("OnionAddress() = %q before Start(), want empty", addr)
	}
	if err := tm.Stop(); err != nil {
		t.Errorf("Stop() with no running service returned error: %v", err)
	}

	client := tm.GetHTTPClient(false)
	if client == nil {
		t.Fatal("GetHTTPClient(false) returned nil")
	}
	if client.Transport != nil {
		t.Errorf("GetHTTPClient(false) has a non-nil Transport, want default client")
	}

	// useTor=true but no dialer exists yet (Start never ran) — must still
	// fall back to a usable default client rather than panicking.
	torClient := tm.GetHTTPClient(true)
	if torClient == nil {
		t.Fatal("GetHTTPClient(true) returned nil when no tor dialer is available")
	}
}

// TestRegenerateAddressMissingKeyFileIsNotAnError verifies that removing a
// key file that was never created (fresh install, tor never started) does
// not surface an error from RegenerateAddress's os.Remove call, since
// os.IsNotExist is explicitly tolerated.
func TestRegenerateAddressMissingKeyFileIsNotAnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dataDir := t.TempDir()
	// Force findTorBinary to fail deterministically (empty PATH, bogus
	// configured binary) so Restart()->Start() no-ops immediately instead of
	// launching a real tor process and blocking on bootstrap, regardless of
	// whether the CI container happens to have a tor binary installed.
	t.Setenv("PATH", t.TempDir())
	cfg := &config.TorConfig{Binary: filepath.Join(t.TempDir(), "no-such-tor")}
	tm := NewTorManager(ctx, 8080, cfg, t.TempDir(), dataDir)

	// Start() is never called directly; RegenerateAddress calls Restart(),
	// whose Start() no-ops because no tor binary can be found. That's exactly
	// the path this test exercises: key removal succeeds (file absent) and
	// Restart's Start() no-ops cleanly.
	if _, err := tm.RegenerateAddress(); err != nil {
		t.Errorf("RegenerateAddress() with no prior key file returned error: %v", err)
	}
}
