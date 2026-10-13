//go:build !darwin && !windows

package adapter

import (
	"context"
	"errors"
)

// 本轮只做 macOS 与 Windows（P1 既定范围）。其他平台留结构不留实现：
// NewAdapter 返回错误，装配处据此不注入 adapter，manager 退化为
// 「只同步不执行」并如实上报 PROCESS_ADAPTER_NOT_CONFIGURED。
func NewAdapter(Options) (Adapter, error) {
	return nil, errors.New("no Hermes process adapter for this platform")
}

// 保留 process 接口的空实现位，避免其他平台构建时接口未被引用而报未使用。
type unsupportedProcess struct{}

func (unsupportedProcess) start(context.Context) error           { return errors.New("unsupported") }
func (unsupportedProcess) stop(context.Context) error            { return errors.New("unsupported") }
func (unsupportedProcess) running(context.Context) (bool, error) { return false, nil }
