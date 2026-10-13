//go:build windows

package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Windows 用当前用户的计划任务（/sc onlogon）而不是 Windows 服务：注册服务
// 需要管理员权限，而 P1 要求「不要求 root/Administrator 常驻权限」。
// 代价是没有 KeepAlive 等价物——崩溃后要等下一轮 reconcile 把它拉起来
// （manager 的同步周期本来就会做进程收敛，所以可接受）。
const scheduledTaskName = "GakHermes"

const (
	windowsStopCheckAttempts = 20
	windowsStopCheckInterval = 250 * time.Millisecond
)

type windowsProcess struct {
	base *Base
}

// NewAdapter 构造 Windows 适配器。
func NewAdapter(options Options) (Adapter, error) {
	proc := &windowsProcess{}
	base, err := newBase(options, proc)
	if err != nil {
		return nil, err
	}
	proc.base = base
	return base, nil
}

// launcher 是计划任务实际执行的批处理：设置 HERMES_HOME、记录 pid、把输出
// 重定向到日志。schtasks 的 /tr 参数没有可靠的多参数与环境变量表达方式，
// 包一层 .cmd 比往 /tr 里塞引号健壮得多。
func (p *windowsProcess) launcherPath() string {
	return filepath.Join(p.base.home, "tools", "start-hermes.cmd")
}

func (p *windowsProcess) pidPath() string {
	return filepath.Join(p.base.home, "gateway.pid")
}

func (p *windowsProcess) start(ctx context.Context) error {
	version, err := p.base.InstalledVersion()
	if err != nil {
		return err
	}
	if version == "" {
		return Coded(ErrCodeNotInstalled, fmt.Errorf("no installed version"))
	}

	launcher := p.launcherPath()
	logDir := filepath.Join(p.base.home, "logs")
	script := renderWindowsLauncher(
		p.base.hermesBinary(version),
		p.base.profileDir(),
		filepath.Join(logDir, "gateway.out.log"),
		filepath.Join(logDir, "gateway.err.log"),
		p.pidPath(),
	)
	if err := atomicWriteFile(filepath.Dir(launcher), filepath.Base(launcher), []byte(script), 0o600); err != nil {
		return err
	}

	// /f 覆盖同名任务，使换版本后的 launcher 幂等重注册。
	if _, err := p.base.runner.Run(ctx, "", nil, "schtasks", "/create", "/f",
		"/tn", scheduledTaskName,
		"/sc", "onlogon",
		"/tr", launcher,
	); err != nil {
		return err
	}
	// onlogon 只在下次登录触发，当下要立刻起来得显式 /run。
	if _, err := p.base.runner.Run(ctx, "", nil, "schtasks", "/run", "/tn", scheduledTaskName); err != nil {
		return err
	}
	return nil
}

func (p *windowsProcess) stop(ctx context.Context) error {
	// /end 结束任务实例。错误先保留：任务不存在属于幂等成功，但任务仍存在
	// 且无法结束时必须上抛，不能让吊销误报成功。
	_, endErr := p.base.runner.Run(ctx, "", nil, "schtasks", "/end", "/tn", scheduledTaskName)

	pid, err := p.readPID()
	if err != nil || pid <= 0 {
		if endErr == nil {
			return nil
		}
		// 没有 pidfile 时再确认任务是否存在。query 也失败通常表示任务尚未
		// 创建，符合首次安装前 Stop 的幂等语义。
		if _, queryErr := p.base.runner.Run(ctx, "", nil, "schtasks", "/query", "/tn", scheduledTaskName); queryErr == nil {
			return endErr
		}
		return nil
	}
	// /t 连带子进程：hermes gateway 会派生 Python 子进程，只杀父进程会留孤儿。
	_, killErr := p.base.runner.Run(ctx, "", nil, "taskkill", "/pid", strconv.Itoa(pid), "/t", "/f")
	if waitErr := p.waitForExit(ctx, pid); waitErr != nil {
		if killErr != nil {
			return fmt.Errorf("taskkill failed: %w; process exit check: %v", killErr, waitErr)
		}
		return waitErr
	}
	if err := os.Remove(p.pidPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (p *windowsProcess) waitForExit(ctx context.Context, pid int) error {
	var lastErr error
	for attempt := 0; attempt < windowsStopCheckAttempts; attempt++ {
		running, err := p.running(ctx)
		if err == nil && !running {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		if attempt == windowsStopCheckAttempts-1 {
			break
		}
		timer := time.NewTimer(windowsStopCheckInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("Hermes process %d is still running after stop", pid)
}

func (p *windowsProcess) running(ctx context.Context) (bool, error) {
	pid, err := p.readPID()
	if err != nil || pid <= 0 {
		return false, nil
	}
	// pidfile 可能是上次崩溃残留，必须向 tasklist 复核该 pid 还活着。
	output, err := p.base.runner.Run(ctx, "", nil, "tasklist", "/fi", fmt.Sprintf("PID eq %d", pid), "/nh")
	if err != nil {
		return false, err
	}
	return strings.Contains(string(output), strconv.Itoa(pid)), nil
}

func (p *windowsProcess) readPID() (int, error) {
	data, err := os.ReadFile(p.pidPath())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}
