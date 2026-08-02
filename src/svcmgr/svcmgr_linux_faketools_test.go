//go:build linux

package svcmgr

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFakeBin writes an executable shell script named `name` into `dir` and
// returns nothing — the caller is expected to have already put `dir` first
// on PATH via prependPATH. This lets us exercise the systemd/runit branches
// of detectInitSystem/checkStatus/disable/startSvc/stopSvc/restartSvc/
// reloadSvc deterministically, without depending on the host actually
// having systemd/runit installed (the Docker test container has neither).
func writeFakeBin(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
}

// prependPATH puts dir first on PATH for the duration of the test, so
// exec.LookPath finds our fake binaries before any real ones. t.Setenv
// restores the original PATH automatically on test cleanup.
func prependPATH(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

const fakeSystemctlOK = `#!/bin/sh
case "$1" in
  --version) echo "systemd 255"; exit 0 ;;
  is-active) echo "active"; exit 0 ;;
  *) exit 0 ;;
esac
`

const fakeSystemctlBrokenVersion = `#!/bin/sh
exit 1
`

const fakeSystemctlFailIsActive = `#!/bin/sh
case "$1" in
  --version) echo "systemd 255"; exit 0 ;;
  is-active) exit 1 ;;
  *) exit 0 ;;
esac
`

const fakeSystemctlFailDisable = `#!/bin/sh
case "$1" in
  --version) echo "systemd 255"; exit 0 ;;
  is-active) echo "active"; exit 0 ;;
  disable) exit 1 ;;
  *) exit 0 ;;
esac
`

const fakeSvOK = `#!/bin/sh
case "$1" in
  status) echo "run: caslink"; exit 0 ;;
  *) exit 0 ;;
esac
`

// TestDetectInitSystem_FakeSystemd proves detectInitSystem() returns
// "systemd" when a working systemctl is first on PATH — the only realistic
// way to exercise this branch in a container that has no real systemd.
func TestDetectInitSystem_FakeSystemd(t *testing.T) {
	dir := t.TempDir()
	writeFakeBin(t, dir, "systemctl", fakeSystemctlOK)
	prependPATH(t, dir)

	if got := detectInitSystem(); got != "systemd" {
		t.Errorf("detectInitSystem() = %q, want %q", got, "systemd")
	}
}

// TestDetectInitSystem_BrokenSystemctlFallsThrough proves that a systemctl
// binary which exists but errors on --version does NOT get classified as
// "systemd" (the `err == nil && len(out) > 0` guard), and — since nothing
// else on PATH looks like openrc/runit/sysvinit in this container — falls
// all the way through to "unknown".
func TestDetectInitSystem_BrokenSystemctlFallsThrough(t *testing.T) {
	dir := t.TempDir()
	writeFakeBin(t, dir, "systemctl", fakeSystemctlBrokenVersion)
	prependPATH(t, dir)

	if got := detectInitSystem(); got == "systemd" {
		t.Errorf("detectInitSystem() = %q with broken systemctl, want anything but %q", got, "systemd")
	}
}

// TestDetectInitSystem_FakeRunit proves detectInitSystem() returns "runit"
// when `sv` is on PATH and no systemctl/openrc/init.d is present.
func TestDetectInitSystem_FakeRunit(t *testing.T) {
	dir := t.TempDir()
	writeFakeBin(t, dir, "sv", fakeSvOK)
	prependPATH(t, dir)

	if got := detectInitSystem(); got != "runit" {
		t.Errorf("detectInitSystem() = %q, want %q", got, "runit")
	}
}

// TestCheckStatus_Systemd_Active exercises the "systemd" case of checkStatus
// when systemctl is-active succeeds, verifying the trimmed stdout is
// returned verbatim.
func TestCheckStatus_Systemd_Active(t *testing.T) {
	dir := t.TempDir()
	writeFakeBin(t, dir, "systemctl", fakeSystemctlOK)
	prependPATH(t, dir)

	if got := checkStatus("caslink"); got != "active" {
		t.Errorf("checkStatus() = %q, want %q", got, "active")
	}
}

// TestCheckStatus_Systemd_IsActiveFails exercises the "systemd" case of
// checkStatus when systemctl is-active errors (e.g. unit not found): the
// function must fall through to the default "unknown" return rather than
// panicking or returning stale/fabricated data.
func TestCheckStatus_Systemd_IsActiveFails(t *testing.T) {
	dir := t.TempDir()
	writeFakeBin(t, dir, "systemctl", fakeSystemctlFailIsActive)
	prependPATH(t, dir)

	if got := checkStatus("caslink"); got != "unknown" {
		t.Errorf("checkStatus() = %q, want %q", got, "unknown")
	}
}

// TestCheckStatus_Runit exercises the "runit" case of checkStatus.
func TestCheckStatus_Runit(t *testing.T) {
	dir := t.TempDir()
	writeFakeBin(t, dir, "sv", fakeSvOK)
	prependPATH(t, dir)

	if got := checkStatus("caslink"); got != "run: caslink" {
		t.Errorf("checkStatus() = %q, want %q", got, "run: caslink")
	}
}

// TestDisable_Systemd_Success exercises disable()'s "systemd" case end to
// end (stop then disable) when both commands succeed.
func TestDisable_Systemd_Success(t *testing.T) {
	dir := t.TempDir()
	writeFakeBin(t, dir, "systemctl", fakeSystemctlOK)
	prependPATH(t, dir)

	if err := disable("caslink"); err != nil {
		t.Errorf("disable() = %v, want nil", err)
	}
}

// TestDisable_Systemd_DisableFails proves disable() surfaces the error from
// `systemctl disable` even though the preceding `systemctl stop` succeeded —
// stop's error is intentionally swallowed (best-effort) but disable's is not.
func TestDisable_Systemd_DisableFails(t *testing.T) {
	dir := t.TempDir()
	writeFakeBin(t, dir, "systemctl", fakeSystemctlFailDisable)
	prependPATH(t, dir)

	if err := disable("caslink"); err == nil {
		t.Error("disable() = nil, want error when systemctl disable fails")
	}
}

// TestDisable_Runit_Success exercises disable()'s "runit" case (sv down).
func TestDisable_Runit_Success(t *testing.T) {
	dir := t.TempDir()
	writeFakeBin(t, dir, "sv", fakeSvOK)
	prependPATH(t, dir)

	if err := disable("caslink"); err != nil {
		t.Errorf("disable() = %v, want nil", err)
	}
}

// TestStartStopRestartReload_Systemd_Success exercises the "systemd" case of
// startSvc/stopSvc/restartSvc/reloadSvc when the underlying systemctl calls
// all succeed.
func TestStartStopRestartReload_Systemd_Success(t *testing.T) {
	dir := t.TempDir()
	writeFakeBin(t, dir, "systemctl", fakeSystemctlOK)
	prependPATH(t, dir)

	if err := startSvc("caslink"); err != nil {
		t.Errorf("startSvc() = %v, want nil", err)
	}
	if err := stopSvc("caslink"); err != nil {
		t.Errorf("stopSvc() = %v, want nil", err)
	}
	if err := restartSvc("caslink"); err != nil {
		t.Errorf("restartSvc() = %v, want nil", err)
	}
	if err := reloadSvc("caslink"); err != nil {
		t.Errorf("reloadSvc() = %v, want nil", err)
	}
}

// TestRestartReload_Runit_Success exercises restartSvc's "runit" case (sv
// down then sv up) and, transitively via reloadSvc's default fallback
// (no dedicated "runit" case, so it calls restartSvc), the same code path
// again through reloadSvc.
func TestRestartReload_Runit_Success(t *testing.T) {
	dir := t.TempDir()
	writeFakeBin(t, dir, "sv", fakeSvOK)
	prependPATH(t, dir)

	if err := restartSvc("caslink"); err != nil {
		t.Errorf("restartSvc() = %v, want nil", err)
	}
	if err := reloadSvc("caslink"); err != nil {
		t.Errorf("reloadSvc() = %v, want nil", err)
	}
}

// TestEscalateIfNeeded_AlreadyRoot exercises the very first guard in
// escalateIfNeeded: when the calling process is already root, it must
// return nil immediately without ever invoking sudo/pkexec or re-executing
// the binary. This is only meaningful (and only safe to assert) when the
// test process actually is root, which is the normal case for the
// casjaysdev/go Docker test image; skip otherwise since a non-root run
// would fall into the sudo/pkexec/os.Exit(0) path, which cannot be tested
// without terminating the test binary.
func TestEscalateIfNeeded_AlreadyRoot(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("test process is not root; escalateIfNeeded's root-guard path is not reachable here " +
			"(the non-root path would exec sudo/pkexec and os.Exit(0), which cannot be safely tested)")
	}

	if err := escalateIfNeeded(); err != nil {
		t.Errorf("escalateIfNeeded() as root = %v, want nil", err)
	}
}

// TestUninstall_ConfirmedRemovesDirsAndPidFile feeds "y" to the interactive
// [y/N] prompt and verifies uninstall() actually removes the passed-in
// directories and PID file. This is safe to run unconditionally (unlike the
// systemd unit file removal or ensureServiceUser/install, which mutate real
// system paths) because: (1) we deliberately leave PATH untouched by any
// other test in this file so detectInitSystem() resolves to "unknown" in
// this container, meaning uninstall()'s systemd-unit-removal switch has no
// matching case and never touches /etc/systemd/system; and (2)
// stopSvc/disable are called with a service name that cannot exist, and
// since no init system is detected they short-circuit to an ignored error
// without invoking any real subprocess that could mutate host state.
func TestUninstall_ConfirmedRemovesDirsAndPidFile(t *testing.T) {
	if got := detectInitSystem(); got != "unknown" {
		t.Skipf("detectInitSystem() = %q, want %q for this test to be safe; "+
			"container environment changed since this test was written", got, "unknown")
	}

	base := t.TempDir()
	configDir := filepath.Join(base, "config")
	dataDir := filepath.Join(base, "data")
	pidFile := filepath.Join(base, "caslink.pid")

	for _, dir := range []string{configDir, dataDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed marker in %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(pidFile, []byte("123"), 0o600); err != nil {
		t.Fatalf("seed pid file: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.Write([]byte("y\n"))
		_ = w.Close()
	}()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = outW
	defer func() { os.Stdout = origStdout }()

	m := &Manager{ServiceName: "caslink-test-definitely-nonexistent-svc-zzz", BinaryPath: "/usr/local/bin/caslink"}
	uerr := m.Uninstall(configDir, dataDir, "", "", "", pidFile)
	_ = outW.Close()
	buf := make([]byte, 4096)
	n, _ := outR.Read(buf)
	_ = n

	if uerr != nil {
		t.Errorf("Uninstall() with confirmed deletion = %v, want nil", uerr)
	}
	if _, statErr := os.Stat(configDir); !os.IsNotExist(statErr) {
		t.Errorf("configDir %s still exists after confirmed Uninstall()", configDir)
	}
	if _, statErr := os.Stat(dataDir); !os.IsNotExist(statErr) {
		t.Errorf("dataDir %s still exists after confirmed Uninstall()", dataDir)
	}
	if _, statErr := os.Stat(pidFile); !os.IsNotExist(statErr) {
		t.Errorf("pidFile %s still exists after confirmed Uninstall()", pidFile)
	}
}
