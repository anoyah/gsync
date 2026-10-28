package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Repository struct {
	Runner
	Root, Branch, Upstream, Remote, Target string
}

func (r *Repository) Discover(ctx context.Context) error {
	root, err := r.Run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	r.Root = root
	r.Dir = root
	return nil
}

// CommonDir identifies a repository across all its linked worktrees.
func (r *Repository) CommonDir(ctx context.Context) (string, error) {
	p, err := r.Run(ctx, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.Dir, p)
	}
	return filepath.EvalSymlinks(p)
}
func (r *Repository) Check(ctx context.Context) error {
	branch, err := r.Run(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return fmt.Errorf("当前为 detached HEAD；请先切换到本地分支: %w", err)
	}
	r.Branch = branch
	for _, name := range []string{"MERGE_HEAD", "rebase-merge", "rebase-apply", "CHERRY_PICK_HEAD", "REVERT_HEAD", "sequencer", "BISECT_LOG", "index.lock", "HEAD.lock"} {
		p, err := r.Run(ctx, "rev-parse", "--git-path", name)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(r.Dir, p)
		}
		if _, err = os.Stat(p); err == nil {
			return fmt.Errorf("存在未完成的 Git 操作或锁 %s；请运行 git status 并完成或取消操作", name)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	status, err := r.Run(ctx, "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("工作区存在未提交修改；请运行 git status，提交或自行 git stash 后重试")
	}
	return nil
}
func (r *Repository) ResolveUpstream(ctx context.Context) error {
	// for-each-ref resolves custom fetch refspecs and differently named upstreams.
	s, err := r.Run(ctx, "for-each-ref", "--format=%(upstream)%00%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads/"+r.Branch)
	if err != nil {
		return err
	}
	parts := strings.Split(s, "\x00")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[1] == "." || !strings.HasPrefix(parts[2], "refs/heads/") {
		return fmt.Errorf("缺少有效远程上游；请检查 git remote -v，并用 git branch --set-upstream-to=<remote>/<branch> 配置")
	}
	r.Upstream = parts[0]
	r.Remote = parts[1]
	r.Target = parts[2]
	return nil
}
func (r *Repository) Difference(ctx context.Context) (int, int, error) {
	s, err := r.Run(ctx, "rev-list", "--left-right", "--count", "HEAD..."+r.Upstream, "--")
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(s)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("无法解析提交差异 %q", s)
	}
	a, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, err
	}
	b, err := strconv.Atoi(fields[1])
	return a, b, err
}
