package tui

import (
	"golang.org/x/sys/windows"
	"os"
)

func duplicateInput(input *os.File) (*os.File, error) {
	var handle windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(input.Fd()), windows.CurrentProcess(), &handle, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), "gsync-tui-input"), nil
}
func prepareOutput(out *os.File) (func(), error) {
	h := windows.Handle(out.Fd())
	var old uint32
	if err := windows.GetConsoleMode(h, &old); err != nil {
		return nil, err
	}
	if err := windows.SetConsoleMode(h, old|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return nil, err
	}
	return func() { _ = windows.SetConsoleMode(h, old) }, nil
}
