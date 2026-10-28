// Package lock provides a process-scoped advisory lock. The lock file is kept
// after release: removing it could allow another process to lock a new inode.
package lock

import (
	"errors"
	"fmt"
	"os"
)

var ErrBusy = errors.New("已有 gsync 正在管理此仓库")

type Lock struct{ file *os.File }

func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := acquire(f); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{file: f}, nil
}

func (l *Lock) Close() error { return l.file.Close() }
