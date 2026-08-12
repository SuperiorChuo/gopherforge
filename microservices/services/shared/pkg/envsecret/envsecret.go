// Package envsecret 读取敏感配置：优先 Docker Swarm secrets（/run/secrets/），
// 再回退环境变量。避免密钥出现在 docker inspect / 进程环境列表中。
//
// 查找顺序（首个非空生效）：
//  1. /run/secrets/<ENV_KEY 小写>
//  2. /run/secrets/<ENV_KEY 小写，_ 换成 ->
//  3. /run/secrets/go-admin-kit-<小写带->
//  4. os.Getenv(ENV_KEY)
//  5. fallback
//
// 例：JWT_SECRET → jwt_secret → jwt-secret → go-admin-kit-jwt-secret
package envsecret

import (
	"os"
	"path/filepath"
	"strings"
)

// SecretsDir 默认为 Swarm 挂载点；测试可覆盖。
var SecretsDir = "/run/secrets"

// Get 读取敏感配置。envKey 为环境变量名（如 JWT_SECRET）。
func Get(envKey, fallback string) string {
	for _, p := range secretPaths(envKey) {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if v := strings.TrimSpace(string(b)); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v
	}
	return fallback
}

// GetRequired 同 Get，但空结果返回 false（便于生产启动失败）。
func GetRequired(envKey string) (string, bool) {
	v := Get(envKey, "")
	return v, v != ""
}

func secretPaths(envKey string) []string {
	key := strings.TrimSpace(envKey)
	if key == "" {
		return nil
	}
	lower := strings.ToLower(key)
	dashed := strings.ReplaceAll(lower, "_", "-")
	return []string{
		filepath.Join(SecretsDir, lower),
		filepath.Join(SecretsDir, dashed),
		filepath.Join(SecretsDir, "go-admin-kit-"+dashed),
	}
}
