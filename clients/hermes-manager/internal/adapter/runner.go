package adapter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Runner 抽象子进程执行，安装流水线与进程适配都经它调用外部命令。
// 单测用 fake 断言完整命令串——这也让「manager 到底会执行什么」在测试里
// 一目了然，呼应「不执行任意命令」的承诺。
type Runner interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error)
}

// ExecRunner 是真实实现。env 为增量环境变量（叠加在当前进程之上）。
type ExecRunner struct {
	// Timeout 单命令上限；0 用默认 15 分钟（pip install 依赖多，宁可宽松）。
	Timeout time.Duration
}

func (r ExecRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if err != nil {
		// 输出截断进错误便于本地排障；上报控制面的只有错误码，不含此正文
		tail := output.Bytes()
		if len(tail) > 2048 {
			tail = tail[len(tail)-2048:]
		}
		return output.Bytes(), fmt.Errorf("%s: %w: %s", name, err, string(tail))
	}
	return output.Bytes(), nil
}
