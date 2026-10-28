package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Runner executes Git directly, with a separate deadline for every command.
type Runner struct {
	Dir            string
	Timeout        time.Duration
	Trace          io.Writer
	NonInteractive bool
}
type Error struct {
	Args   []string
	Output string
	Code   int
	Cause  error
}

func (e *Error) Error() string {
	return Redact(fmt.Sprintf("git %s: %s (%v)", strings.Join(e.Args, " "), e.Output, e.Cause))
}
func (e *Error) Unwrap() error { return e.Cause }

var credentials = regexp.MustCompile(`(https?://)[^\s/@]+@`)

func Redact(s string) string { return credentials.ReplaceAllString(s, "${1}[redacted]@") }
func (r Runner) Run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	if r.Trace != nil {
		fmt.Fprintf(r.Trace, "  git %s\n", Redact(strings.Join(args, " ")))
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Dir
	cmd.Stdin = os.Stdin
	if r.NonInteractive {
		cmd.Stdin = nil
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		isolateTerminal(cmd)
	}
	// Keep stdout separate so successful Git diagnostics cannot corrupt parsers.
	var out, diagnostic bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostic
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if err != nil {
		code := -1
		if e, ok := err.(*exec.ExitError); ok {
			code = e.ExitCode()
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return "", &Error{args, Redact(out.String() + diagnostic.String()), code, err}
	}
	if r.Trace != nil && diagnostic.Len() > 0 {
		fmt.Fprint(r.Trace, Redact(diagnostic.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
