package syncer

import (
	"context"
	"fmt"
	"path/filepath"

	git "gsync/internal/git"
	"gsync/internal/lock"
)

func lockRepository(ctx context.Context, dir string, o Options) (*lock.Lock, error) {
	r := git.Repository{Runner: git.Runner{Dir: dir, Timeout: o.Timeout, NonInteractive: o.NonInteractive}}
	if _, err := r.Run(ctx, "--version"); err != nil {
		return nil, failure(1, err)
	}
	if err := r.Discover(ctx); err != nil {
		return nil, failure(3, err)
	}
	common, err := r.CommonDir(ctx)
	if err != nil {
		return nil, failure(3, err)
	}
	l, err := lock.Acquire(filepath.Join(common, "gsync.lock"))
	if err != nil {
		return nil, failure(4, fmt.Errorf("无法获取仓库锁: %w", err))
	}
	return l, nil
}
