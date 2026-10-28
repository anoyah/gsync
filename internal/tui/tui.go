// Package tui provides a keyboard-driven dashboard without changing CLI modes.
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

type event struct {
	kind, text string
	snapshot   Snapshot
	err        error
}

type eventWriter struct {
	ctx    context.Context
	events chan<- event
}

func (w eventWriter) Write(p []byte) (int, error) {
	select {
	case w.events <- event{kind: "log", text: string(p)}:
	case <-w.ctx.Done():
	}
	return len(p), nil
}

func Run(ctx context.Context, b Backend, interval time.Duration, input, output *os.File) error {
	if !term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return fmt.Errorf("TUI 需要交互式终端；脚本请使用 --once 或普通监听模式")
	}
	reader, err := duplicateInput(input)
	if err != nil {
		return err
	}
	defer reader.Close()
	state, err := term.MakeRaw(int(reader.Fd()))
	if err != nil {
		return err
	}
	defer term.Restore(int(reader.Fd()), state)
	restore, err := prepareOutput(output)
	if err != nil {
		return err
	}
	defer restore()
	if _, err := fmt.Fprint(output, "\x1b[?1049h\x1b[?25l\x1b[?2004h\x1b[2J"); err != nil {
		return err
	}
	defer fmt.Fprint(output, "\x1b[0m\x1b[?2004l\x1b[?25h\x1b[?1049l")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	keys := make(chan string, 16)
	go readKeys(ctx, reader, keys)
	size := func() (int, int) {
		w, h, err := term.GetSize(int(output.Fd()))
		if err != nil {
			return 80, 24
		}
		return w, h
	}
	return runLoop(ctx, b, interval, keys, output, size)
}

func runLoop(parent context.Context, b Backend, interval time.Duration, keys <-chan string, out io.Writer, size func() (int, int)) error {
	ctx, cancel := context.WithCancel(parent)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	events := make(chan event, 64)
	send := func(e event) {
		select {
		case events <- e:
		case <-ctx.Done():
		}
	}
	m := model{interval: interval, background: b.BackgroundAvailable(), notice: "按 w 开始前台监听；打开界面不会自动安装后台服务。"}
	refreshing := false
	refresh := func() {
		if refreshing {
			return
		}
		refreshing = true
		workers.Add(1)
		go func() {
			defer workers.Done()
			s, err := b.Snapshot(ctx)
			send(event{kind: "snapshot", snapshot: s, err: err})
		}()
	}
	var stopWatch context.CancelFunc
	action := func(kind string) {
		m.busy = true
		m.notice = "正在处理后台服务"
		workers.Add(1)
		go func() {
			defer workers.Done()
			var text string
			var err error
			if kind == "start" {
				text, err = b.Start(ctx)
			} else {
				text, err = b.Stop(ctx)
			}
			send(event{kind: kind, text: text, err: err})
		}()
	}
	refresh()
	clock := time.NewTicker(250 * time.Millisecond)
	defer clock.Stop()
	poll := time.NewTicker(2 * time.Second)
	defer poll.Stop()
	lastFrame := ""
	dirty := true
	lastWidth, lastHeight := 0, 0
	for {
		width, height := size()
		if dirty || width != lastWidth || height != lastHeight {
			frame := m.view(width, height)
			previous := lastFrame
			if width != lastWidth || height != lastHeight {
				previous = ""
			}
			if patch := frameDiff(previous, frame); patch != "" {
				if _, err := io.WriteString(out, patch); err != nil {
					return err
				}
			}
			lastFrame = frame
			lastWidth, lastHeight = width, height
			dirty = false
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-clock.C:
			if m.busy {
				m.spin++
				dirty = true
			}
		case <-poll.C:
			refresh()
		case key, ok := <-keys:
			dirty = true
			if !ok || key == "quit" {
				m.notice = "正在退出，等待任务安全结束…"
				w, h := size()
				_, _ = io.WriteString(out, m.view(w, h))
				return nil
			}
			switch key {
			case "refresh":
				refresh()
			case "up":
				m.offset++
			case "down":
				if m.offset > 0 {
					m.offset--
				}
			case "latest":
				m.offset = 0
			case "tab":
				m.backgroundLog = !m.backgroundLog
				m.offset = 0
			case "watch":
				if m.busy {
					m.notice = "请等待后台操作完成"
					continue
				}
				if m.stopping {
					continue
				}
				if m.foreground {
					m.stopping = true
					m.notice = "正在停止前台监听"
					stopWatch()
					continue
				}
				watchCtx, stop := context.WithCancel(ctx)
				stopWatch = stop
				m.foreground = true
				m.backgroundLog = false
				m.offset = 0
				m.notice = "前台监听已启动，退出界面会停止前台监听"
				workers.Add(1)
				go func() {
					defer workers.Done()
					err := b.Watch(watchCtx, eventWriter{ctx: ctx, events: events})
					send(event{kind: "watchStopped", err: err})
				}()
			case "start":
				if m.busy {
					continue
				}
				if m.foreground {
					m.notice = "请先按 w 停止前台监听，再启动后台服务"
					continue
				}
				action("start")
			case "stop":
				if !m.busy {
					action("stop")
				}
			}
		case e := <-events:
			dirty = true
			switch e.kind {
			case "snapshot":
				refreshing = false
				if e.err != nil {
					m.notice = "刷新失败：" + e.err.Error()
				} else {
					m.snapshot = e.snapshot
					if strings.HasPrefix(m.notice, "刷新失败：") {
						m.notice = "状态读取已恢复"
					}
				}
			case "log":
				m.appendLog(e.text)
			case "watchStopped":
				m.foreground = false
				m.stopping = false
				stopWatch = nil
				if e.err != nil && !errors.Is(e.err, context.Canceled) {
					m.notice = "前台监听已停止：" + e.err.Error()
					m.appendLog(m.notice)
				} else {
					m.notice = "前台监听已停止"
				}
				refresh()
			case "start", "stop":
				m.busy = false
				if e.err != nil {
					m.notice = "操作失败：" + e.err.Error()
					m.appendLog(m.notice)
					m.backgroundLog = false
					m.offset = 0
				} else {
					m.notice = e.text
					if e.kind == "start" {
						m.backgroundLog = true
						m.offset = 0
					}
				}
				refresh()
			}
		}
	}
}
