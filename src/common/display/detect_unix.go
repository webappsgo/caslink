//go:build !windows

package display

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func (e *DisplayEnv) detectPlatformDisplay() {
	if waylandDisplay := os.Getenv("WAYLAND_DISPLAY"); waylandDisplay != "" {
		e.HasDisplay = true
		e.DisplayType = "wayland"
		return
	}
	if display := os.Getenv("DISPLAY"); display != "" {
		e.HasDisplay = true
		e.DisplayType = "x11"
		return
	}
	if runtime.GOOS == "darwin" {
		if !e.IsSSH && os.Getenv("__CFBundleIdentifier") != "" {
			e.HasDisplay = true
			e.DisplayType = "macos"
			return
		}
		cmd := exec.Command("launchctl", "managername")
		if output, err := cmd.Output(); err == nil {
			if strings.Contains(string(output), "Aqua") {
				e.HasDisplay = true
				e.DisplayType = "macos"
				return
			}
		}
	}
	e.HasDisplay = false
	e.DisplayType = "none"
}
