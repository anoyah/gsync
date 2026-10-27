package main

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

// Exercise the actual CLI so option routing cannot accidentally enable the
// legacy engine's push/rebase behavior or turn the default into a single run.
func TestCLIWatchPullOnly(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "no-config"))
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
		return strings.TrimSpace(string(b))
	}
	commit := func(dir, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		git(dir, "add", "--", name)
		git(dir, "commit", "-m", name)
	}
	remote := filepath.Join(root, "remote.git")
	peer := filepath.Join(root, "peer")
	local := filepath.Join(root, "local")
	git(root, "init", "--bare", "--initial-branch=topic", remote)
	git(root, "clone", remote, peer)
	commit(peer, "base", "base\n")
	git(peer, "push", "-u", "origin", "topic")
	git(root, "clone", remote, local)
	binary := filepath.Join(root, "gsync")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if b, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, b)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	cmd := exec.CommandContext(ctx, binary, "--interval=20ms")
	cmd.Dir = local
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stopped := false
	defer func() {
		cancel()
		if !stopped {
			<-done
		}
	}()
	waitForHead := func(want string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if git(local, "rev-parse", "HEAD") == want {
				return
			}
			select {
			case err := <-done:
				stopped = true
				t.Fatalf("watch exited early: %v\n%s", err, output.String())
			default:
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatal("watch did not pull remote update before deadline")
	}
	commit(peer, "first", "first")
	git(peer, "push")
	waitForHead(git(peer, "rev-parse", "HEAD"))
	commit(peer, "second", "second")
	git(peer, "push")
	waitForHead(git(peer, "rev-parse", "HEAD"))
	// An unpushed local commit must survive both one-shot and watch modes.
	commit(local, "local", "local only")
	remoteHead := git(remote, "rev-parse", "topic")
	if runtime.GOOS != "windows" {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			t.Fatal(err)
		}
	} else {
		cancel()
	}
	err := <-done
	stopped = true
	if runtime.GOOS != "windows" {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 9 {
			t.Fatalf("interrupt: %v\n%s", err, output.String())
		}
	}
	if git(remote, "rev-parse", "topic") != remoteHead {
		t.Fatal("watch pushed local commits")
	}
	once := exec.Command(binary, "--once")
	once.Dir = local
	if b, err := once.CombinedOutput(); err != nil {
		t.Fatalf("once: %v\n%s", err, b)
	}
	if git(remote, "rev-parse", "topic") != remoteHead {
		t.Fatal("CLI pushed local commits")
	}
}
