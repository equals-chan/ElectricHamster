//go:build windows

package archive

import (
	"os/exec"
	"syscall"
)

// configureProcess hides the console window that would otherwise flash when a
// GUI application spawns 7-Zip.
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
