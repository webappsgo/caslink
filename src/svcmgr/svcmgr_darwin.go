//go:build darwin

package svcmgr

import (
	"fmt"
	"os"
	"os/exec"
)

const plistName = "us.casapps.caslink"
const plistPath = "/Library/LaunchDaemons/" + plistName + ".plist"

const launchdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>us.casapps.caslink</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/caslink</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/casapps/caslink/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/casapps/caslink/stderr.log</string>
</dict>
</plist>
`

func checkStatus(name string) string {
	out, err := exec.Command("launchctl", "list", plistName).Output()
	if err != nil {
		return "not installed"
	}
	if len(out) > 0 {
		return "running"
	}
	return "stopped"
}

func install(m *Manager) error {
	if err := os.WriteFile(plistPath, []byte(launchdPlist), 0644); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("launchctl load failed: %w", err)
	}
	fmt.Println("caslink service installed and started (launchd)")
	return nil
}

func uninstall(m *Manager, configDir, dataDir, cacheDir, logDir, backupDir, pidFile string) error {
	fmt.Print("This will delete ALL data, configs, and the system service. Continue? [y/N] ")
	var answer string
	fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("Aborted.")
		return nil
	}

	exec.Command("launchctl", "unload", plistPath).Run()
	os.Remove(plistPath)

	for _, dir := range []string{configDir, dataDir, cacheDir, logDir, backupDir} {
		if dir != "" {
			os.RemoveAll(dir)
		}
	}
	if pidFile != "" {
		os.Remove(pidFile)
	}

	fmt.Printf("Service uninstalled. Delete binary manually: rm %s\n", m.BinaryPath)
	return nil
}

func disable(name string) error {
	exec.Command("launchctl", "unload", plistPath).Run()
	return nil
}

func startSvc(name string) error {
	return exec.Command("launchctl", "load", plistPath).Run()
}

func stopSvc(name string) error {
	return exec.Command("launchctl", "unload", plistPath).Run()
}

func restartSvc(name string) error {
	exec.Command("launchctl", "unload", plistPath).Run()
	return exec.Command("launchctl", "load", plistPath).Run()
}

func reloadSvc(name string) error {
	return exec.Command("launchctl", "kickstart", "-k", "system/"+plistName).Run()
}
