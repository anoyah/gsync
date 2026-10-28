//go:build !windows

package tui

import (
	"os"
	"syscall"
)

func duplicateInput(input *os.File) (*os.File, error) {
	fd, err := syscall.Dup(int(input.Fd()))
	if err != nil {
		return nil, err
	}
	syscall.CloseOnExec(fd)
	return os.NewFile(uintptr(fd), "gsync-tui-input"), nil
}
func prepareOutput(*os.File) (func(), error) { return func() {}, nil }
