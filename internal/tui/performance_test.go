package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// A full retained log buffer is common after a long-running watch session.
func BenchmarkDashboard500Logs(b *testing.B) {
	m := model{snapshot: Snapshot{Directory: "/workspace/gsync", Branch: "main", Upstream: "origin/main", Worktree: "干净", Known: true}, foreground: true, interval: 5 * time.Second}
	for i := 0; i < 500; i++ {
		m.appendLog(fmt.Sprintf("[%03d] 检查完成，工作区干净，等待下次更新 /workspace/repository/very-long-path/file.go", i))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.view(120, 32)
	}
}

func TestLogLayoutCacheInvalidation(t *testing.T) {
	m := model{}
	m.appendLog("first\n")
	a := m.layoutLogs(m.local, 20, false)
	m.appendLog("second\n")
	b := m.layoutLogs(m.local, 20, false)
	if len(a) != 1 || len(b) != 2 {
		t.Fatal("new local logs did not invalidate cache")
	}
	if len(m.layoutLogs(m.local, 3, false)) <= len(b) {
		t.Fatal("resize did not reflow logs")
	}
	m.snapshot.Logs = "background\n"
	if m.layoutLogs([]string{"background"}, 20, true)[0].text != "background" {
		t.Fatal("source switch did not invalidate cache")
	}
	m.snapshot.Logs = "changed\n"
	if m.layoutLogs([]string{"changed"}, 20, true)[0].text != "changed" {
		t.Fatal("updated background log did not invalidate cache")
	}
}

func TestFrameDiffUpdatesOnlyChangedRows(t *testing.T) {
	m := model{snapshot: Snapshot{Directory: "/repo", Branch: "main", Worktree: "干净"}, interval: time.Second}
	a := m.view(120, 32)
	if frameDiff(a, a) != "" {
		t.Fatal("unchanged view was repainted")
	}
	m.notice = "new notification"
	b := m.view(120, 32)
	patch := frameDiff(a, b)
	if !strings.Contains(patch, "new notification") || strings.Contains(patch, "仓库概览") || len(patch) >= len(b)/2 {
		t.Fatal("notification repainted the dashboard")
	}
	if frameDiff(a, m.view(80, 24)) != m.view(80, 24) {
		t.Fatal("resize should draw a complete frame")
	}
}
