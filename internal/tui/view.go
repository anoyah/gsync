package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	git "github.com/anoyah/gsync/internal/git"
)

type model struct {
	snapshot                   Snapshot
	local                      []string
	foreground, stopping, busy bool
	notice                     string
	offset                     int
	interval                   time.Duration
	background                 bool
	backgroundLog              bool
	spin                       int
	logRevision                uint64
	logCache                   logLayout
}

// Never allow Git output, file names, or log lines to inject terminal controls.
func safe(s string) string {
	s = git.Redact(s)
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if r == '\n' {
			return r
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
}

func cellWidth(r rune) int {
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
		return 0
	}
	if r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a || r >= 0x2e80 && r <= 0xa4cf || r >= 0xac00 && r <= 0xd7a3 || r >= 0xf900 && r <= 0xfaff || r >= 0xfe10 && r <= 0xfe6f || r >= 0xff00 && r <= 0xff60 || r >= 0xffe0 && r <= 0xffe6 || r >= 0x1f300 && r <= 0x1faff || r >= 0x20000 && r <= 0x3ffff) {
		return 2
	}
	return 1
}

func clip(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = strings.ReplaceAll(safe(s), "\n", " ")
	var b strings.Builder
	n := 0
	for _, r := range s {
		w := cellWidth(r)
		if n+w > width {
			break
		}
		b.WriteRune(r)
		n += w
	}
	return b.String()
}

func (m *model) appendLog(s string) {
	m.logRevision++
	for _, line := range strings.Split(strings.TrimSuffix(safe(s), "\n"), "\n") {
		if line != "" {
			// Keep individual diagnostics bounded as well as the line count.
			if len(line) > 8192 {
				line = clip(line, 4096) + "…"
			}
			m.local = append(m.local, line)
		}
	}
	if len(m.local) > 500 {
		m.local = m.local[len(m.local)-500:]
	}
}

func (m *model) view(width, height int) string {
	c := newCanvas(width-1, height-1)
	if c.width < 10 || c.height < 4 {
		c.put(0, 0, "GSYNC · q 退出", muted, c.width)
		return c.render()
	}
	c.put(2, 0, "GSYNC", bright, c.width-4)
	if width < 58 || height < 18 {
		c.put(9, 0, "/ Git 自动拉取", muted, c.width-11)
		c.put(2, 3, "终端窗口较小", accent, c.width-4)
		c.put(2, 5, "建议至少 58 列 × 18 行", muted, c.width-4)
		c.put(2, 7, "w 前台启停 · r 刷新 · q 退出", text, c.width-4)
		return c.render()
	}
	s := m.snapshot
	c.put(10, 0, "/ Git 自动拉取", muted, c.width-24)
	badge, role := m.badge()
	c.right(c.width-2, 0, badge, role)
	name := "正在读取仓库…"
	if s.Directory != "" {
		name = filepath.Base(s.Directory)
	}
	c.put(2, 1, name, bright, c.width-20)
	c.right(c.width-2, 1, "PULL ONLY", faint)
	pathWidth := c.width - 4
	if !s.Updated.IsZero() {
		pathWidth -= 19
		c.right(c.width-2, 2, "刷新 "+s.Updated.Format("15:04:05"), faint)
	}
	c.put(2, 2, tail(s.Directory, pathWidth), muted, pathWidth)
	c.rule(2, 3, c.width-4, faint)
	logX, logTop, logWidth := 2, 9, c.width-4
	if width >= 100 && height >= 24 {
		side := 28
		if width >= 140 {
			side = 32
		}
		divider := side + 4
		logX = divider + 3
		logTop = 5
		logWidth = c.width - logX - 2
		for y := 5; y <= c.height-5; y++ {
			c.put(divider, y, "│", faint, 1)
		}
		m.sidebar(c, side)
	} else {
		branch := s.Branch
		if branch == "" {
			branch = "等待读取"
		}
		x := c.put(2, 5, branch, bright, c.width/2-2)
		c.put(x+2, 5, "→  "+shortUpstream(s.Upstream), muted, c.width-x-4)
		status, statusTone := worktree(s.Worktree)
		x = c.put(2, 6, "● "+status, statusTone, c.width/2)
		c.put(x+3, 6, m.difference(), text, c.width-x-5)
		front, frontTone := m.frontStatus()
		x = c.put(2, 7, "前台  "+front, frontTone, 20)
		back, _ := m.backStatus()
		c.put(x+3, 7, "后台  "+back+"   ·   "+m.interval.String(), muted, c.width-x-5)
	}
	m.logPane(c, logX, logTop, logWidth)
	m.footer(c)
	return c.render()
}

func (m model) badge() (string, tone) {
	if m.busy {
		return []string{"◴", "◷", "◶", "◵"}[m.spin%4] + " 操作处理中", accent
	}
	if m.stopping {
		return "● 正在停止", warning
	}
	if m.foreground {
		if m.snapshot.Ahead > 0 && m.snapshot.Behind > 0 || m.snapshot.Worktree != "" && m.snapshot.Worktree != "干净" {
			return "● 等待处理", warning
		}
		return "● 前台监听", success
	}
	if strings.Contains(m.snapshot.Service, "state = running") {
		return "● 后台运行", success
	}
	return "○ 待命", muted
}

func (m model) frontStatus() (string, tone) {
	if m.stopping {
		return "正在停止", warning
	}
	if m.foreground {
		return "监听中", success
	}
	return "未启动", muted
}
func (m model) backStatus() (string, tone) {
	s := m.snapshot.Service
	if !m.background {
		return "仅 macOS", faint
	}
	if strings.Contains(s, "失败") {
		return "状态不可用", warning
	}
	if strings.Contains(s, "state = running") {
		return "运行中", success
	}
	if strings.HasPrefix(s, "已加载") {
		return "已加载", accent
	}
	if s == "" {
		return "读取中", faint
	}
	return "未运行", muted
}
func worktree(s string) (string, tone) {
	if s == "干净" {
		return "工作区干净", success
	}
	if s == "" {
		return "等待读取工作区", muted
	}
	return s, warning
}
func shortUpstream(s string) string {
	if s == "" {
		return "未配置上游"
	}
	return strings.TrimPrefix(s, "refs/remotes/")
}
func (m model) difference() string {
	if !m.snapshot.Known {
		return "提交差异待读取"
	}
	return fmt.Sprintf("本地 ↑%d   远程 ↓%d", m.snapshot.Ahead, m.snapshot.Behind)
}
func (m model) sidebar(c *canvas, width int) {
	s := m.snapshot
	c.put(2, 5, "仓库概览", bright, width)
	c.put(2, 7, "分支", muted, 4)
	c.put(8, 7, s.Branch, text, width-6)
	c.put(2, 8, "上游", muted, 4)
	c.put(8, 8, shortUpstream(s.Upstream), text, width-6)
	status, role := worktree(s.Worktree)
	c.put(2, 10, "● "+status, role, width)
	c.put(2, 11, m.difference(), text, width)
	c.put(2, 13, "运行状态", bright, width)
	front, role := m.frontStatus()
	c.put(2, 14, "前台", muted, 4)
	c.put(8, 14, front, role, width-6)
	back, role := m.backStatus()
	c.put(2, 15, "后台", muted, 4)
	c.put(8, 15, back, role, width-6)
	c.put(2, 16, "间隔", muted, 4)
	c.put(8, 16, m.interval.String(), text, width-6)
	if c.height >= 27 {
		c.put(2, 19, "安全策略", bright, width)
		c.put(2, 20, "只拉取 · Fast-forward", muted, width)
		c.put(2, 21, "不提交 / 不推送 / 不 Rebase", faint, width)
	}
}

func (m *model) logPane(c *canvas, x, y, width int) {
	source := m.backgroundLog || !m.foreground && len(m.local) == 0
	a, b := selected, muted
	if source {
		a, b = b, a
	}
	first := c.put(x, y, " 前台日志 ", a, 10)
	c.put(first+2, y, " 后台日志 ", b, 10)
	if width >= 48 {
		c.right(x+width, y, "Tab 切换", faint)
	}
	c.rule(x, y+1, width, faint)
	activeX := x
	if source {
		activeX = first + 2
	}
	c.rule(activeX, y+1, 10, accent)
	logs := m.local
	if source {
		logs = strings.Split(strings.TrimSuffix(safe(m.snapshot.Logs), "\n"), "\n")
	}
	available := c.height - 5 - (y + 2)
	if available < 1 {
		return
	}
	empty := len(logs) == 0 || len(logs) == 1 && (logs[0] == "" || logs[0] == "暂无日志")
	if empty {
		center := y + 2 + available/2
		c.center(x, width, center-1, "等待第一次同步", text)
		hint := "按 w 开始前台监听"
		if source && m.background {
			hint = "按 s 启动后台服务"
		}
		c.center(x, width, center+1, hint, muted)
		c.right(x+width, c.height-5, "暂无记录", faint)
		return
	}
	entries := m.layoutLogs(logs, width-6, source)
	offset := m.offset
	max := len(entries) - available
	if max < 0 {
		max = 0
	}
	if offset > max {
		offset = max
	}
	end := len(entries) - offset
	start := end - available
	if start < 0 {
		start = 0
	}
	for row, e := range entries[start:end] {
		label := fmt.Sprintf("%3d", e.number)
		if e.continued {
			label = "  ·"
		}
		c.put(x, y+2+row, label, faint, 4)
		c.put(x+6, y+2+row, e.text, logTone(e.text), width-6)
	}
	c.put(x, c.height-5, fmt.Sprintf("%d–%d / %d 行", start+1, end, len(entries)), faint, width/2)
	follow := "跟随最新"
	role := muted
	if offset > 0 {
		follow = "浏览历史 ↑" + fmt.Sprint(offset)
		role = accent
	}
	c.right(x+width, c.height-5, follow, role)
}
func (m model) footer(c *canvas) {
	notice := m.notice
	role := muted
	if m.snapshot.Detail != "" {
		notice = m.snapshot.Detail
		role = warning
	}
	if strings.Contains(m.snapshot.Service, "失败") {
		notice = m.snapshot.Service
		role = warning
	}
	if m.notice != "" && (strings.Contains(m.notice, "失败") || strings.HasPrefix(m.notice, "请先")) {
		notice = m.notice
		role = warning
	}
	if m.busy {
		notice = m.notice
		role = accent
	}
	if notice == "" {
		notice = "只拉取 · 不提交 · 不推送 · 不 Rebase"
	}
	c.put(2, c.height-3, "›", accent, 1)
	c.put(4, c.height-3, notice, role, c.width-6)
	x := 2
	limit := c.width - 12
	label := "开始监听"
	if m.foreground {
		label = "停止监听"
	}
	x = c.shortcut(x, c.height-2, "w", label, accent, limit)
	if m.background {
		x = c.shortcut(x, c.height-2, "s", "后台启动", text, limit)
		c.shortcut(x, c.height-2, "x", "后台停止", text, limit)
	}
	c.shortcut(c.width-10, c.height-2, "q", "退出", text, c.width)
	c.put(2, c.height-1, "r 刷新  Tab 日志源  ↑↓ 滚动  G 最新", faint, c.width-4)
	if c.width >= 100 {
		c.right(c.width-2, c.height-1, "退出不影响后台服务", muted)
	}
}

type logEntry struct {
	text      string
	number    int
	continued bool
}

type logLayout struct {
	width      int
	background bool
	revision   uint64
	content    string
	entries    []logEntry
}

func (m *model) layoutLogs(logs []string, width int, background bool) []logEntry {
	c := &m.logCache
	content := ""
	if background {
		content = m.snapshot.Logs
	}
	if c.entries == nil || c.width != width || c.background != background || background && c.content != content || !background && c.revision != m.logRevision {
		*c = logLayout{width: width, background: background, revision: m.logRevision, content: content, entries: wrapLogs(logs, width)}
	}
	return c.entries
}

func wrapLogs(lines []string, width int) []logEntry {
	var entries []logEntry
	if width < 1 {
		return entries
	}
	for i, line := range lines {
		line = safe(line)
		var b strings.Builder
		cells := 0
		continued := false
		for _, r := range line {
			w := cellWidth(r)
			if cells+w > width && b.Len() > 0 {
				entries = append(entries, logEntry{b.String(), i + 1, continued})
				b.Reset()
				cells = 0
				continued = true
			}
			b.WriteRune(r)
			cells += w
		}
		entries = append(entries, logEntry{b.String(), i + 1, continued})
	}
	return entries
}
func logTone(s string) tone {
	switch {
	case strings.HasPrefix(s, "正在持续监听"):
		return accent
	case strings.Contains(s, "失败") || strings.Contains(s, "fatal:") || strings.Contains(s, "error:"):
		return danger
	case strings.Contains(s, "暂停") || strings.Contains(s, "警告"):
		return warning
	case strings.Contains(s, "完成") || strings.Contains(s, "成功") || strings.Contains(s, "恢复"):
		return success
	case strings.HasPrefix(s, "[20") || strings.HasPrefix(s, "  git "):
		return muted
	default:
		return text
	}
}
