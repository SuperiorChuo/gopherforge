//go:build windows

package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsStartRegistersOnlogonTaskAndRunsIt(t *testing.T) {
	home := t.TempDir()
	runner := &fakeRunner{}
	adp, err := NewAdapter(Options{Home: home, Runner: runner})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	base := adp.(*Base)
	if err := base.savePointer(&currentPointer{Version: "v1"}); err != nil {
		t.Fatalf("save pointer: %v", err)
	}

	if err := base.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	joined := runner.joined()
	// /f 覆盖同名任务，让换版本后的重注册幂等
	for _, want := range []string{
		"schtasks /create /f",
		"/sc onlogon",
		"schtasks /run",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("start must register and run task, missing %q:\n%s", want, joined)
		}
	}
	// launcher 落盘后计划任务才有东西可跑
	if _, err := os.Stat(filepath.Join(home, "tools", "start-hermes.cmd")); err != nil {
		t.Fatalf("launcher script missing: %v", err)
	}
}

// 残留 pidfile 不能被当成「还在跑」——否则 reconcile 永远不会把崩掉的 gateway 拉起。
func TestWindowsRunningRequiresLiveProcess(t *testing.T) {
	home := t.TempDir()
	runner := &fakeRunner{}
	adp, err := NewAdapter(Options{Home: home, Runner: runner})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	base := adp.(*Base)

	// 无 pidfile：未运行
	running, err := base.Running(context.Background())
	if err != nil || running {
		t.Fatalf("no pidfile → running=%v err=%v, want false", running, err)
	}

	// 有 pidfile 但 tasklist 不认这个 pid（fakeRunner 回 "ok"）：仍算未运行
	if err := os.WriteFile(filepath.Join(home, "gateway.pid"), []byte("4321\n"), 0o600); err != nil {
		t.Fatalf("write pidfile: %v", err)
	}
	running, err = base.Running(context.Background())
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if running {
		t.Fatal("stale pidfile must not report running")
	}
}

func TestWindowsStopKillsProcessTree(t *testing.T) {
	home := t.TempDir()
	runner := &fakeRunner{}
	adp, err := NewAdapter(Options{Home: home, Runner: runner})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	base := adp.(*Base)
	if err := os.WriteFile(filepath.Join(home, "gateway.pid"), []byte("4321\n"), 0o600); err != nil {
		t.Fatalf("write pidfile: %v", err)
	}

	if err := base.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}

	joined := runner.joined()
	if !strings.Contains(joined, "schtasks /end") {
		t.Fatalf("stop must end the task:\n%s", joined)
	}
	// /t 连带子进程：hermes 会派生 Python 子进程，漏了会留孤儿进程
	if !strings.Contains(joined, "taskkill /pid 4321 /t /f") {
		t.Fatalf("stop must kill the process tree:\n%s", joined)
	}
	if _, err := os.Stat(filepath.Join(home, "gateway.pid")); !os.IsNotExist(err) {
		t.Fatal("stop must remove the pidfile")
	}
}
