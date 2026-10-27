package git

import (
	"context"
	"errors"
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
