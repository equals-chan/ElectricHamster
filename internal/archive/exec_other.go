//go:build !windows

package archive

import "os/exec"

// configureProcess is a no-op on non-Windows platforms.
func configureProcess(cmd *exec.Cmd) {}
