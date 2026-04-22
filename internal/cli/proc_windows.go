//go:build windows

package cli

import (
	"os/exec"
	"syscall"
)

func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// waitForProcessExit blocks until the process has fully exited and the OS
// has released all its file handles. Uses WaitForSingleObject with an infinite
// timeout — this is the correct Windows synchronization primitive, not a poll loop.
func waitForProcessExit(pid int) {
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return // process already gone
	}
	defer syscall.CloseHandle(h)
	syscall.WaitForSingleObject(h, 0xFFFFFFFF) // INFINITE
}

// isProcessAlive checks whether a process is still running on Windows.
// Signal(0) is a Unix concept and always fails on Windows, so we use
// OpenProcess + GetExitCodeProcess instead. STILL_ACTIVE (259) means running.
func isProcessAlive(pid int) bool {
	const processQueryLimitedInformation = 0x1000
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == 259 // STILL_ACTIVE
}
