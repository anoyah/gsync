package git

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

func isolateTerminal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
