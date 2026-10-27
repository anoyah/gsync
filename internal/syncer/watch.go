package syncer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// Watch polls both local and remote state. Each interval starts after the
// previous cycle finishes, so slow fetches cannot overlap subsequent syncs.
func Watch(ctx context.Context, dir string, o Options, interval time.Duration, out, errOut io.Writer) error {
	// Watching is pull-only, regardless of the options of a one-shot engine.
	o.NoPush = true
	o.NoRebase = true
	return watch(ctx, interval, o.Verbose, out, errOut, func(ctx context.Context, w io.Writer) error {
		return Run(ctx, dir, o, w)
	})
}

func watch(ctx context.Context, interval time.Duration, verbose bool, out, errOut io.Writer, cycle func(context.Context, io.Writer) error) error {
	if interval <= 0 {
		return failure(2, fmt.Errorf("监听间隔必须大于 0"))
	}
	fmt.Fprintf(out, "正在持续监听，每轮完成后等待 %s；Ctrl+C 停止。\n", interval)
	last := ""
	for {
		if ctx.Err() != nil {
			return failure(9, ctx.Err())
		}
		var buf bytes.Buffer
		err := cycle(ctx, &buf)
		if ctx.Err() != nil {
			// Preserve recovery diagnostics if interrupted during a rebase.
			if err != nil {
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
				fmt.Fprintln(errOut, "暂停本轮同步：", err)
			}
			if err == nil && len(last) > 6 && last[:6] == "error:" {
				fmt.Fprintln(out, "仓库已恢复，继续自动同步。")
			}
		}
		last = key
		if err != nil {
			var syncErr *Error
			if !errors.As(err, &syncErr) || syncErr.Code == 1 || syncErr.Code == 3 || syncErr.Code == 7 {
				return fmt.Errorf("监听已停止；请处理错误后重新启动 gsync: %w", err)
			}
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return failure(9, ctx.Err())
		case <-timer.C:
		}
	}
}
