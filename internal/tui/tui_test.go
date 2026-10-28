package tui

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestKeysIgnorePastedCommands(t *testing.T) {
	keys := make(chan string, 32)
	readKeys(context.Background(), strings.NewReader("\x1b[200~wsxrqq\x1b[201~\x1b[A\x1b[Bw\tr\x03"), keys)
	var got []string
	for key := range keys {
		got = append(got, key)
	}
	if strings.Join(got, ",") != "up,down,watch,tab,refresh,quit" {
		t.Fatal(got)
	}
}

func TestViewIsBoundedAndSanitized(t *testing.T) {
	m := model{snapshot: Snapshot{Directory: "/项目/目录", Branch: "topic", Logs: "\x1b]52;secret\a\nhttps://token@example.com/repo\n"}, interval: 30 * time.Second, background: true}
	frame := m.view(80, 24)
	if strings.Contains(frame, "\x1b]52") || strings.Contains(frame, "https://token@") {
		t.Fatal("untrusted terminal escape or credentials reached screen")
	}
	if !strings.Contains(frame, "GSYNC") || !strings.Contains(frame, "后台日志") {
		t.Fatal(frame)
	}
	if got := clip("仓库abc", 5); got != "仓库a" {
		t.Fatal(got)
	}
	if frame := m.view(30, 10); !strings.Contains(frame, "窗口较小") {
		t.Fatal(frame)
	}
	m.appendLog(strings.Repeat("line\n", 600))
	if len(m.local) != 500 {
		t.Fatal("local logs are unbounded")
	}
}

func TestLayoutsFitTerminalCells(t *testing.T) {
	m := model{snapshot: Snapshot{Directory: "/工作区/项目/atlas-api", Branch: "feature/非常长的中文分支名称", Upstream: "refs/remotes/origin/main", Worktree: "干净", Known: true}, interval: 5 * time.Second, background: true, foreground: true}
	m.appendLog(strings.Repeat("远程更新包含中文路径与较长的诊断信息。", 8))
	ansi := regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	for _, size := range [][2]int{{58, 18}, {80, 24}, {100, 24}, {120, 18}, {120, 32}, {160, 40}} {
		frame := m.view(size[0], size[1])
		plain := ansi.ReplaceAllString(frame, "")
		rows := strings.Split(plain, "\r\n")
		if len(rows) != size[1]-1 {
			t.Fatalf("%v: got %d rows", size, len(rows))
		}
		for y, row := range rows {
			if n := textWidth(row); n != size[0]-1 {
				t.Fatalf("%v row %d occupies %d cells", size, y, n)
			}
		}
		if !strings.Contains(plain, "退出") {
			t.Fatalf("%v: exit action clipped", size)
		}
	}
}

func TestLongLogsWrapWithoutLosingText(t *testing.T) {
	line := "中文路径 /workspace/some-very-long-repository/file.go"
	entries := wrapLogs([]string{line}, 12)
	var joined strings.Builder
	for _, e := range entries {
		if textWidth(e.text) > 12 {
			t.Fatal("log exceeds viewport")
		}
		joined.WriteString(e.text)
	}
	if joined.String() != line || len(entries) < 2 {
		t.Fatal("wrapping lost log content")
	}
}

type fakeBackend struct {
	starts, stops atomic.Int32
	watchEnded    chan struct{}
}

func (b *fakeBackend) BackgroundAvailable() bool { return true }
func (b *fakeBackend) Snapshot(context.Context) (Snapshot, error) {
	return Snapshot{Directory: "/repo", Branch: "topic", Upstream: "origin/topic", Worktree: "干净", Service: "未运行", Known: true}, nil
}
func (b *fakeBackend) Watch(ctx context.Context, w io.Writer) error {
	_, _ = io.WriteString(w, "watch-live\n")
	<-ctx.Done()
	close(b.watchEnded)
	return ctx.Err()
}
func (b *fakeBackend) Start(context.Context) (string, error) {
	b.starts.Add(1)
	return "service-started", nil
}
func (b *fakeBackend) Stop(context.Context) (string, error) {
	b.stops.Add(1)
	return "service-stopped", nil
}

type screen struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}
func (s *screen) contains(text string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Contains(s.buf.String(), text)
}
func awaitScreen(t *testing.T, s *screen, text string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s.contains(text) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("screen never displayed %q", text)
}

func TestQuitStopsForegroundWithoutStoppingService(t *testing.T) {
	b := &fakeBackend{watchEnded: make(chan struct{})}
	keys := make(chan string, 8)
	out := &screen{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runLoop(ctx, b, time.Second, keys, out, func() (int, int) { return 100, 30 }) }()
	keys <- "watch"
	awaitScreen(t, out, "watch-live")
	keys <- "start"
	awaitScreen(t, out, "请先按 w 停止前台监听")
	if b.starts.Load() != 0 {
		t.Fatal("started background while foreground was active")
	}
	keys <- "quit"
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case <-b.watchEnded:
	default:
		t.Fatal("quit left foreground watcher running")
	}
	if b.stops.Load() != 0 {
		t.Fatal("quitting UI stopped background service")
	}
}

func TestBackgroundActionsAndQuit(t *testing.T) {
	b := &fakeBackend{watchEnded: make(chan struct{})}
	keys := make(chan string, 8)
	out := &screen{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runLoop(ctx, b, time.Second, keys, out, func() (int, int) { return 100, 30 }) }()
	keys <- "start"
	awaitScreen(t, out, "service-started")
	keys <- "stop"
	awaitScreen(t, out, "service-stopped")
	keys <- "quit"
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if b.starts.Load() != 1 || b.stops.Load() != 1 {
		t.Fatal("wrong background operation count")
	}
}
