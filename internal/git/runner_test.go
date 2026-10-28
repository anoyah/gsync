package git

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDeadline(t *testing.T) {
	r := Runner{Dir: t.TempDir(), Timeout: time.Nanosecond}
	_, err := r.Run(context.Background(), "--version")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline: %v", err)
	}
}
func TestRedact(t *testing.T) {
	got := Redact("fatal https://user:secret@example.com/repo and http://token@example.org/repo")
	if got != "fatal https://[redacted]@example.com/repo and http://[redacted]@example.org/repo" {
		t.Fatal(got)
	}
}

func TestErrorRedactsArguments(t *testing.T) {
	err := &Error{Args: []string{"fetch", "https://user:secret@example.com/repo"}, Output: "https://token@example.com/repo", Cause: errors.New("failed")}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "token") {
		t.Fatal("credentials leaked")
	}
}

func TestNonInteractiveRunner(t *testing.T) {
	r := Runner{Dir: t.TempDir(), Timeout: time.Second, NonInteractive: true}
	if out, err := r.Run(context.Background(), "--version"); err != nil || !strings.HasPrefix(out, "git version") {
		t.Fatalf("isolated Git command failed: %s %v", out, err)
	}
}

func TestCancellationAndTimeoutHints(t *testing.T) {
	for _, tc := range []struct {
		cause error
		hint  string
	}{{context.Canceled, "操作已取消"}, {context.DeadlineExceeded, "--timeout"}} {
		err := &Error{Args: []string{"fetch", "origin"}, Cause: tc.cause}
		if !errors.Is(err, tc.cause) || !strings.Contains(err.Error(), tc.hint) {
			t.Fatalf("missing useful hint: %v", err)
		}
	}
}
