package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	git "github.com/anoyah/gsync/internal/git"
	"github.com/anoyah/gsync/internal/service"
	"github.com/anoyah/gsync/internal/syncer"
)

type Snapshot struct {
	Directory, Branch, Upstream, Worktree, Service, Logs, Detail string
	Ahead, Behind                                                int
	Known                                                        bool
	Updated                                                      time.Time
}

type Backend interface {
	Snapshot(context.Context) (Snapshot, error)
	Watch(context.Context, io.Writer) error
	Start(context.Context) (string, error)
	Stop(context.Context) (string, error)
	BackgroundAvailable() bool
}

type repositoryBackend struct {
	root, common string
	options      syncer.Options
	interval     time.Duration
	manager      *service.Manager
}

func NewBackend(ctx context.Context, dir string, o syncer.Options, interval time.Duration) (Backend, error) {
	o.NoPush = true
	o.NoRebase = true
	o.NonInteractive = true
	r := git.Repository{Runner: git.Runner{Dir: dir, Timeout: o.Timeout, NonInteractive: true}}
	if _, err := r.Run(ctx, "--version"); err != nil {
		return nil, &syncer.Error{Code: 1, Err: err}
	}
	if err := r.Discover(ctx); err != nil {
		return nil, &syncer.Error{Code: 3, Err: err}
	}
	common, err := r.CommonDir(ctx)
	if err != nil {
		return nil, err
	}
	b := &repositoryBackend{root: r.Root, common: common, options: o, interval: interval}
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		b.manager = service.New(home, os.Getuid(), exe, os.Getenv("PATH"))
	}
	return b, nil
}

func (b *repositoryBackend) BackgroundAvailable() bool { return b.manager != nil }

// Snapshot never fetches or changes history. The UI can observe a repository
// even while its watcher owns the repository lock.
func (b *repositoryBackend) Snapshot(ctx context.Context) (Snapshot, error) {
	s := Snapshot{Directory: b.root, Updated: time.Now(), Service: "此平台不支持后台服务", Upstream: "未配置"}
	r := git.Repository{Runner: git.Runner{Dir: b.root, Timeout: b.options.Timeout, NonInteractive: true}}
	branch, err := r.Run(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if ctx.Err() != nil {
			return s, ctx.Err()
		}
		s.Branch = "detached HEAD / 分支不可用"
		s.Detail = err.Error()
	} else {
		s.Branch = branch
		r.Branch = branch
	}
	status, err := r.Run(ctx, "--no-optional-locks", "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return s, err
	}
	if status == "" {
		s.Worktree = "干净"
	} else {
		s.Worktree = fmt.Sprintf("有修改（%d 项），等待人工处理", len(strings.Split(status, "\n")))
	}
	if r.Branch != "" {
		if err := r.ResolveUpstream(ctx); err != nil {
			s.Detail = err.Error()
		} else {
			s.Upstream = r.Upstream
			a, c, err := r.Difference(ctx)
			if err != nil {
				s.Detail = err.Error()
			} else {
				s.Ahead = a
				s.Behind = c
				s.Known = true
				if a > 0 && c > 0 {
					s.Detail = "分支已分叉，暂停自动拉取，等待人工处理"
				}
			}
		}
	}
	if b.manager != nil {
		serviceStatus, err := b.manager.Status(ctx, b.common)
		if err != nil {
			s.Service = "状态读取失败：" + err.Error()
		} else {
			s.Service = serviceStatus
		}
		logs, err := b.manager.Logs(b.common, 200)
		if err != nil {
			s.Logs = "日志读取失败：" + err.Error()
		} else {
			s.Logs = logs
		}
	}
	return s, nil
}

func (b *repositoryBackend) Watch(ctx context.Context, w io.Writer) error {
	return syncer.Watch(ctx, b.root, b.options, b.interval, w, w)
}
func (b *repositoryBackend) Start(ctx context.Context) (string, error) {
	if b.manager == nil {
		return "", fmt.Errorf("后台服务仅支持 macOS，请按 w 使用前台监听")
	}
	started, err := b.manager.Start(ctx, b.common, service.Config{Directory: b.root, Interval: b.interval, Timeout: b.options.Timeout, Verbose: b.options.Verbose})
	if err != nil {
		return "", err
	}
	if !started {
		return "后台服务已加载；更改配置前请先停止", nil
	}
	return "后台服务已加载并配置登录启动，请检查状态与日志", nil
}
func (b *repositoryBackend) Stop(ctx context.Context) (string, error) {
	if b.manager == nil {
		return "", fmt.Errorf("后台服务仅支持 macOS")
	}
	if err := b.manager.Stop(ctx, b.common); err != nil {
		return "", err
	}
	return "后台服务已停止，已取消登录启动", nil
}
