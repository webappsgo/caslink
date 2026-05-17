//go:build windows

package svcmgr

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func checkStatus(name string) string {
	out, err := exec.Command("sc", "query", name).Output()
	if err != nil {
		return "not installed"
	}
	if len(out) > 0 {
		return "see sc query output"
	}
	return "unknown"
}

func install(m *Manager) error {
	cmd := exec.Command("sc", "create", m.ServiceName,
		"binPath=", m.BinaryPath,
		"DisplayName=", m.DisplayName,
		"start=", "auto",
		"obj=", "LocalSystem")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sc create failed: %w", err)
	}
	if err := exec.Command("sc", "start", m.ServiceName).Run(); err != nil {
		return fmt.Errorf("sc start failed: %w", err)
	}
	fmt.Println("caslink service installed and started (Windows SCM)")
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

	_ = exec.Command("sc", "stop", m.ServiceName).Run()
	time.Sleep(2 * time.Second)
	_ = exec.Command("sc", "delete", m.ServiceName).Run()

	for _, dir := range []string{configDir, dataDir, cacheDir, logDir, backupDir} {
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
	}
	if pidFile != "" {
		_ = os.Remove(pidFile)
	}

	fmt.Printf("Service uninstalled. Delete binary manually: del %s\n", m.BinaryPath)
	return nil
}

func disable(name string) error {
	_ = exec.Command("sc", "stop", name).Run()
	time.Sleep(2 * time.Second)
	return exec.Command("sc", "config", name, "start=", "disabled").Run()
}

func startSvc(name string) error {
	return exec.Command("sc", "start", name).Run()
}

func stopSvc(name string) error {
	return exec.Command("sc", "stop", name).Run()
}

func restartSvc(name string) error {
	_ = exec.Command("sc", "stop", name).Run()
	time.Sleep(2 * time.Second)
	return exec.Command("sc", "start", name).Run()
}

func reloadSvc(name string) error {
	return restartSvc(name)
}
