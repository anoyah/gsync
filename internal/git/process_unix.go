//go:build !windows

package git

import (
	"os/exec"
	"syscall"
)

// SSH can open /dev/tty even with a closed stdin. A separate session keeps it
// from reading TUI keys while still allowing an existing SSH agent to work.
func isolateTerminal(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
