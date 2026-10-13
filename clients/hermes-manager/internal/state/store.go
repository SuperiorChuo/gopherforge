package state

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
)

var ErrNotEnrolled = errors.New("Hermes manager is not enrolled")

const (
	configFileName       = "manager.json"
	privateKeyFileName   = "device.key"
	desiredStateFileName = "desired-state.json"
)

type Config struct {
	ServerURL              string `json:"server_url"`
	DeviceID               string `json:"device_id"`
	PublicKey              string `json:"public_key"`
	PolicySigningPublicKey string `json:"policy_signing_public_key"`
	ManagerVersion         string `json:"manager_version"`
	DesiredPolicyRevision  uint64 `json:"desired_policy_revision"`
	AppliedPolicyRevision  uint64 `json:"applied_policy_revision"`
	ActualState            string `json:"actual_state"`
	HealthStatus           string `json:"health_status"`
	HermesVersion          string `json:"hermes_version"`
	ConfigDigest           string `json:"config_digest"`
	LastErrorCode          string `json:"last_error_code"`
	// FailedUpdate* 隔离一次已自动回滚的目标，避免常驻循环对同一失败
	// 制品每分钟重复安装。新策略或 retry_update 会解除隔离。
	FailedUpdateArtifactID     uint64 `json:"failed_update_artifact_id,omitempty"`
	FailedUpdatePolicyRevision uint64 `json:"failed_update_policy_revision,omitempty"`
	FailedUpdateVersion        string `json:"failed_update_version,omitempty"`
	// UpdateInProgress* 在切版本前落盘。若 manager 在停旧进程、切指针或
	// 启动确认之间崩溃，下一轮仍能识别这是未完成更新并执行自动回滚。
	UpdateInProgressArtifactID     uint64 `json:"update_in_progress_artifact_id,omitempty"`
	UpdateInProgressPolicyRevision uint64 `json:"update_in_progress_policy_revision,omitempty"`
	UpdateInProgressVersion        string `json:"update_in_progress_version,omitempty"`
	// ModelTokenExpiresAt 是当前写入受管 .env 的模型 token 到期时间（不存明文
	// token 本身，明文只在 .env 里）。用来决定何时换发：服务端换发即吊销旧
	// token，而本地 Hermes 只在启动时读 .env，所以不能每轮同步都换——那会把
	// 正在运行的 gateway 手里的凭据打废。
	ModelTokenExpiresAt time.Time `json:"model_token_expires_at,omitzero"`
	// APIServerKey 是 manager 本机生成的 Gateway API server 密钥（三期）。原始
	// key 只存 manager.json（0600）与 profile .env，控制面只见指纹。长期密钥，
	// 不随 desired-state 轮转。APIServerFingerprint 是 sha256(key)[:16] 的 hex，
	// 随心跳上报。LastPushedRunID 是最近一次成功下发的任务 run（幂等去重）。
	APIServerKey          string `json:"api_server_key,omitempty"`
	APIServerFingerprint  string `json:"api_server_fingerprint,omitempty"`
	LastPushedRunID       uint64 `json:"last_pushed_run_id,omitempty"`
	// KanbanReportedRunID 是最近一次经 kanban 通道完成并回传的控制面 run（幂等）。
	// KanbanConfigDigest 是运行中 gateway 所持 company_cluster config 摘要；
	// 与 ConfigDigest 不一致 = 需重启 gateway 才能加载 kanban 工具。
	KanbanReportedRunID uint64 `json:"kanban_reported_run_id,omitempty"`
	KanbanConfigDigest  string `json:"kanban_config_digest,omitempty"`
	// SkillsAppliedDigest 是最近一次成功应用的技能集摘要。与 desired Skills 的
	// 摘要不一致 = 需下载/解压新技能并重启 gateway 加载。
	SkillsAppliedDigest string `json:"skills_applied_digest,omitempty"`
	// McpServersAppliedDigest 是最近一次成功应用的 MCP server 集摘要。与 desired
	// McpServers 摘要不一致 = 需拉取密钥、写 .env 并重启 gateway 加载。
	McpServersAppliedDigest string `json:"mcp_servers_applied_digest,omitempty"`
	// CronLastRuns 是 cron 任务最近一次触发的分钟槽（jobID → unix/60，M5）。
	// 用于本分钟内幂等：同一分钟不重复推同一任务；重启后从落盘恢复。
	CronLastRuns map[string]int64 `json:"cron_last_runs,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type Store struct {
	dir string
}

func DefaultDir() (string, error) {
	if override := os.Getenv("GAK_HERMES_HOME"); override != "" {
		return filepath.Clean(override), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(configDir, "go-admin-kit", "hermes-manager"), nil
}

func NewStore(dir string) (*Store, error) {
	if dir == "" {
		var err error
		dir, err = DefaultDir()
		if err != nil {
			return nil, err
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create manager state directory: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil {
			return nil, fmt.Errorf("secure manager state directory: %w", err)
		}
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) EnsureDeviceKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	privateKey, err := s.LoadPrivateKey()
	if err == nil {
		return privateKey.Public().(ed25519.PublicKey), privateKey, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate device key: %w", err)
	}
	encoded := []byte(base64.RawStdEncoding.EncodeToString(privateKey) + "\n")
	if err := s.atomicWrite(privateKeyFileName, encoded, 0o600); err != nil {
		return nil, nil, err
	}
	return publicKey, privateKey, nil
}

func (s *Store) LoadPrivateKey() (ed25519.PrivateKey, error) {
	encoded, err := os.ReadFile(filepath.Join(s.dir, privateKeyFileName))
	if err != nil {
		return nil, err
	}
	decoded, err := base64.RawStdEncoding.DecodeString(string(bytesTrimSpace(encoded)))
	if err != nil || len(decoded) != ed25519.PrivateKeySize {
		return nil, errors.New("stored Hermes device key is invalid")
	}
	return ed25519.PrivateKey(decoded), nil
}

func (s *Store) LoadConfig() (*Config, error) {
	var config Config
	if err := s.readJSON(configFileName, &config); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotEnrolled
		}
		return nil, err
	}
	if config.DeviceID == "" || config.ServerURL == "" || config.PolicySigningPublicKey == "" {
		return nil, ErrNotEnrolled
	}
	return &config, nil
}

func (s *Store) SaveConfig(config *Config) error {
	if config == nil {
		return errors.New("manager config is nil")
	}
	config.UpdatedAt = time.Now().UTC()
	return s.writeJSON(configFileName, config, 0o600)
}

func (s *Store) SaveDesiredState(desired *protocol.DesiredState) error {
	if desired == nil {
		return errors.New("desired state is nil")
	}
	return s.writeJSON(desiredStateFileName, desired, 0o600)
}

func (s *Store) LoadDesiredState() (*protocol.DesiredState, error) {
	var desired protocol.DesiredState
	if err := s.readJSON(desiredStateFileName, &desired); err != nil {
		return nil, err
	}
	return &desired, nil
}

func (s *Store) readJSON(name string, target any) error {
	data, err := os.ReadFile(filepath.Join(s.dir, name))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}

func (s *Store) writeJSON(name string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return s.atomicWrite(name, data, mode)
}

func (s *Store) atomicWrite(name string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(s.dir, ".hermes-manager-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, filepath.Join(s.dir, name))
}

func bytesTrimSpace(value []byte) []byte {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\n' || value[start] == '\r' || value[start] == '\t') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\n' || value[end-1] == '\r' || value[end-1] == '\t') {
		end--
	}
	return value[start:end]
}
