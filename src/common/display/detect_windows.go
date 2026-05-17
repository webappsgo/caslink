//go:build windows

package display

import (
	"os"

	"golang.org/x/sys/windows"
)

func (e *DisplayEnv) detectPlatformDisplay() {
	var sessionID uint32
	_ = windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sessionID)
	if sessionID == 0 {
		e.HasDisplay = false
		e.DisplayType = "none"
		return
	}
	if os.Getenv("SESSIONNAME") != "" {
		e.HasDisplay = true
		e.DisplayType = "windows"
		return
	}
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	hwnd, _, _ := getConsoleWindow.Call()
	e.HasDisplay = hwnd != 0
	if e.HasDisplay {
		e.DisplayType = "windows"
	} else {
		e.DisplayType = "none"
	}
}
