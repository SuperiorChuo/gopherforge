//go:build darwin

package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// macOS 用 per-user LaunchAgent（不是系统级 LaunchDaemon）：以员工自己的账户
// 运行，安装与启停都不需要管理员权限。代价是本机管理员能 bootout 掉它——
// 这属于文档 §2.2 承认的妥协，控制面靠心跳把这种情况标成离线/不合规。
const launchAgentLabel = "com.gak.hermes"

type darwinProcess struct {
	base *Base
	// plistFile 仅在内部测试注入；空值保持生产的用户 LaunchAgents 路径。
	plistFile string
}

// NewAdapter 构造 macOS 适配器。
func NewAdapter(options Options) (Adapter, error) {
	proc := &darwinProcess{}
	base, err := newBase(options, proc)
	if err != nil {
		return nil, err
	}
	proc.base = base
	return base, nil
}

func (p *darwinProcess) plistPath() (string, error) {
	if p.plistFile != "" {
		return p.plistFile, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"), nil
}

// serviceTarget 是 launchctl 的 gui/<uid>/<label> 形式服务目标。
func (p *darwinProcess) serviceTarget() string {
	return fmt.Sprintf("gui/%d/%s", os.Getuid(), launchAgentLabel)
}

func (p *darwinProcess) domainTarget() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func (p *darwinProcess) start(ctx context.Context) error {
	version, err := p.base.InstalledVersion()
	if err != nil {
		return err
	}
	if version == "" {
		return Coded(ErrCodeNotInstalled, fmt.Errorf("no installed version"))
	}
	plistFile, err := p.plistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistFile), 0o755); err != nil {
		return err
	}
	logDir := filepath.Join(p.base.home, "logs")
	plist := renderPlist(launchAgentLabel, p.base.hermesBinary(version), p.base.profileDir(),
		filepath.Join(logDir, "gateway.out.log"), filepath.Join(logDir, "gateway.err.log"))
	if err := atomicWriteFile(filepath.Dir(plistFile), filepath.Base(plistFile), []byte(plist), 0o644); err != nil {
		return err
	}

	// 先 bootout 再 bootstrap：plist 变了（换版本）时 launchd 不会自动重读，
	// 幂等重装服务定义是唯一可靠的做法。已不存在时 bootout 报错，忽略。
	_, _ = p.base.runner.Run(ctx, "", nil, "launchctl", "bootout", p.serviceTarget())
	var bootstrapErr error
	for attempt := 0; attempt < 5; attempt++ {
		if _, bootstrapErr = p.base.runner.Run(ctx, "", nil, "launchctl", "bootstrap", p.domainTarget(), plistFile); bootstrapErr == nil {
			break
		}
		if attempt == 4 {
			return bootstrapErr
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	// enable 让服务在被显式 disable 过之后仍能起来（disable 状态跨重启保留）
	_, _ = p.base.runner.Run(ctx, "", nil, "launchctl", "enable", p.serviceTarget())
	return nil
}

func (p *darwinProcess) stop(ctx context.Context) error {
	// bootout 对未加载的服务返回非零，视作已停止。
	if _, err := p.base.runner.Run(ctx, "", nil, "launchctl", "bootout", p.serviceTarget()); err != nil {
		running, checkErr := p.running(ctx)
		if checkErr == nil && !running {
			return nil
		}
		return err
	}
	return nil
}

// running 解析 launchctl print 的 state / PID 字段。服务已加载但未在跑
// （state = waiting）不算 running。
func (p *darwinProcess) running(ctx context.Context) (bool, error) {
	output, err := p.base.runner.Run(ctx, "", nil, "launchctl", "print", p.serviceTarget())
	if err != nil {
		// 未加载：launchctl print 返回非零
		return false, nil
	}
	return parseLaunchctlRunning(string(output)), nil
}
