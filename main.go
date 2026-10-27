package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gsync/internal/syncer"
)

var version = "dev"

func run(ctx context.Context, args []string, out, errOut io.Writer) int {
	f := flag.NewFlagSet("gsync", flag.ContinueOnError)
	f.SetOutput(errOut)
	var o syncer.Options
	var showVersion bool
	f.BoolVar(&o.DryRun, "dry-run", false, "Fetch 并展示计划；会更新远程跟踪引用，不整合或推送")
	f.BoolVar(&o.NoPush, "no-push", false, "整合远程更新但不推送")
	f.BoolVar(&o.NoRebase, "no-rebase", false, "分叉时停止；默认 Rebase 会重写本地未推送提交 SHA")
	f.BoolVar(&o.Verbose, "verbose", false, "输出 Git 命令和诊断")
	f.BoolVar(&o.Verbose, "v", false, "同 --verbose")
	f.BoolVar(&showVersion, "version", false, "显示版本")
	f.DurationVar(&o.Timeout, "timeout", 2*time.Minute, "每条 Git 命令超时")
	f.Usage = func() {
		fmt.Fprintln(errOut, "gsync · Git 自动同步\n用法：gsync [参数]\n默认分叉时 Rebase，会重写本地未推送提交 SHA。\n请勿同时对同一仓库执行同步。")
		f.PrintDefaults()
	}
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if f.NArg() != 0 || o.Timeout <= 0 {
		fmt.Fprintln(errOut, "参数错误：不接受位置参数，--timeout 必须大于 0")
		return 2
	}
	if showVersion {
		fmt.Fprintln(out, "gsync", version)
		return 0
	}
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if err = syncer.Run(ctx, dir, o, out); err != nil {
		fmt.Fprintln(errOut, "✗", err)
		var e *syncer.Error
		if errors.As(err, &e) {
			return e.Code
		}
		return 1
	}
	return 0
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
