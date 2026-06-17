//go:build linux

package svcmgr

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func detectInitSystem() string {
	if _, err := exec.LookPath("systemctl"); err == nil {
		if out, err := exec.Command("systemctl", "--version").Output(); err == nil && len(out) > 0 {
			return "systemd"
		}
	}
	if _, err := os.Stat("/sbin/openrc-run"); err == nil {
		return "openrc"
	}
	if _, err := exec.LookPath("sv"); err == nil {
		return "runit"
	}
	if _, err := os.Stat("/etc/init.d"); err == nil {
		return "sysvinit"
	}
	return "unknown"
}

func checkStatus(name string) string {
	switch detectInitSystem() {
	case "systemd":
		out, err := exec.Command("systemctl", "is-active", name).Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "openrc":
		out, err := exec.Command("rc-service", name, "status").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "runit":
		out, err := exec.Command("sv", "status", name).Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return "unknown"
}

// systemdUnit is the service unit written by --service --install.
// User=caslink / Group=caslink per AI.md PART 24/25: the service drops
// privileges to the caslink system user after installation.
const systemdUnit = `[Unit]
Description=Caslink URL Shortener
Documentation=https://caslink.casapps.us
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=caslink
Group=caslink
ExecStart=/usr/local/bin/caslink
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/etc/casapps/caslink
ReadWritePaths=/var/lib/casapps/caslink
ReadWritePaths=/var/cache/casapps/caslink
ReadWritePaths=/var/log/casapps/caslink

[Install]
WantedBy=multi-user.target
`

// ensureServiceUser creates the caslink system user and group if they do not
// exist. Requires root. Called by install() before writing the unit file.
func ensureServiceUser() error {
	if err := exec.Command("id", "caslink").Run(); err == nil {
		return nil // user already exists
	}
	if err := exec.Command("useradd",
		"--system",
		"--no-create-home",
		"--shell", "/usr/sbin/nologin",
		"--comment", "Caslink service account",
		"caslink",
	).Run(); err != nil {
		return fmt.Errorf("useradd caslink: %w", err)
	}
	return nil
}

// escalateIfNeeded re-executes the binary with sudo when not running as root.
// Returns nil and continues when already root.
func escalateIfNeeded() error {
	if os.Getuid() == 0 {
		return nil
	}
	// Try sudo first, then pkexec.
	for _, escalator := range []string{"sudo", "pkexec"} {
		if _, err := exec.LookPath(escalator); err == nil {
			args := append([]string{"/proc/self/exe"}, os.Args[1:]...)
			cmd := exec.Command(escalator, args...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("%s: %w", escalator, err)
			}
			os.Exit(0)
		}
	}
	return fmt.Errorf("root privileges required; install sudo or pkexec, or run as root")
}

func install(m *Manager) error {
	switch detectInitSystem() {
	case "systemd":
		if err := escalateIfNeeded(); err != nil {
			return err
		}
		if err := ensureServiceUser(); err != nil {
			return fmt.Errorf("create service user: %w", err)
		}
		unitPath := "/etc/systemd/system/caslink.service"
		if err := os.WriteFile(unitPath, []byte(systemdUnit), 0644); err != nil {
			return fmt.Errorf("failed to write unit file: %w", err)
		}
		if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
			return fmt.Errorf("daemon-reload failed: %w", err)
		}
		if err := exec.Command("systemctl", "enable", "caslink").Run(); err != nil {
			return fmt.Errorf("enable failed: %w", err)
		}
		if err := exec.Command("systemctl", "start", "caslink").Run(); err != nil {
			return fmt.Errorf("start failed: %w", err)
		}
		fmt.Println("caslink service installed and started (systemd) — running as User=caslink")
		return nil
	default:
		return fmt.Errorf("unsupported init system: %s", detectInitSystem())
	}
}

func uninstall(m *Manager, configDir, dataDir, cacheDir, logDir, backupDir, pidFile string) error {
	fmt.Print("This will delete ALL data, configs, and the system service. Continue? [y/N] ")
	var answer string
	_, _ = fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("Aborted.")
		return nil
	}

	_ = stopSvc(m.ServiceName)
	_ = disable(m.ServiceName)

	switch detectInitSystem() {
	case "systemd":
		_ = os.Remove("/etc/systemd/system/caslink.service")
		_ = exec.Command("systemctl", "daemon-reload").Run()
	}

	for _, dir := range []string{configDir, dataDir, cacheDir, logDir, backupDir} {
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
	}
	if pidFile != "" {
		_ = os.Remove(pidFile)
	}

	exe := m.BinaryPath
	fmt.Printf("Service uninstalled. Delete binary manually: rm %s\n", exe)
	return nil
}

func disable(name string) error {
	switch detectInitSystem() {
	case "systemd":
		_ = exec.Command("systemctl", "stop", name).Run()
		return exec.Command("systemctl", "disable", name).Run()
	case "openrc":
		_ = exec.Command("rc-service", name, "stop").Run()
		return exec.Command("rc-update", "del", name, "default").Run()
	case "runit":
		return exec.Command("sv", "down", name).Run()
	}
	return fmt.Errorf("unsupported init system: %s", detectInitSystem())
}

func startSvc(name string) error {
	switch detectInitSystem() {
	case "systemd":
		return exec.Command("systemctl", "start", name).Run()
	case "openrc":
		return exec.Command("rc-service", name, "start").Run()
	case "runit":
		return exec.Command("sv", "up", name).Run()
	}
	return fmt.Errorf("unsupported init system: %s", detectInitSystem())
}

func stopSvc(name string) error {
	switch detectInitSystem() {
	case "systemd":
		return exec.Command("systemctl", "stop", name).Run()
	case "openrc":
		return exec.Command("rc-service", name, "stop").Run()
	case "runit":
		return exec.Command("sv", "down", name).Run()
	}
	return fmt.Errorf("unsupported init system: %s", detectInitSystem())
}

func restartSvc(name string) error {
	switch detectInitSystem() {
	case "systemd":
		return exec.Command("systemctl", "restart", name).Run()
	case "openrc":
		return exec.Command("rc-service", name, "restart").Run()
	case "runit":
		_ = exec.Command("sv", "down", name).Run()
		return exec.Command("sv", "up", name).Run()
	}
	return fmt.Errorf("unsupported init system: %s", detectInitSystem())
}

func reloadSvc(name string) error {
	switch detectInitSystem() {
	case "systemd":
		return exec.Command("systemctl", "reload", name).Run()
	case "openrc":
		return exec.Command("rc-service", name, "reload").Run()
	}
	return restartSvc(name)
}
