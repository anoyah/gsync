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
	o := syncer.Options{NoPush: true, NoRebase: true}
	var showVersion bool
	var once bool
	var interval time.Duration
	f.BoolVar(&once, "once", false, "只同步一次后退出（默认持续监听）")
	f.DurationVar(&interval, "interval", 30*time.Second, "持续监听时，每轮完成后的等待间隔")
	f.BoolVar(&o.DryRun, "dry-run", false, "Fetch 并展示计划；会更新远程跟踪引用，不整合或推送")
	f.BoolVar(&o.Verbose, "verbose", false, "输出 Git 命令和诊断")
	f.BoolVar(&o.Verbose, "v", false, "同 --verbose")
	f.BoolVar(&showVersion, "version", false, "显示版本")
	f.DurationVar(&o.Timeout, "timeout", 2*time.Minute, "每条 Git 命令超时")
	f.Usage = func() {
		fmt.Fprintln(errOut, "gsync · Git 自动拉取\n用法：gsync [参数]\n默认持续轮询监听远程，Ctrl+C 停止；--once 仅运行一轮。\n只允许 Fast-forward，不提交、不推送、不 Rebase；工作区有修改或分叉时等待处理。\n--dry-run 仅预览一轮。请勿同时对同一仓库执行同步。")
		f.PrintDefaults()
	}
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if f.NArg() != 0 || o.Timeout <= 0 || interval <= 0 {
		fmt.Fprintln(errOut, "参数错误：不接受位置参数，--timeout 和 --interval 必须大于 0")
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
	if once || o.DryRun {
		err = syncer.Run(ctx, dir, o, out)
	} else {
		err = syncer.Watch(ctx, dir, o, interval, out, errOut)
	}
	if err != nil {
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
