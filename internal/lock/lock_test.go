package lock

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExclusionAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gsync.lock")
	first, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Acquire(path); !errors.Is(err, ErrBusy) {
		if other != nil {
			other.Close()
		}
		first.Close()
		t.Fatalf("expected busy: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal("persistent lock file removed:", err)
	}
}

func TestLockHelper(t *testing.T) {
	path := os.Getenv("GSYNC_TEST_LOCK_PATH")
	if path == "" {
		return
	}
	l, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestCrashReleasesLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gsync.lock")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLockHelper$")
	cmd.Env = append(os.Environ(), "GSYNC_TEST_LOCK_PATH="+path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatal("helper did not acquire lock")
	}
	if other, err := Acquire(path); !errors.Is(err, ErrBusy) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("expected cross-process exclusion: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	l, err := Acquire(path)
	if err != nil {
		t.Fatalf("crashed process left lock held: %v", err)
	}
	l.Close()
}
