package syncer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"gsync/internal/lock"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatchRetriesAndRecovers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, diagnostic bytes.Buffer
	calls := 0
	err := watch(ctx, time.Millisecond, false, &out, &diagnostic, func(context.Context, io.Writer) error {
		calls++
		if calls <= 2 {
			return failure(4, fmt.Errorf("dirty"))
		}
		if calls == 4 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || calls != 4 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	if strings.Count(diagnostic.String(), "dirty") != 1 {
		t.Fatal("duplicate error was not suppressed")
	}
	if !strings.Contains(out.String(), "仓库已恢复") {
		t.Fatal("missing recovery message")
	}
}

func TestWatchWaitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- watch(ctx, time.Hour, false, io.Discard, io.Discard, func(context.Context, io.Writer) error { close(called); return nil })
	}()
	<-called
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt wait")
	}
}

func TestWatchFatalErrors(t *testing.T) {
	for _, code := range []int{1, 3, 7} {
		calls := 0
		err := watch(context.Background(), time.Millisecond, false, io.Discard, io.Discard, func(context.Context, io.Writer) error { calls++; return failure(code, fmt.Errorf("fatal")) })
		var e *Error
		if calls != 1 || !errors.As(err, &e) || e.Code != code {
			t.Fatalf("code=%d calls=%d err=%v", code, calls, err)
		}
	}
}

func TestWatchPullsAfterDivergenceResolved(t *testing.T) {
	f := setup(t)
	f.commit(f.peer, "remote", "remote update")
	f.git(f.peer, "push")
	remote := f.git(f.remote, "rev-parse", "topic")
	// A local commit is never pushed, even after multiple watching cycles.
	f.commit(f.local, "local", "local change")
	local := f.git(f.local, "rev-parse", "HEAD")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := watch(ctx, time.Millisecond, false, io.Discard, io.Discard, func(ctx context.Context, w io.Writer) error {
		calls++
		err := Run(ctx, f.local, Options{NoPush: true, NoRebase: true, Timeout: 10 * time.Second}, w)
		if calls == 1 {
			var e *Error
			if !errors.As(err, &e) || e.Code != 4 {
				t.Fatalf("divergence should pause: %v", err)
			}
			if f.git(f.local, "rev-parse", "HEAD") != local || f.git(f.remote, "rev-parse", "topic") != remote {
				t.Fatal("divergence changed history")
			}
			// Simulate the user switching to a clean branch tracking the server.
			f.git(f.local, "checkout", "-b", "clean", "origin/topic")
			f.commit(f.peer, "next", "next remote update")
			f.git(f.peer, "push")
		} else {
			if err != nil {
				t.Fatal(err)
			}
			if f.git(f.local, "rev-parse", "HEAD") != f.git(f.remote, "rev-parse", "topic") {
				t.Fatal("watch did not pull the next remote update")
			}
			cancel()
		}
		return err
	})
	if !errors.Is(err, context.Canceled) || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestWatchPullsAfterDirtyWorktreeRecovers(t *testing.T) {
	f := setup(t)
	f.commit(f.peer, "remote", "remote update")
	f.git(f.peer, "push")
	path := filepath.Join(f.local, "base")
	if err := os.WriteFile(path, []byte("uncommitted"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := watch(ctx, time.Millisecond, false, io.Discard, io.Discard, func(ctx context.Context, w io.Writer) error {
		calls++
		err := Run(ctx, f.local, Options{NoPush: true, NoRebase: true, Timeout: 10 * time.Second}, w)
		if calls == 1 {
			var e *Error
			if !errors.As(err, &e) || e.Code != 4 {
				t.Fatalf("dirty worktree should pause: %v", err)
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil || string(data) != "uncommitted" {
				t.Fatal("user modification changed")
			}
			if err := os.WriteFile(path, []byte("base\n"), 0600); err != nil {
				t.Fatal(err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			if f.git(f.local, "rev-parse", "HEAD") != f.git(f.remote, "rev-parse", "topic") {
				t.Fatal("remote update not pulled")
			}
			cancel()
		}
		return err
	})
	if !errors.Is(err, context.Canceled) || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestWatchForcesPullOnly(t *testing.T) {
	f := setup(t)
	remote := f.git(f.remote, "rev-parse", "topic")
	f.commit(f.local, "local", "local change")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := Watch(ctx, f.local, Options{Timeout: 10 * time.Second}, time.Hour, io.Discard, io.Discard)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if f.git(f.remote, "rev-parse", "topic") != remote {
		t.Fatal("watch pushed local commits")
	}
}

func TestLinkedWorktreesShareLock(t *testing.T) {
	f := setup(t)
	dir := filepath.Join(f.root, "linked")
	f.git(f.local, "worktree", "add", "-b", "linked", dir, "topic")
	f.git(dir, "branch", "--set-upstream-to=origin/topic")
	l, err := lock.Acquire(filepath.Join(f.local, ".git", "gsync.lock"))
	if err != nil {
		t.Fatal(err)
	}
	execute(t, dir, Options{NoPush: true, NoRebase: true}, 4)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	execute(t, dir, Options{NoPush: true, NoRebase: true}, 0)
}
