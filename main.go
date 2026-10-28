package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	git "gsync/internal/git"
	"gsync/internal/lock"
	"gsync/internal/service"
	"gsync/internal/syncer"
	"gsync/internal/tui"
)

var version = "dev"

func run(ctx context.Context, args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(out, menuText)
		return 0
	}
	command := ""
	if len(args) > 0 {
		switch args[0] {
		case "watch", "tui", "start", "stop", "status", "logs":
			command = args[0]
			args = args[1:]
		}
	}
	f := flag.NewFlagSet("gsync", flag.ContinueOnError)
	f.SetOutput(errOut)
	o := syncer.Options{NoPush: true, NoRebase: true}
	var showVersion bool
	var once bool
	var interval time.Duration
	var repo string
	var lines int
	f.StringVar(&repo, "repo", "", "仓库路径，默认当前目录")
	if command == "logs" {
		f.IntVar(&lines, "lines", 100, "显示最后 N 行日志（最多读取 64 KiB）")
	}
	f.BoolVar(&once, "once", false, "只同步一次后退出")
	f.DurationVar(&interval, "interval", 30*time.Second, "持续监听时，每轮完成后的等待间隔")
	f.BoolVar(&o.DryRun, "dry-run", false, "Fetch 并展示计划；会更新远程跟踪引用，不整合或推送")
	f.BoolVar(&o.Verbose, "verbose", false, "输出 Git 命令和诊断")
	f.BoolVar(&o.Verbose, "v", false, "同 --verbose")
	f.BoolVar(&showVersion, "version", false, "显示版本")
	f.DurationVar(&o.Timeout, "timeout", 2*time.Minute, "每条 Git 命令超时")
	f.Usage = func() {
		fmt.Fprintln(errOut, "gsync · Git 自动拉取\n用法：gsync 显示操作菜单，或 gsync <watch|tui|start|stop|status|logs> [参数]\nwatch 持续监听并自动拉取，Ctrl+C 停止；tui 打开终端交互面板。\n--once 仅运行一轮，--dry-run 仅预览一轮；保留仅带监听参数的旧用法。\nmacOS: start 安装后台服务并配置登录启动；stop 停止并取消登录启动。\nstatus 查看后台状态；logs 查看日志；--repo 指定仓库。\n只允许 Fast-forward，不提交、不推送、不 Rebase。工作区有修改或分叉时等待处理。")
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
	if command != "" && (once || o.DryRun || showVersion) {
		fmt.Fprintln(errOut, "参数错误：子命令不接受 --once、--dry-run 或 --version")
		return 2
	}
	if command == "logs" && (lines <= 0 || lines > 10000) {
		fmt.Fprintln(errOut, "参数错误：--lines 必须为 1 到 10000")
		return 2
	}
	invalid := false
	if command == "stop" || command == "status" || command == "logs" {
		f.Visit(func(p *flag.Flag) {
			switch p.Name {
			case "interval", "timeout", "verbose", "v":
				invalid = true
			}
		})
	}
	if invalid {
		fmt.Fprintln(errOut, "参数错误：监听参数只用于前台运行或 start")
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
	if repo != "" {
		dir, err = filepath.Abs(repo)
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
	}
	if command == "tui" {
		file, ok := out.(*os.File)
		if !ok {
			err = fmt.Errorf("TUI 需要交互式终端输出")
		} else {
			var backend tui.Backend
			backend, err = tui.NewBackend(ctx, dir, o, interval)
			if err == nil {
				err = tui.Run(ctx, backend, interval, os.Stdin, file)
			}
		}
	} else if command != "" && command != "watch" {
		err = manageService(ctx, command, dir, o, interval, lines, out)
	} else if once || o.DryRun {
		err = syncer.Run(ctx, dir, o, out)
	} else {
		err = syncer.Watch(ctx, dir, o, interval, out, errOut)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(errOut, "操作已停止；如在拉取中中断，请运行 git status 检查状态。")
			return 9
		}
		fmt.Fprintln(errOut, "✗", err)
		if errors.Is(err, context.DeadlineExceeded) {
			return 9
		}
		if errors.Is(err, lock.ErrBusy) {
			return 4
		}
		var e *syncer.Error
		if errors.As(err, &e) {
			return e.Code
		}
		return 1
	}
	return 0
}

func manageService(ctx context.Context, command, dir string, o syncer.Options, interval time.Duration, lines int, out io.Writer) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("后台服务管理目前仅支持 macOS；当前平台可前台运行 gsync")
	}
	r := git.Repository{Runner: git.Runner{Dir: dir, Timeout: o.Timeout}}
	if _, err := r.Run(ctx, "--version"); err != nil {
		return &syncer.Error{Code: 1, Err: err}
	}
	if err := r.Discover(ctx); err != nil {
		return &syncer.Error{Code: 3, Err: err}
	}
	common, err := r.CommonDir(ctx)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	m := service.New(home, os.Getuid(), executable, os.Getenv("PATH"))
	p := m.Paths(common)
	switch command {
	case "start":
		started, err := m.Start(ctx, common, service.Config{Directory: r.Root, Interval: interval, Timeout: o.Timeout, Verbose: o.Verbose})
		if err != nil {
			return err
		}
		if started {
			fmt.Fprintln(out, "后台服务已加载，将在登录后自动启动。请用 gsync status 检查运行状态。宿主系统休眠或未登录时不会同步。")
		} else {
			fmt.Fprintln(out, "后台服务已加载；如需更改配置，请先 stop 再 start。")
		}
		fmt.Fprintln(out, "日志：", p.Log)
	case "stop":
		if err := m.Stop(ctx, common); err != nil {
			return err
		}
		fmt.Fprintln(out, "后台服务已停止，已取消登录启动；日志和服务程序副本保留。")
	case "status":
		s, err := m.Status(ctx, common)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, s)
		fmt.Fprintln(out, "配置：", p.Plist)
		fmt.Fprintln(out, "日志：", p.Log)
	case "logs":
		s, err := m.Logs(common, lines)
		if err != nil {
			return err
		}
		fmt.Fprint(out, s)
	}
	return nil
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
