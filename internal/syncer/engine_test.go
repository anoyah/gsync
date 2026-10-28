package syncer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	t                         *testing.T
	root, local, peer, remote string
}

func (f fixture) git(dir string, args ...string) string {
	f.t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(f.root, "no-config"), "GIT_TERMINAL_PROMPT=0", "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	b, e := c.CombinedOutput()
	if e != nil {
		f.t.Fatalf("git %v: %v\n%s", args, e, b)
	}
	return strings.TrimSpace(string(b))
}
func (f fixture) commit(dir, name, content string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		f.t.Fatal(err)
	}
	f.git(dir, "add", "--", name)
	f.git(dir, "commit", "-m", name)
}
func setup(t *testing.T) fixture {
	t.Helper()
	f := fixture{t: t, root: t.TempDir()}
	f.local = filepath.Join(f.root, "中文 local")
	f.peer = filepath.Join(f.root, "peer")
	f.remote = filepath.Join(f.root, "remote.git")
	// Isolate the engine's Git subprocesses from developer configuration, too.
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(f.root, "no-config"))
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	f.git(f.root, "init", "--bare", "--initial-branch=topic", f.remote)
	f.git(f.root, "clone", f.remote, f.local)
	for _, dir := range []string{f.local} {
		f.git(dir, "config", "user.name", "Test")
		f.git(dir, "config", "user.email", "test@example.com")
	}
	f.commit(f.local, "base", "base\n")
	f.git(f.local, "push", "-u", "origin", "topic")
	f.git(f.root, "clone", f.remote, f.peer)
	f.git(f.peer, "config", "user.name", "Test")
	f.git(f.peer, "config", "user.email", "test@example.com")
	return f
}
func execute(t *testing.T, dir string, o Options, want int) string {
	t.Helper()
	if o.Timeout == 0 {
		o.Timeout = 10 * time.Second
	}
	var out bytes.Buffer
	err := Run(context.Background(), dir, o, &out)
	code := 0
	if err != nil {
		var e *Error
		if !errors.As(err, &e) {
			t.Fatal(err)
		}
		code = e.Code
	}
	if code != want {
		t.Fatalf("code=%d want=%d error=%v output=%s", code, want, err, out.String())
	}
	return out.String()
}
func TestSyncStates(t *testing.T) {
	for _, state := range []string{"equal", "ahead", "behind", "diverged", "conflict", "dirty", "untracked", "no-upstream", "detached", "fetch-failure", "push-failure", "dry-run", "no-push", "no-rebase", "in-progress", "deleted-upstream", "different-target"} {
		t.Run(state, func(t *testing.T) {
			f := setup(t)
			want := 0
			o := Options{}
			before := f.git(f.local, "rev-parse", "HEAD")
			switch state {
			case "ahead", "no-push", "push-failure", "different-target":
				f.commit(f.local, "local", "local\n")
			case "behind", "dry-run":
				f.commit(f.peer, "remote", "remote\n")
				f.git(f.peer, "push")
				o.DryRun = state == "dry-run"
			case "diverged", "no-rebase":
				f.commit(f.local, "local", "local\n")
				f.commit(f.peer, "remote", "remote\n")
				f.git(f.peer, "push")
				if state == "no-rebase" {
					o.NoRebase = true
					want = 4
				}
			case "conflict":
				f.commit(f.local, "base", "local\n")
				before = f.git(f.local, "rev-parse", "HEAD")
				f.commit(f.peer, "base", "remote\n")
				f.git(f.peer, "push")
				want = 7
			case "dirty":
				if err := os.WriteFile(filepath.Join(f.local, "base"), []byte("dirty"), 0600); err != nil {
					t.Fatal(err)
				}
				want = 4
			case "untracked":
				if err := os.WriteFile(filepath.Join(f.local, "new"), []byte("new"), 0600); err != nil {
					t.Fatal(err)
				}
				want = 4
			case "no-upstream":
				f.git(f.local, "branch", "--unset-upstream")
				want = 5
			case "detached":
				f.git(f.local, "checkout", "--detach")
				want = 4
			case "fetch-failure":
				f.git(f.local, "remote", "set-url", "origin", filepath.Join(f.root, "missing"))
				want = 6
			case "in-progress":
				if err := os.WriteFile(filepath.Join(f.local, ".git", "MERGE_HEAD"), []byte(before+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
				want = 4
			case "deleted-upstream":
				f.git(f.remote, "update-ref", "-d", "refs/heads/topic")
				want = 6
			}
			if state == "no-push" {
				o.NoPush = true
			}
			if state == "push-failure" {
				f.git(f.remote, "config", "receive.denyNonFastForwards", "true")
				f.git(f.local, "remote", "set-url", "--push", "origin", filepath.Join(f.root, "missing"))
				want = 8
			}
			if state == "different-target" {
				f.git(f.local, "push", "origin", "HEAD:refs/heads/destination")
				f.git(f.local, "fetch")
				f.git(f.local, "branch", "--set-upstream-to=origin/destination")
				f.commit(f.local, "next", "next")
				f.git(f.local, "config", "push.default", "nothing")
				f.git(f.local, "config", "push.followTags", "true")
			}
			execute(t, f.local, o, want)
			if state == "conflict" || state == "dry-run" || state == "dirty" {
				if got := f.git(f.local, "rev-parse", "HEAD"); got != before {
					t.Fatal("HEAD unexpectedly changed")
				}
			}
			if state == "conflict" {
				if got := f.git(f.local, "status", "--porcelain"); got != "" {
					t.Fatalf("abort left dirty state: %s", got)
				}
			}
			if want == 0 && !o.DryRun && !o.NoPush {
				if f.git(f.local, "rev-parse", "HEAD") != f.git(f.local, "rev-parse", "@{upstream}") {
					t.Fatal("not synchronized")
				}
			}
			if state == "no-push" {
				if f.git(f.remote, "rev-parse", "topic") != before {
					t.Fatal("unexpected push")
				}
			}
		})
	}
}
func TestNotRepository(t *testing.T) { execute(t, t.TempDir(), Options{}, 3) }
func TestCanceled(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, f.local, Options{Timeout: time.Second}, &bytes.Buffer{})
	var e *Error
	if !errors.As(err, &e) || e.Code != 9 {
		t.Fatalf("expected cancellation: %v", err)
	}
}

func TestAbortFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX Git hook")
	}
	f := setup(t)
	f.commit(f.local, "local", "local")
	f.commit(f.peer, "remote", "remote")
	f.git(f.peer, "push")
	// A failing pre-rebase hook prevents rebase from starting, so abort also
	// fails. This exercises the recovery diagnostic with a real Git process.
	if err := os.WriteFile(filepath.Join(f.local, ".git", "hooks", "pre-rebase"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := Run(context.Background(), f.local, Options{Timeout: 10 * time.Second}, &out)
	var e *Error
	if !errors.As(err, &e) || e.Code != 7 || !strings.Contains(err.Error(), "Abort 失败") || !strings.Contains(err.Error(), "git rebase --abort") {
		t.Fatalf("missing recovery guidance: %v", err)
	}
}

func TestPushRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX Git hook")
	}
	f := setup(t)
	before := f.git(f.remote, "rev-parse", "topic")
	f.commit(f.local, "local", "local")
	if err := os.WriteFile(filepath.Join(f.remote, "hooks", "pre-receive"), []byte("#!/bin/sh\necho rejected >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	execute(t, f.local, Options{}, 8)
	if f.git(f.remote, "rev-parse", "topic") != before {
		t.Fatal("rejected push changed remote")
	}
}

func TestSubdirectory(t *testing.T) {
	f := setup(t)
	dir := filepath.Join(f.local, "目录 with spaces")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	execute(t, dir, Options{}, 0)
}

func TestDryRunPreservesHistory(t *testing.T) {
	for _, diverged := range []bool{false, true} {
		t.Run(map[bool]string{false: "ahead", true: "diverged"}[diverged], func(t *testing.T) {
			f := setup(t)
			f.commit(f.local, "local", "local")
			if diverged {
				f.commit(f.peer, "remote", "remote")
				f.git(f.peer, "push")
			}
			local := f.git(f.local, "rev-parse", "HEAD")
			remote := f.git(f.remote, "rev-parse", "topic")
			execute(t, f.local, Options{DryRun: true}, 0)
			if f.git(f.local, "rev-parse", "HEAD") != local || f.git(f.remote, "rev-parse", "topic") != remote {
				t.Fatal("dry-run changed history")
			}
			if f.git(f.local, "rev-parse", "@{upstream}") != remote {
				t.Fatal("dry-run did not fetch")
			}
		})
	}
}

func TestNoPushRebases(t *testing.T) {
	f := setup(t)
	f.commit(f.local, "local", "local")
	f.commit(f.peer, "remote", "remote")
	f.git(f.peer, "push")
	remote := f.git(f.remote, "rev-parse", "topic")
	out := execute(t, f.local, Options{NoPush: true}, 0)
	if f.git(f.remote, "rev-parse", "topic") != remote {
		t.Fatal("unexpected push")
	}
	if f.git(f.local, "rev-list", "--left-right", "--count", "HEAD...@{upstream}") != "1\t0" {
		t.Fatal("remote updates were not integrated")
	}
	if !strings.Contains(out, "保留本地提交") {
		t.Fatal(out)
	}
}

func TestCustomFetchRefspec(t *testing.T) {
	f := setup(t)
	f.git(f.local, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/custom/*")
	f.git(f.local, "fetch")
	f.commit(f.peer, "remote", "remote")
	f.git(f.peer, "push")
	execute(t, f.local, Options{}, 0)
	if f.git(f.local, "rev-parse", "HEAD") != f.git(f.remote, "rev-parse", "topic") {
		t.Fatal("custom upstream not synchronized")
	}
}

func TestLinkedWorktree(t *testing.T) {
	f := setup(t)
	dir := filepath.Join(f.root, "linked worktree")
	f.git(f.local, "worktree", "add", "-b", "linked", dir, "topic")
	f.git(dir, "branch", "--set-upstream-to=origin/topic")
	f.commit(f.peer, "remote", "remote")
	f.git(f.peer, "push")
	execute(t, dir, Options{}, 0)
	if f.git(dir, "rev-parse", "HEAD") != f.git(f.remote, "rev-parse", "topic") {
		t.Fatal("worktree not synchronized")
	}
}

func TestStagedChangesPreserved(t *testing.T) {
	f := setup(t)
	if err := os.WriteFile(filepath.Join(f.local, "base"), []byte("staged"), 0600); err != nil {
		t.Fatal(err)
	}
	f.git(f.local, "add", "base")
	before := f.git(f.local, "diff", "--cached")
	execute(t, f.local, Options{}, 4)
	if f.git(f.local, "diff", "--cached") != before {
		t.Fatal("staged changes modified")
	}
}

func TestIdleCycleUsesOneSafetyScan(t *testing.T) {
	f := setup(t)
	out := execute(t, f.local, Options{Verbose: true, NoPush: true, NoRebase: true}, 0)
	count := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  git ") {
			count++
		}
	}
	if count > 9 {
		t.Fatalf("idle cycle spawned %d traced commands, want at most 9\n%s", count, out)
	}
	if strings.Count(out, "--git-path MERGE_HEAD") != 1 || strings.Count(out, "status --porcelain") != 1 {
		t.Fatal("idle safety scan repeated")
	}
	t.Logf("idle cycle: %d Git commands (excluding lock preparation)", count)
}

func TestBatchedSafetyScanDetectsEveryMarker(t *testing.T) {
	f := setup(t)
	for _, name := range []string{"MERGE_HEAD", "rebase-merge", "rebase-apply", "CHERRY_PICK_HEAD", "REVERT_HEAD", "sequencer", "BISECT_LOG", "index.lock", "HEAD.lock"} {
		path := filepath.Join(f.local, ".git", name)
		if err := os.WriteFile(path, []byte("in progress"), 0600); err != nil {
			t.Fatal(err)
		}
		execute(t, f.local, Options{NoPush: true, NoRebase: true}, 4)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFastForwardRetainsFinalSafetyScan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX Git hook")
	}
	f := setup(t)
	f.commit(f.peer, "remote", "remote update")
	f.git(f.peer, "push")
	hook := filepath.Join(f.local, ".git", "hooks", "post-merge")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf 'hook modification' > base\n"), 0700); err != nil {
		t.Fatal(err)
	}
	execute(t, f.local, Options{NoPush: true, NoRebase: true}, 4)
	data, err := os.ReadFile(filepath.Join(f.local, "base"))
	if err != nil || string(data) != "hook modification" {
		t.Fatal("post-merge modification was not preserved")
	}
}
