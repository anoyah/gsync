package syncer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Watch polls both local and remote state. Each interval starts after the
// previous cycle finishes, so slow fetches cannot overlap subsequent syncs.
func Watch(ctx context.Context, dir string, o Options, interval time.Duration, out, errOut io.Writer) error {
	// Watching is pull-only, regardless of the options of a one-shot engine.
	o.NoPush = true
	o.NoRebase = true
	l, err := lockRepository(ctx, dir, o)
	if err != nil {
		return err
	}
	defer l.Close()
	return watch(ctx, interval, o.Verbose, out, errOut, func(ctx context.Context, w io.Writer) error {
		return runUnlocked(ctx, dir, o, w)
	})
}

func watch(ctx context.Context, interval time.Duration, verbose bool, out, errOut io.Writer, cycle func(context.Context, io.Writer) error) error {
	if interval <= 0 {
		return failure(2, fmt.Errorf("监听间隔必须大于 0"))
	}
	fmt.Fprintf(out, "正在持续监听，每轮完成后等待 %s；Ctrl+C 停止。\n", interval)
	last := ""
	delay := interval
	for {
		if ctx.Err() != nil {
			return failure(9, ctx.Err())
		}
		var buf bytes.Buffer
		err := cycle(ctx, &buf)
		if ctx.Err() != nil {
			if err != nil && verbose {
				fmt.Fprintln(errOut, err)
			}
			return failure(9, ctx.Err())
		}
		key := "ok:" + buf.String()
		if err != nil {
			key = "error:" + err.Error()
		}
		if verbose || key != last {
			fmt.Fprintf(out, "\n[%s]\n", time.Now().Format(time.RFC3339))
			if err == nil || verbose {
				fmt.Fprint(out, buf.String())
			}
			if err != nil {
				fmt.Fprintln(errOut, "本轮未拉取：", err)
			}
			if err == nil && strings.HasPrefix(last, "error:") {
				fmt.Fprintln(out, "仓库已恢复，继续自动拉取。")
			}
		}
		if err != nil {
			var syncErr *Error
			if !errors.As(err, &syncErr) || syncErr.Code == 1 || syncErr.Code == 3 || syncErr.Code == 7 {
				return fmt.Errorf("监听已停止；请处理错误后重新运行 gsync watch: %w", err)
			}
		}
		next := retryDelay(interval, delay, err)
		if err != nil && (next != delay || key != last) {
			fmt.Fprintf(errOut, "下次检查将在 %s 后进行；修复后会自动继续。\n", next)
		}
		delay = next
		last = key
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return failure(9, ctx.Err())
		case <-timer.C:
		}
	}
}

// Local safety problems stay responsive. Only failed network/timeout cycles
// back off; successful checks immediately restore the configured interval.
func retryDelay(base, current time.Duration, err error) time.Duration {
	var e *Error
	if !errors.As(err, &e) || (e.Code != 6 && e.Code != 9) {
		return base
	}
	limit := 5 * time.Minute
	if base > limit {
		limit = base
	}
	if current >= limit/2 {
		return limit
	}
	return current * 2
}
