package git

import (
	"bytes"
	"context"
	"errors"
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
	detail := strings.TrimSpace(e.Output)
	reason := ""
	switch {
	case errors.Is(e.Cause, context.Canceled):
		reason = "操作已取消"
	case errors.Is(e.Cause, context.DeadlineExceeded):
		reason = "已超过单条命令超时限制，可使用 --timeout 调整"
	case detail == "":
		detail = fmt.Sprint(e.Cause)
	}
	if reason != "" {
		if detail != "" {
			detail = reason + "\n" + detail
		} else {
			detail = reason
		}
	}
	return Redact(fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), detail))
}
func (e *Error) Unwrap() error { return e.Cause }

var credentials = regexp.MustCompile(`(https?://)[^\s/@]+@`)

func Redact(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	return credentials.ReplaceAllString(s, "${1}[redacted]@")
}
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
