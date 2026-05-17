//go:build freebsd || openbsd || netbsd

package svcmgr

import (
	"fmt"
	"os"
	"os/exec"
)

const rcdPath = "/usr/local/etc/rc.d/caslink"

const rcdScript = `#!/bin/sh

# PROVIDE: caslink
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="caslink"
rcvar="caslink_enable"
command="/usr/local/bin/caslink"

load_rc_config $name
run_rc_command "$1"
`

func checkStatus(name string) string {
	out, err := exec.Command("service", name, "status").Output()
	if err != nil {
		return "stopped"
	}
	if len(out) > 0 {
		return "running"
	}
	return "unknown"
}

func install(m *Manager) error {
	if err := os.WriteFile(rcdPath, []byte(rcdScript), 0755); err != nil {
		return fmt.Errorf("failed to write rc.d script: %w", err)
	}
	if err := exec.Command("service", "caslink", "enable").Run(); err != nil {
		return fmt.Errorf("enable failed: %w", err)
	}
	if err := exec.Command("service", "caslink", "start").Run(); err != nil {
		return fmt.Errorf("start failed: %w", err)
	}
	fmt.Println("caslink service installed and started (rc.d)")
	return nil
}

func uninstall(m *Manager, configDir, dataDir, cacheDir, logDir, backupDir, pidFile string) error {
	fmt.Print("This will delete ALL data, configs, and the system service. Continue? [y/N] ")
	var answer string
	_, _ = fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("Aborted.")
		return nil
	}

	_ = exec.Command("service", "caslink", "stop").Run()
	_ = exec.Command("service", "caslink", "disable").Run()
	_ = os.Remove(rcdPath)

	for _, dir := range []string{configDir, dataDir, cacheDir, logDir, backupDir} {
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
	}
	if pidFile != "" {
		_ = os.Remove(pidFile)
	}

	fmt.Printf("Service uninstalled. Delete binary manually: rm %s\n", m.BinaryPath)
	return nil
}

func disable(name string) error {
	_ = exec.Command("service", name, "stop").Run()
	return exec.Command("service", name, "disable").Run()
}

func startSvc(name string) error {
	return exec.Command("service", name, "start").Run()
}

func stopSvc(name string) error {
	return exec.Command("service", name, "stop").Run()
}

func restartSvc(name string) error {
	return exec.Command("service", name, "restart").Run()
}

func reloadSvc(name string) error {
	return exec.Command("service", name, "reload").Run()
}
