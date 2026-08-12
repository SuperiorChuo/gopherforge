// Package secretstrength 提供密钥强度校验（生产环境凭据门禁）。

package secretstrength

import "strings"

func IsProductionEnv(env string) bool {
	return strings.EqualFold(strings.TrimSpace(env), "production")
}

func IsStrongSecret(value string, minLength int) bool {
	value = strings.TrimSpace(value)
	return len(value) >= minLength && !IsPlaceholderValue(value)
}

var weakValues = map[string]struct{}{
	"123456":                {},
	"access-key":            {},
	"accesskey":             {},
	"admin":                 {},
	"aws-access-key-id":     {},
	"aws-secret-access-key": {},
	"aws_access_key_id":     {},
	"aws_secret_access_key": {},
	"awsaccesskeyid":        {},
	"awssecretaccesskey":    {},
	"changeme":              {},
	"default":               {},
	"demo":                  {},
	"development":           {},
	"example":               {},
	"go-admin-kit":          {},
	"local":                 {},
	"minioadmin":            {},
	"password":              {},
	"redis":                 {},
	"root":                  {},
	"sample":                {},
	"secret":                {},
	"secret-key":            {},
	"secretkey":             {},
	"test":                  {},
	"test123":               {},
}

func IsWeakCredential(value string) bool {
	normalized := NormalizeSecretValue(value)
	if normalized == "" || IsPlaceholderValue(normalized) {
		return true
	}
	if _, ok := weakValues[normalized]; ok {
		return true
	}
	return strings.HasPrefix(normalized, "dev-")
}

var placeholderValues = map[string]struct{}{
	"change-me":                                  {},
	"changeme":                                   {},
	"dev-im-ai-internal-token":                   {},
	"dev-notify-internal-token":                  {},
	"local-dev-secret-change-me-32-chars":        {},
	"replace-me":                                 {},
	"replace-with-at-least-32-random-characters": {},
	"your-password":                              {},
	"your-secret-key":                            {},
}

func IsPlaceholderValue(value string) bool {
	normalized := NormalizeSecretValue(value)
	if normalized == "" {
		return true
	}
	if _, ok := placeholderValues[normalized]; ok {
		return true
	}
	return strings.Contains(normalized, "change-me") ||
		strings.Contains(normalized, "placeholder") ||
		strings.Contains(normalized, "replace-with") ||
		strings.HasPrefix(normalized, "your-")
}

func NormalizeSecretValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func OAuthConfigValueReady(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !IsPlaceholderValue(value)
}
