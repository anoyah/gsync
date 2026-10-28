//go:build !darwin && !linux && !windows

package lock

import (
	"fmt"
	"os"
)

func acquire(*os.File) error { return fmt.Errorf("当前平台尚不支持仓库锁") }
