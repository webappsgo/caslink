// Package svcmgr implements cross-platform service management for caslink.
package svcmgr

import (
	"fmt"
	"os"
	"runtime"
)

// Manager handles service install/uninstall/start/stop/restart/reload.
type Manager struct {
	BinaryPath  string
	ServiceName string
	DisplayName string
	Description string
}

// New returns a Manager configured for caslink.
func New() *Manager {
	exe, _ := os.Executable()
	return &Manager{
		BinaryPath:  exe,
		ServiceName: "caslink",
		DisplayName: "Caslink URL Shortener",
		Description: "Self-hosted URL shortener service",
	}
}

// Status returns a human-readable service status string.
func (m *Manager) Status() string {
	return checkStatus(m.ServiceName)
}

// Install installs, enables, and starts the service.
func (m *Manager) Install() error {
	return install(m)
}

// Uninstall stops, disables, and removes the service and all data.
func (m *Manager) Uninstall(configDir, dataDir, cacheDir, logDir, backupDir, pidFile string) error {
	return uninstall(m, configDir, dataDir, cacheDir, logDir, backupDir, pidFile)
}

// Disable stops and disables the service without removing data.
func (m *Manager) Disable() error {
	return disable(m.ServiceName)
}

// Start starts the service.
func (m *Manager) Start() error {
	return startSvc(m.ServiceName)
}

// Stop stops the service.
func (m *Manager) Stop() error {
	return stopSvc(m.ServiceName)
}

// Restart restarts the service.
func (m *Manager) Restart() error {
	return restartSvc(m.ServiceName)
}

// Reload sends a reload signal to the service.
func (m *Manager) Reload() error {
	return reloadSvc(m.ServiceName)
}

// PrintHelp prints the --service --help text.
func (m *Manager) PrintHelp() {
	status := m.Status()
	fmt.Printf(`Service management commands:

  start       Start the service
  stop        Stop the service
  restart     Restart the service
  reload      Reload configuration without restart

  --install   Install, enable, and start service
  --disable   Stop and disable service (keeps data)
  --uninstall Stop, disable, and remove everything (keeps binary)
  --help      Show this help

Current status:
  Platform:   %s
  Service:    %s
  Binary:     %s
`, runtime.GOOS, status, m.BinaryPath)
}
