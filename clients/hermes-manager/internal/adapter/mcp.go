package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// M3 MCP 管理：把 MCP secret 值写进受管 profile 的 .env（MCP_* 前缀），供
// config.yaml 的 mcp_servers 块 ${VAR} 引用。密钥只存在于 .env，不进 config.yaml
// （保持 digest 确定性）。

// ApplyMcpSecrets 把 secrets 写进 orchestrator + 每个 worker profile 的 .env，
// 并移除 desired 集之外的 MCP_* 键（幂等，profile 降级时清理）。空 secrets =
// 清掉全部 MCP_* 键。
func (b *Base) ApplyMcpSecrets(_ context.Context, secrets map[string]string) error {
	// 目标 profile 目录：orchestrator + 每个 worker
	targetDirs := []string{b.profileDir()}
	workersRoot := filepath.Join(b.profileDir(), "profiles")
	if entries, err := os.ReadDir(workersRoot); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				targetDirs = append(targetDirs, filepath.Join(workersRoot, entry.Name()))
			}
		}
	}
	for _, dir := range targetDirs {
		if err := applyMcpSecretsToDir(dir, secrets); err != nil {
			return err
		}
	}
	return nil
}

// applyMcpSecretsToDir 对单个 profile 目录 upsert/移除 MCP_* 键。
func applyMcpSecretsToDir(dir string, secrets map[string]string) error {
	// 先移除所有 MCP_* 键（幂等清理）
	if err := removeMcpEnvKeys(dir); err != nil {
		return err
	}
	for key, value := range secrets {
		if !strings.HasPrefix(key, "MCP_") {
			continue // 只写 MCP_* 命名空间的键
		}
		if err := upsertEnvKeyIn(dir, key, value); err != nil {
			return err
		}
	}
	return nil
}

// removeMcpEnvKeys 移除目录 .env 里所有 MCP_ 前缀键。
func removeMcpEnvKeys(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "MCP_") && strings.Contains(trimmed, "=") {
			continue // 丢弃 MCP_* 键，下面重新写
		}
		kept = append(kept, line)
	}
	content := strings.Join(kept, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return atomicWriteFile(dir, ".env", []byte(content), 0o600)
}

// upsertEnvKeyIn 对指定目录 upsert 一个 .env 键（复用 Base 的原子写）。
func upsertEnvKeyIn(dir, key, value string) error {
	envPath := filepath.Join(dir, ".env")
	var lines []string
	if data, err := os.ReadFile(envPath); err == nil {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	} else if !os.IsNotExist(err) {
		return err
	}
	replaced := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+"=") {
			lines[index] = key + "=" + value
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, key+"="+value)
	}
	content := strings.Join(lines, "\n") + "\n"
	return atomicWriteFile(dir, ".env", []byte(content), 0o600)
}
