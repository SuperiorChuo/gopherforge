// Package sms 提供短信发送的 provider 抽象与模板参数渲染。
// provider 分三种：debug（只回显、直接成功，供开发联调）、aliyun、tencent。
// 云厂商实现走 HTTP + 签名，无真实密钥时无法真机验证，但代码可编译、签名逻辑可单测。
package sms

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

// Provider 名称常量，与 sms_channels.provider 取值一致。
const (
	ProviderDebug   = "debug"
	ProviderAliyun  = "aliyun"
	ProviderTencent = "tencent"
)

// SendRequest 是一次发送的入参（模板已渲染为 Content）。
type SendRequest struct {
	Mobile string            // 目标手机号
	Sign   string            // 短信签名（云厂商要求）
	Params map[string]string // 模板参数，云厂商模板发送时按 key 传递
	Content string           // 本地渲染后的完整短信内容（debug/日志用）
	// ProviderTemplateID 是云厂商侧模板号；阿里/腾讯按模板号 + 参数发送，
	// 不直接发 Content。为空时云厂商实现会报错。
	ProviderTemplateID string
}

// SendResult 是发送结果。
type SendResult struct {
	MessageID string // 云厂商回执 ID（debug 为空）
}

// Sender 是可插拔的短信发送器。实现需保证 ctx 取消/超时被尊重。
type Sender interface {
	// Provider 返回该实现对应的 provider 名称。
	Provider() string
	// Send 发送一条短信；返回 error 即视为发送失败。
	Send(ctx context.Context, req SendRequest) (*SendResult, error)
}

// paramPattern 匹配 {name} 形式的占位符；name 由字母数字下划线组成。
var paramPattern = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

// RenderTemplate 把 content 里的 {key} 占位替换为 params[key]。
// 未提供的占位保持原样（不静默吞掉），便于测试发送时看出缺参。
func RenderTemplate(content string, params map[string]string) string {
	if content == "" || len(params) == 0 {
		return content
	}
	return paramPattern.ReplaceAllStringFunc(content, func(match string) string {
		key := match[1 : len(match)-1] // 去掉花括号
		if v, ok := params[key]; ok {
			return v
		}
		return match
	})
}

// ExtractParams 返回模板里出现的占位 key（去重、稳定排序），供前端提示填参。
func ExtractParams(content string) []string {
	matches := paramPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	keys := make([]string, 0, len(matches))
	for _, m := range matches {
		key := m[1]
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// MissingParams 返回模板要求但 params 未提供的占位 key。
func MissingParams(content string, params map[string]string) []string {
	required := ExtractParams(content)
	if len(required) == 0 {
		return nil
	}
	var missing []string
	for _, key := range required {
		if v, ok := params[key]; !ok || strings.TrimSpace(v) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}
