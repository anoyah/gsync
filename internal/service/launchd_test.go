package service

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/anoyah/gsync/internal/lock"
)

func setupManager(t *testing.T) (*Manager, string, Config) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home & 中文")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(home, "source binary")
	if err := os.WriteFile(binary, []byte("test executable"), 0700); err != nil {
		t.Fatal(err)
	}
	common := t.TempDir()
	m := New(home, 501, binary, "/usr/bin:/a & b/bin")
	return m, common, Config{Directory: home, Interval: 5 * time.Second, Timeout: time.Minute, Verbose: true}
}

func absent(context.Context, ...string) (string, error) {
	return "", errors.New("Could not find service")
}

func TestStartStopLifecycle(t *testing.T) {
	m, common, c := setupManager(t)
	p := m.Paths(common)
	loaded := false
	var calls [][]string
	m.command = func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, args)
		switch args[0] {
		case "print":
			if !loaded {
				return "", errors.New("Could not find service")
			}
			return "state = running\npid = 42\nlast exit code = 0", nil
		case "bootstrap":
			if !reflect.DeepEqual(args, []string{"bootstrap", "gui/501", p.Plist}) {
				t.Fatal(args)
			}
			loaded = true
		case "bootout":
			loaded = false
		default:
			t.Fatal(args)
		}
		return "", nil
	}
	started, err := m.Start(context.Background(), common, c)
	if err != nil || !started {
		t.Fatalf("start=%v err=%v", started, err)
	}
	data, err := os.ReadFile(p.Plist)
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var text []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if value, ok := token.(xml.CharData); ok {
			text = append(text, string(value))
		}
	}
	if !strings.Contains(strings.Join(text, ""), c.Directory) || !strings.Contains(string(data), "<key>RunAtLoad</key><true/>") || !strings.Contains(string(data), "<key>GIT_TERMINAL_PROMPT</key>") {
		t.Fatal("invalid plist", string(data))
	}
	if strings.Contains(string(data), "--once") || strings.Contains(string(data), "--dry-run") {
		t.Fatal("background must keep watching")
	}
	if runtime.GOOS == "darwin" {
		if b, err := exec.Command("/usr/bin/plutil", "-lint", p.Plist).CombinedOutput(); err != nil {
			t.Fatalf("plutil: %v\n%s", err, b)
		}
	}
	b, err := os.ReadFile(p.Binary)
	if err != nil || string(b) != "test executable" {
		t.Fatal("stable binary not copied", err)
	}
	if started, err := m.Start(context.Background(), common, c); err != nil || started {
		t.Fatalf("start should be idempotent: %v %v", started, err)
	}
	if status, err := m.Status(context.Background(), common); err != nil || !strings.Contains(status, "pid = 42") {
		t.Fatalf("status=%s err=%v", status, err)
	}
	if err := m.Stop(context.Background(), common); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.Plist); !os.IsNotExist(err) {
		t.Fatal("stop left autostart configuration")
	}
	if status, err := m.Status(context.Background(), common); err != nil || !strings.Contains(status, "未配置") {
		t.Fatalf("status=%s err=%v", status, err)
	}
	if err := m.Stop(context.Background(), common); err != nil {
		t.Fatal("stop must be idempotent", err)
	}
	bootstraps := 0
	for _, args := range calls {
		if args[0] == "bootstrap" {
			bootstraps++
		}
	}
	if bootstraps != 1 {
		t.Fatal("duplicate bootstrap")
	}
}

func TestStartRollsBackBootstrapFailure(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[existing], func(t *testing.T) {
			m, common, c := setupManager(t)
			p := m.Paths(common)
			if existing {
				if err := os.MkdirAll(filepath.Dir(p.Plist), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p.Plist, []byte("old config"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			m.command = func(ctx context.Context, args ...string) (string, error) {
				if args[0] == "print" {
					return absent(ctx, args...)
				}
				return "", errors.New("bootstrap denied")
			}
			if _, err := m.Start(context.Background(), common, c); err == nil {
				t.Fatal("expected bootstrap error")
			}
			b, err := os.ReadFile(p.Plist)
			if existing {
				if err != nil || string(b) != "old config" {
					t.Fatal("old config not restored", err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("failed start left login configuration")
			}
		})
	}
}

func TestStartCleansPartiallyLoadedJob(t *testing.T) {
	m, common, c := setupManager(t)
	loaded := false
	m.command = func(ctx context.Context, args ...string) (string, error) {
		switch args[0] {
		case "print":
			if loaded {
				return "state = running", nil
			}
			return absent(ctx, args...)
		case "bootstrap":
			loaded = true
			return "", context.DeadlineExceeded
		case "bootout":
			if ctx.Err() != nil {
				t.Fatal("cleanup used canceled context")
			}
			loaded = false
			return "", nil
		}
		t.Fatal(args)
		return "", nil
	}
	if _, err := m.Start(context.Background(), common, c); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected original timeout: %v", err)
	}
	if loaded {
		t.Fatal("failed bootstrap left a loaded job")
	}
	if _, err := os.Stat(m.Paths(common).Plist); !os.IsNotExist(err) {
		t.Fatal("failed bootstrap left login configuration")
	}
}

func TestPermissionErrorsDoNotRemoveConfiguration(t *testing.T) {
	m, common, _ := setupManager(t)
	p := m.Paths(common)
	if err := os.MkdirAll(filepath.Dir(p.Plist), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Plist, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	m.command = func(context.Context, ...string) (string, error) { return "", errors.New("operation not permitted") }
	if _, err := m.Status(context.Background(), common); err == nil {
		t.Fatal("permission error reported as missing")
	}
	if err := m.Stop(context.Background(), common); err == nil {
		t.Fatal("permission error ignored")
	}
	if _, err := os.Stat(p.Plist); err != nil {
		t.Fatal("configuration removed on failure")
	}
}

func TestBootoutFailureKeepsConfiguration(t *testing.T) {
	m, common, _ := setupManager(t)
	p := m.Paths(common)
	if err := os.MkdirAll(filepath.Dir(p.Plist), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Plist, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	m.command = func(_ context.Context, args ...string) (string, error) {
		if args[0] == "print" {
			return "state = running", nil
		}
		return "", errors.New("bootout failed")
	}
	if err := m.Stop(context.Background(), common); err == nil {
		t.Fatal("ignored bootout error")
	}
	if _, err := os.Stat(p.Plist); err != nil {
		t.Fatal("removed configuration while job may still be running")
	}
}

func TestStartRejectsForegroundWatcher(t *testing.T) {
	m, common, c := setupManager(t)
	m.command = absent
	l, err := lock.Acquire(filepath.Join(common, "gsync.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err := m.Start(context.Background(), common, c); !errors.Is(err, lock.ErrBusy) {
		t.Fatalf("expected busy: %v", err)
	}
	if _, err := os.Stat(m.Paths(common).Plist); !os.IsNotExist(err) {
		t.Fatal("busy repo got login configuration")
	}
}

func TestBoundedLogs(t *testing.T) {
	m, common, _ := setupManager(t)
	p := m.Paths(common)
	if got, err := m.Logs(common, 5); err != nil || got != "暂无日志\n" {
		t.Fatal(got, err)
	}
	if err := os.MkdirAll(filepath.Dir(p.Log), 0700); err != nil {
		t.Fatal(err)
	}
	data := strings.Repeat("old line\n", 10000) + "recent\nhttps://secret@example.com/repo\n"
	if err := os.WriteFile(p.Log, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := m.Logs(common, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != "recent\nhttps://[redacted]@example.com/repo\n" {
		t.Fatal(got)
	}
	if _, err := m.Logs(common, 0); err == nil {
		t.Fatal("accepted invalid line count")
	}
}
