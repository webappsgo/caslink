//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

// replaceBinary replaces the running binary (Windows).
func replaceBinary(currentPath, newBinaryPath string) error {
	oldPath := currentPath + ".old"
	os.Remove(oldPath)

	if err := os.Rename(currentPath, oldPath); err != nil {
		return fmt.Errorf("failed to rename current binary: %w", err)
	}

	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		os.Rename(oldPath, currentPath)
		return fmt.Errorf("failed to move new binary: %w", err)
	}

	oldPathPtr, _ := windows.UTF16PtrFromString(oldPath)
	windows.MoveFileEx(oldPathPtr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)

	return nil
}

// RestartSelf starts a new instance and exits (Windows).
func RestartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start new process: %w", err)
	}

	time.Sleep(100 * time.Millisecond)
	os.Exit(0)
	return nil
}
