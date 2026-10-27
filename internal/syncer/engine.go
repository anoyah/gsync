package syncer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	git "gsync/internal/git"
)

type Options struct {
	DryRun, NoPush, NoRebase, Verbose bool
	Timeout                           time.Duration
}
type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }
func failure(code int, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		code = 9
	}
	return &Error{code, err}
}
func Run(ctx context.Context, dir string, o Options, w io.Writer) error {
	r := git.Repository{Runner: git.Runner{Dir: dir, Timeout: o.Timeout}}
	if o.Verbose {
		r.Trace = w
	}
	if _, err := r.Run(ctx, "--version"); err != nil {
		return failure(1, err)
	}
	if err := r.Discover(ctx); err != nil {
		return failure(3, fmt.Errorf("当前目录不属于可用 Git 工作树: %w", err))
	}
	if err := r.Check(ctx); err != nil {
		return failure(4, err)
	}
	if err := r.ResolveUpstream(ctx); err != nil {
		return failure(5, err)
	}
	fmt.Fprintf(w, "项目：%s\n分支：%s\n远程：%s\n上游：%s\n[1/4] 检查仓库状态 完成\n", filepath.Base(r.Root), r.Branch, r.Remote, r.Upstream)
	if _, err := r.Run(ctx, "fetch", "--no-prune", "--no-tags", "--", r.Remote, "+"+r.Target+":"+r.Upstream); err != nil {
		return failure(6, fmt.Errorf("Fetch 失败；请检查网络、认证和 git remote -v: %w", err))
	}
	fmt.Fprintln(w, "[2/4] 获取远程更新 完成")
	a, b, err := r.Difference(ctx)
	if err != nil {
		return failure(5, fmt.Errorf("无法比较上游；请检查远程分支是否存在: %w", err))
	}
	fmt.Fprintf(w, "[3/4] 分析提交差异 本地领先 %d，远程领先 %d\n", a, b)
	action := "无需操作"
	if a > 0 && b == 0 && o.NoPush {
		action = "保留本地提交，不推送"
	}
	if b > 0 {
		action = "Fast-forward"
		if a > 0 {
			action = "Rebase"
		}
	}
	if a > 0 && !o.NoPush {
		if b > 0 {
			action += "，然后 Push"
		} else {
			action = "Push"
		}
	}
	if a > 0 && b > 0 && o.NoRebase {
		return failure(4, fmt.Errorf("双方已分叉，禁止自动 Rebase；请手动处理分叉后重试"))
	}
	if o.DryRun {
		fmt.Fprintf(w, "计划：%s\nDry-run：已 Fetch，未执行整合或 Push。\n", action)
		return nil
	}
	// Recheck safety after fetching, which can take arbitrarily long.
	branch := r.Branch
	if err := r.Check(ctx); err != nil {
		return failure(4, err)
	}
	if branch != r.Branch {
		return failure(4, fmt.Errorf("当前分支发生变化，已停止"))
	}
	if b > 0 {
		command := "merge"
		args := []string{"merge", "--ff-only", "--no-autostash", "--", r.Upstream}
		if a > 0 {
			command = "rebase"
			args = []string{"-c", "rebase.autoSquash=false", "-c", "rebase.updateRefs=false", "-c", "rebase.rebaseMerges=false", "rebase", "--no-autostash", r.Upstream}
		}
		if _, err := r.Run(ctx, args...); err != nil {
			if command == "rebase" {
				// Cleanup has its own bounded context even after Ctrl+C or timeout.
				_, abortErr := r.Run(context.Background(), "rebase", "--abort")
				if abortErr != nil {
					return failure(7, fmt.Errorf("Rebase 失败: %w\nAbort 失败: %v\n仓库可能仍处于 Rebase 中；请运行 git status 和 git rebase --abort", err, abortErr))
				}
				return failure(7, fmt.Errorf("Rebase 失败，已 Abort；请用 git status 检查并手动处理冲突: %w", err))
			}
			return failure(7, fmt.Errorf("Fast-forward 失败；请运行 git status: %w", err))
		}
	}
	a, b, err = r.Difference(ctx)
	if err != nil {
		return failure(7, err)
	}
	if b > 0 {
		return failure(7, fmt.Errorf("整合后仍落后上游，已停止；请运行 git status"))
	}
	if err := r.Check(ctx); err != nil {
		return failure(4, err)
	}
	if branch != r.Branch {
		return failure(4, fmt.Errorf("当前分支发生变化，已停止"))
	}
	if a > 0 && !o.NoPush {
		if _, err := r.Run(ctx, "-c", "push.followTags=false", "push", "--no-follow-tags", "--recurse-submodules=no", "--", r.Remote, "HEAD:"+r.Target); err != nil {
			return failure(8, fmt.Errorf("Push 失败；请检查权限或重新运行 gsync 获取最新远程状态: %w", err))
		}
	}
	a, b, err = r.Difference(ctx)
	if err != nil {
		return failure(1, err)
	}
	if o.NoPush && a > 0 {
		fmt.Fprintln(w, "[4/4] 远程更新已检查，保留本地提交，不推送")
	} else {
		fmt.Fprintln(w, "[4/4] 同步完成")
	}
	fmt.Fprintf(w, "本地领先：%d，远程领先：%d\n", a, b)
	head, err := r.Run(ctx, "rev-parse", "--short", "HEAD")
	if err != nil {
		return failure(1, err)
	}
	fmt.Fprintln(w, "当前提交：", head)
	return nil
}
