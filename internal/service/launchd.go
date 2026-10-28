// Package service manages a per-repository macOS user LaunchAgent.
package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	git "gsync/internal/git"
	"gsync/internal/lock"
)

type Config struct {
	Directory         string
	Interval, Timeout time.Duration
	Verbose           bool
}

type Paths struct{ Label, Plist, Binary, Log string }

type Manager struct {
	home, executable, pathEnv, domain string
	command                           func(context.Context, ...string) (string, error)
}

func New(home string, uid int, executable, pathEnv string) *Manager {
	m := &Manager{home: home, executable: executable, pathEnv: pathEnv, domain: fmt.Sprintf("gui/%d", uid)}
	m.command = func(ctx context.Context, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		b, err := exec.CommandContext(ctx, "/bin/launchctl", args...).CombinedOutput()
		if err != nil {
			if ctx.Err() != nil {
				err = ctx.Err()
			}
			return "", fmt.Errorf("launchctl %s: %s: %w", strings.Join(args, " "), git.Redact(string(b)), err)
		}
		return string(b), nil
	}
	return m
}

func (m *Manager) Paths(common string) Paths {
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(common)))[:20]
	label := "io.gsync." + id
	return Paths{Label: label, Plist: filepath.Join(m.home, "Library", "LaunchAgents", label+".plist"), Binary: filepath.Join(m.home, "Library", "Application Support", "gsync", id, "gsync"), Log: filepath.Join(m.home, "Library", "Logs", "gsync", id+".log")}
}

func missing(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "Could not find service") || strings.Contains(err.Error(), "No such process"))
}

// Start installs a stable executable, so go run and rebuilding the original
// binary cannot invalidate the LaunchAgent. It never invokes a shell.
func (m *Manager) Start(ctx context.Context, common string, c Config) (bool, error) {
	management, err := lock.Acquire(filepath.Join(common, "gsync-service.lock"))
	if err != nil {
		return false, err
	}
	defer management.Close()
	p := m.Paths(common)
	if _, err := m.command(ctx, "print", m.domain+"/"+p.Label); err == nil {
		return false, nil
	} else if !missing(err) {
		return false, err
	}
	if c.Interval <= 0 || c.Timeout <= 0 || !filepath.IsAbs(c.Directory) {
		return false, fmt.Errorf("后台服务需要绝对仓库路径及正数时间参数")
	}
	// Reject a known foreground watcher before installing a service. The
	// service itself takes the same lock again when launchd starts it.
	repository, err := lock.Acquire(filepath.Join(common, "gsync.lock"))
	if err != nil {
		return false, err
	}
	repository.Close()
	for _, dir := range []string{filepath.Dir(p.Plist), filepath.Dir(p.Binary), filepath.Dir(p.Log)} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return false, err
		}
	}
	if err := copyExecutable(m.executable, p.Binary); err != nil {
		return false, err
	}
	old, err := os.ReadFile(p.Plist)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	existed := err == nil
	if err := writeAtomic(p.Plist, plist(p, c, m.pathEnv), 0600); err != nil {
		return false, err
	}
	if _, err := m.command(ctx, "bootstrap", m.domain, p.Plist); err != nil {
		// A timeout can happen after launchd accepted the job. Do not roll
		// back its manifest while silently leaving a running process behind.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, inspectErr := m.command(cleanupCtx, "print", m.domain+"/"+p.Label); inspectErr == nil {
			if _, stopErr := m.command(cleanupCtx, "bootout", m.domain+"/"+p.Label); stopErr != nil {
				return false, fmt.Errorf("%w；清理已加载服务失败: %v；已保留配置，请运行 gsync status / stop", err, stopErr)
			}
		} else if !missing(inspectErr) {
			return false, fmt.Errorf("%w；无法确认后台服务状态: %v；已保留配置，请运行 gsync status / stop", err, inspectErr)
		}
		var rollback error
		if existed {
			rollback = writeAtomic(p.Plist, old, 0600)
		} else {
			rollback = os.Remove(p.Plist)
		}
		if rollback != nil {
			return false, fmt.Errorf("%w；恢复启动配置失败: %v", err, rollback)
		}
		return false, err
	}
	return true, nil
}

// Stop also removes the login-start configuration; binaries and logs remain.
func (m *Manager) Stop(ctx context.Context, common string) error {
	management, err := lock.Acquire(filepath.Join(common, "gsync-service.lock"))
	if err != nil {
		return err
	}
	defer management.Close()
	p := m.Paths(common)
	_, err = m.command(ctx, "print", m.domain+"/"+p.Label)
	if err == nil {
		if _, err = m.command(ctx, "bootout", m.domain+"/"+p.Label); err != nil {
			return err
		}
	} else if !missing(err) {
		return err
	}
	if err := os.Remove(p.Plist); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (m *Manager) Status(ctx context.Context, common string) (string, error) {
	p := m.Paths(common)
	s, err := m.command(ctx, "print", m.domain+"/"+p.Label)
	if err != nil {
		if !missing(err) {
			return "", err
		}
		if _, err := os.Stat(p.Plist); err == nil {
			return "未运行（已配置登录启动）", nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		return "未运行（未配置登录启动）", nil
	}
	var details []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		for _, key := range []string{"state = ", "pid = ", "last exit code = "} {
			if strings.HasPrefix(line, key) {
				details = append(details, line)
				break
			}
		}
	}
	return "已加载：" + strings.Join(details, "，"), nil
}

// Logs reads a bounded tail, rather than loading a long-running log into RAM.
func (m *Manager) Logs(common string, lines int) (string, error) {
	if lines <= 0 || lines > 10000 {
		return "", fmt.Errorf("日志行数必须为 1 到 10000")
	}
	f, err := os.Open(m.Paths(common).Log)
	if os.IsNotExist(err) {
		return "暂无日志\n", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	offset := info.Size() - 64*1024
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", err
	}
	b, err := io.ReadAll(io.LimitReader(f, 64*1024))
	if err != nil {
		return "", err
	}
	if offset > 0 {
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	s := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(s) > lines {
		s = s[len(s)-lines:]
	}
	return git.Redact(strings.Join(s, "\n")) + "\n", nil
}

func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func plist(p Paths, c Config, pathEnv string) []byte {
	args := []string{p.Binary, "watch", "--interval", c.Interval.String(), "--timeout", c.Timeout.String()}
	if c.Verbose {
		args = append(args, "--verbose")
	}
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict>\n")
	fmt.Fprintf(&b, "<key>Label</key><string>%s</string>\n<key>ProgramArguments</key><array>\n", escape(p.Label))
	for _, arg := range args {
		fmt.Fprintf(&b, "<string>%s</string>\n", escape(arg))
	}
	fmt.Fprintf(&b, "</array>\n<key>WorkingDirectory</key><string>%s</string>\n", escape(c.Directory))
	fmt.Fprintf(&b, "<key>StandardOutPath</key><string>%s</string>\n<key>StandardErrorPath</key><string>%s</string>\n", escape(p.Log), escape(p.Log))
	fmt.Fprintf(&b, "<key>EnvironmentVariables</key><dict><key>PATH</key><string>%s</string><key>GIT_TERMINAL_PROMPT</key><string>0</string></dict>\n", escape(pathEnv))
	b.WriteString("<key>RunAtLoad</key><true/>\n<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>\n<key>ThrottleInterval</key><integer>30</integer>\n</dict></plist>\n")
	return []byte(b.String())
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".gsync-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func copyExecutable(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("可执行文件不是普通文件")
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	return writeAtomic(dst, data, 0755)
}
