package sms

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnsupportedProvider 表示渠道 provider 不在支持列表内。
var ErrUnsupportedProvider = errors.New("unsupported sms provider")

// NewSenderFromConfig 按 provider + 渠道 config（sms_channels.config JSON）构造发送器。
// config 的 key 约定（示例值全为占位，真实密钥只存数据库）：
//
//	aliyun:  access_key_id / access_key_secret / sign_name / region_id(可选)
//	tencent: secret_id / secret_key / sdk_app_id / sign_name / region(可选)
//	debug:   无需配置
func NewSenderFromConfig(provider string, config map[string]any) (Sender, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderDebug:
		return NewDebugSender(), nil
	case ProviderAliyun:
		return NewAliyunSender(AliyunConfig{
			AccessKeyID:     configString(config, "access_key_id"),
			AccessKeySecret: configString(config, "access_key_secret"),
			SignName:        configString(config, "sign_name"),
			RegionID:        configString(config, "region_id"),
		}), nil
	case ProviderTencent:
		return NewTencentSender(TencentConfig{
			SecretID:  configString(config, "secret_id"),
			SecretKey: configString(config, "secret_key"),
			SdkAppID:  configString(config, "sdk_app_id"),
			SignName:  configString(config, "sign_name"),
			Region:    configString(config, "region"),
		}), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProvider, provider)
	}
}

// configString 从 config map 取字符串值（缺失/类型不符返回空串）。
func configString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	if v, ok := config[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
