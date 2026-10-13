package protocol

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const maxResponseBody = 2 * 1024 * 1024

// maxSkillBytes 是技能 tarball 上限（与 server 侧 16MB 上传上限一致）。
const maxSkillBytes = 16 * 1024 * 1024

type APIError struct {
	Status    int
	Code      string
	Message   string
	Retryable bool
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("Hermes API %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("Hermes API returned HTTP %d: %s", e.Status, e.Message)
}

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	now        func() time.Time
}

func NewClient(serverURL string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid control-plane URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("control-plane URL must use http or https")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{baseURL: parsed, httpClient: httpClient, now: time.Now}, nil
}

func (c *Client) CreateDeviceAuthorization(ctx context.Context, request DeviceAuthorizationRequest) (*DeviceAuthorization, error) {
	return doJSON[DeviceAuthorization](c, ctx, http.MethodPost, "/api/v1/ai/hermes/device-authorizations", request, nil)
}

func (c *Client) ExchangeDeviceCode(ctx context.Context, deviceCode string) (*DeviceIdentity, error) {
	return doJSON[DeviceIdentity](c, ctx, http.MethodPost, "/api/v1/ai/hermes/device-authorizations/token", map[string]string{"device_code": deviceCode}, nil)
}

func (c *Client) DesiredState(ctx context.Context, identity *Identity) (*DesiredState, error) {
	state, err := doJSON[DesiredState](c, ctx, http.MethodGet, "/api/v1/ai/hermes/device/desired-state", nil, identity)
	if err != nil {
		return nil, err
	}
	if err := state.Verify(identity.PolicySigningPublicKey, c.now()); err != nil {
		return nil, err
	}
	return state, nil
}

func (c *Client) SendHeartbeat(ctx context.Context, identity *Identity, heartbeat Heartbeat) error {
	_, err := doJSON[map[string]bool](c, ctx, http.MethodPost, "/api/v1/ai/hermes/device/heartbeat", heartbeat, identity)
	return err
}

func (c *Client) SendEvent(ctx context.Context, identity *Identity, event DeviceEvent) error {
	_, err := doJSON[map[string]bool](c, ctx, http.MethodPost, "/api/v1/ai/hermes/device/events", event, identity)
	return err
}

// IssueModelToken 用设备签名身份换取给本地 Hermes 用的短期模型访问 token。
func (c *Client) IssueModelToken(ctx context.Context, identity *Identity) (*ModelToken, error) {
	return doJSON[ModelToken](c, ctx, http.MethodPost, "/api/v1/ai/hermes/device/model-token", map[string]string{}, identity)
}

// DownloadSkill 下载技能 tarball（M2 技能市场）。SourceURL 是相对路径时对
// ServerURL 解析（设备签名 GET）；绝对 URL 仅允许与控制面同 host。返回原始
// bytes，调用方负责 sha256 校验。
func (c *Client) DownloadSkill(ctx context.Context, identity *Identity, skill DesiredStateSkill) ([]byte, error) {
	path := skill.SourceURL
	if parsed, err := url.Parse(skill.SourceURL); err == nil && parsed.Host != "" {
		// 绝对 URL：host 必须与控制面一致（P0 只下发相对路径，此为防呆）
		if !strings.EqualFold(parsed.Host, c.baseURL.Host) {
			return nil, fmt.Errorf("skill source url host mismatch")
		}
		path = parsed.Path
	}
	target := *c.baseURL
	target.Path = strings.TrimRight(c.baseURL.Path, "/") + path
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	if identity != nil {
		if err := SignRequest(httpRequest, nil, identity.DeviceID, identity.PrivateKey, c.now()); err != nil {
			return nil, err
		}
	}
	responseValue, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("call Hermes control plane: %w", err)
	}
	defer responseValue.Body.Close()
	if responseValue.StatusCode < 200 || responseValue.StatusCode >= 300 {
		return nil, &APIError{
			Status:    responseValue.StatusCode,
			Code:      "SKILL_DOWNLOAD_FAILED",
			Message:   "skill download failed",
			Retryable: responseValue.StatusCode >= 500,
		}
	}
	body, err := io.ReadAll(io.LimitReader(responseValue.Body, maxSkillBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read skill download: %w", err)
	}
	if len(body) > maxSkillBytes {
		return nil, fmt.Errorf("skill download exceeds %d bytes", maxSkillBytes)
	}
	return body, nil
}

// FetchMcpSecrets 拉取 MCP server 密钥值（M3）。设备签名，按 envKeys 请求，
// 服务端只返回集群授权键的交集。写进 profile .env 供 config.yaml 的 ${VAR} 引用。
func (c *Client) FetchMcpSecrets(ctx context.Context, identity *Identity, envKeys []string) (map[string]string, error) {
	body := map[string]any{"env_keys": envKeys}
	type secretsResponse struct {
		Secrets map[string]string `json:"secrets"`
	}
	result, err := doJSON[secretsResponse](c, ctx, http.MethodPost, "/api/v1/ai/hermes/device/mcp-secrets", body, identity)
	if err != nil {
		return nil, err
	}
	return result.Secrets, nil
}

// ReportTaskRun 上报设备端执行完任务后的结果（completed/failed + result）。
// 走设备签名通道，服务端据此把 run 置终态并写结果摘要。
func (c *Client) ReportTaskRun(ctx context.Context, identity *Identity, runID uint64, status, result string) error {
	body := map[string]any{
		"run_id": runID,
		"status": status,
		"result": result,
	}
	_, err := doJSON[map[string]bool](c, ctx, http.MethodPost, "/api/v1/ai/hermes/device/task-runs", body, identity)
	return err
}

// ReportCronRun 把 cron 任务执行结果回传控制面（M5）。
func (c *Client) ReportCronRun(ctx context.Context, identity *Identity, jobID uint64, status, result string) error {
	body := map[string]any{
		"job_id": jobID,
		"status": status,
		"result": result,
	}
	_, err := doJSON[map[string]bool](c, ctx, http.MethodPost, "/api/v1/ai/hermes/device/cron-runs", body, identity)
	return err
}

type Identity struct {
	DeviceID               string
	PrivateKey             ed25519.PrivateKey
	PolicySigningPublicKey string
}

func doJSON[T any](client *Client, ctx context.Context, method, path string, requestBody any, identity *Identity) (*T, error) {
	var body []byte
	var err error
	if requestBody != nil {
		body, err = json.Marshal(requestBody)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
	}
	target := *client.baseURL
	target.Path = strings.TrimRight(client.baseURL.Path, "/") + path
	httpRequest, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if requestBody != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	if identity != nil {
		if err := SignRequest(httpRequest, body, identity.DeviceID, identity.PrivateKey, client.now()); err != nil {
			return nil, err
		}
	}

	responseValue, err := client.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("call Hermes control plane: %w", err)
	}
	defer responseValue.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(responseValue.Body, maxResponseBody+1))
	if err != nil {
		return nil, fmt.Errorf("read Hermes response: %w", err)
	}
	if len(responseBody) > maxResponseBody {
		return nil, fmt.Errorf("Hermes response exceeds %d bytes", maxResponseBody)
	}
	var envelope Envelope[T]
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode Hermes response: %w", err)
	}
	if responseValue.StatusCode < 200 || responseValue.StatusCode >= 300 || envelope.Code != http.StatusOK {
		return nil, &APIError{
			Status:    responseValue.StatusCode,
			Code:      envelope.ErrorCode,
			Message:   envelope.Message,
			Retryable: responseValue.StatusCode >= 500 || envelope.ErrorCode == "HERMES_AUTHORIZATION_PENDING",
		}
	}
	return &envelope.Data, nil
}

func CanonicalRequest(method, escapedPath, rawQuery, timestamp, nonce, bodyHash string) []byte {
	return []byte(strings.Join([]string{
		strings.ToUpper(method),
		escapedPath,
		rawQuery,
		timestamp,
		nonce,
		strings.ToLower(bodyHash),
	}, "\n"))
}

func SignRequest(request *http.Request, body []byte, deviceID string, privateKey ed25519.PrivateKey, now time.Time) error {
	if len(privateKey) != ed25519.PrivateKeySize || strings.TrimSpace(deviceID) == "" {
		return fmt.Errorf("device identity is incomplete")
	}
	nonceBytes := make([]byte, 18)
	if _, err := io.ReadFull(rand.Reader, nonceBytes); err != nil {
		return fmt.Errorf("generate request nonce: %w", err)
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	timestamp := fmt.Sprintf("%d", now.Unix())
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	canonical := CanonicalRequest(request.Method, request.URL.EscapedPath(), request.URL.RawQuery, timestamp, nonce, digestHex)
	signature := ed25519.Sign(privateKey, canonical)
	request.Header.Set("X-Hermes-Device-ID", deviceID)
	request.Header.Set("X-Hermes-Timestamp", timestamp)
	request.Header.Set("X-Hermes-Nonce", nonce)
	request.Header.Set("X-Hermes-Body-SHA256", digestHex)
	request.Header.Set("X-Hermes-Signature", base64.RawStdEncoding.EncodeToString(signature))
	return nil
}

func (state *DesiredState) Verify(encodedPublicKey string, now time.Time) error {
	if state == nil {
		return errors.New("desired state is nil")
	}
	publicKey, err := decodeBase64(encodedPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("control-plane signing key is invalid")
	}
	signature, err := decodeBase64(state.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("desired-state signature is invalid")
	}
	payload, err := json.Marshal(state.UnsignedDesiredState)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		return errors.New("desired-state signature verification failed")
	}
	if state.SchemaVersion != 1 ||
		(state.Profile != "company" && state.Profile != "company_cluster") {
		return errors.New("desired-state schema or profile is unsupported")
	}
	if state.LeaseExpiresAt.Before(now.Add(-30 * time.Second)) {
		return errors.New("desired-state lease has expired")
	}
	if state.DesiredState != "active" && state.DesiredState != "stopped" && state.DesiredState != "revoked" {
		return errors.New("desired-state action is unsupported")
	}
	// 运维动作只认固定枚举——控制面按契约不具备下发任意命令的能力，客户端
	// 同样拒绝任何越出枚举的值（哪怕签名有效）。
	switch state.PendingAction {
	case "", "retry_update", "apply_policy", "restart":
	default:
		return errors.New("desired-state pending action is unsupported")
	}
	if state.Privacy.UploadConversations || state.Privacy.UploadMemory || state.Privacy.UploadLocalFiles {
		return errors.New("desired-state requests unsupported privacy uploads")
	}
	if err := validateProfileToolsets(state.Profile, state.Toolsets); err != nil {
		return err
	}
	if err := validateTask(state.Task); err != nil {
		return err
	}
	if state.Kanban != nil && state.Profile != "company_cluster" {
		return errors.New("desired-state kanban requires company_cluster profile")
	}
	if err := validateKanban(state.Kanban); err != nil {
		return err
	}
	if err := validateSkills(state.Skills); err != nil {
		return err
	}
	if err := validateMcpServers(state.McpServers); err != nil {
		return err
	}
	if err := validateCron(state.Cron); err != nil {
		return err
	}
	if state.Hermes.ID > 0 {
		if len(state.Hermes.SHA256) != sha256.Size*2 {
			return errors.New("desired-state artifact digest is invalid")
		}
		if _, err := hex.DecodeString(state.Hermes.SHA256); err != nil {
			return errors.New("desired-state artifact digest is invalid")
		}
		artifactURL, err := url.Parse(state.Hermes.SourceURL)
		if err != nil || artifactURL.Host == "" || (artifactURL.Scheme != "https" && artifactURL.Scheme != "http") {
			return errors.New("desired-state artifact URL is invalid")
		}
	}
	return nil
}

func isExactP1Toolset(values []string) bool {
	if len(values) != 4 {
		return false
	}
	want := map[string]bool{"clarify": true, "memory": true, "no_mcp": true, "todo": true}
	for _, value := range values {
		if !want[value] {
			return false
		}
		delete(want, value)
	}
	return len(want) == 0
}

// hermesP1Toolsets / hermesClusterToolsets 与服务端 service 侧的
// hermesP1Toolsets / hermesClusterToolsets 保持一致。company_cluster 是
// dispatcher 开放 profile，允许 delegation/skills/mcp 作为超集，但必须仍含
// P1 四件套（guard 下限）。
var hermesP1Toolsets = []string{"clarify", "memory", "no_mcp", "todo"}

var hermesClusterToolsets = []string{"clarify", "memory", "no_mcp", "todo", "delegation", "skills", "mcp"}

// validateProfileToolsets 按 profile 校验工具集白名单：
//   - company：精确 P1 四件套（既有行为，manager isExactP1Toolset 语义不变）。
//   - company_cluster：长度 4-7，每项∈白名单，必须含全部 P1 四件套，无重复。
func validateProfileToolsets(profile string, values []string) error {
	if profile == "company" {
		if !isExactP1Toolset(values) {
			return errors.New("desired-state toolsets exceed the P1 allowlist")
		}
		return nil
	}
	if profile == "company_cluster" {
		if len(values) < 4 || len(values) > len(hermesClusterToolsets) {
			return errors.New("desired-state toolsets are invalid for company_cluster")
		}
		seen := make(map[string]bool, len(values))
		for _, value := range values {
			if !stringIn(value, hermesClusterToolsets) {
				return errors.New("desired-state toolsets exceed the company_cluster allowlist")
			}
			if seen[value] {
				return errors.New("desired-state toolsets contain duplicates")
			}
			seen[value] = true
		}
		for _, required := range hermesP1Toolsets {
			if !seen[required] {
				return errors.New("desired-state toolsets missing P1 baseline for company_cluster")
			}
		}
		return nil
	}
	return errors.New("desired-state profile is unsupported")
}

// validateTask 校验签过名的任务意图形状：畸形（run/task id 为 0、空描述、
// 超长描述）即使签名有效也拒绝，避免把坏任务推给本地 HTTP 服务。
func validateTask(task *DesiredStateTask) error {
	if task == nil {
		return nil
	}
	if task.TaskID == 0 || task.RunID == 0 {
		return errors.New("desired-state task references invalid ids")
	}
	if task.Description == "" {
		return errors.New("desired-state task description is empty")
	}
	if len(task.Description) > 10000 {
		return errors.New("desired-state task description is too long")
	}
	return nil
}

// kanbanWorkerNamePattern 是 kanban worker profile 名白名单。worker 名会成为
// 文件系统目录名（manager 渲染 <root>/profiles/<name>/），因此必须是小写字母
// 数字 + . _ -，无路径分隔符、无 ..、无 : \。这是安全边界：恶意 worker 名不能
// 逃逸 profile 目录。
var kanbanWorkerNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// kanbanBoardSlugPattern 是 board slug 白名单。slug 会成为 board 目录名，
// 允许小写字母数字 + _ -，不含点。
var kanbanBoardSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// validateKanban 校验签过名的 kanban fan-out 指令形状：worker 名/slug 安全、
// assignee 在 worker 集内、无重复、title/body 长度受限。即使签名有效，畸形
// 或不安全的 kanban 指令也必须拒绝（worker 名会落到文件系统）。
func validateKanban(k *DesiredStateKanban) error {
	if k == nil {
		return nil
	}
	if k.TaskID == 0 || k.RunID == 0 {
		return errors.New("desired-state kanban references invalid ids")
	}
	if !kanbanBoardSlugPattern.MatchString(k.BoardSlug) {
		return errors.New("desired-state kanban board slug is invalid")
	}
	if !kanbanWorkerNamePattern.MatchString(k.Assignee) {
		return errors.New("desired-state kanban assignee is invalid")
	}
	if k.Title == "" || len(k.Title) > 120 {
		return errors.New("desired-state kanban title is invalid")
	}
	if k.Body == "" || len(k.Body) > 10000 {
		return errors.New("desired-state kanban body is invalid")
	}
	if len(k.Workers) == 0 {
		return errors.New("desired-state kanban worker set is empty")
	}
	seen := make(map[string]bool, len(k.Workers))
	for _, worker := range k.Workers {
		if !kanbanWorkerNamePattern.MatchString(worker.Name) {
			return errors.New("desired-state kanban worker name is invalid")
		}
		if seen[worker.Name] {
			return errors.New("desired-state kanban worker names contain duplicates")
		}
		seen[worker.Name] = true
	}
	if !seen[k.Assignee] {
		return errors.New("desired-state kanban assignee is not in the worker set")
	}
	return nil
}

// hermesSkillNamePattern 是技能名白名单：会成为 profile skills 目录名，必须
// 文件系统安全（小写字母数字 + . _ -）。与 server 侧 hermesSkillNamePattern 一致。
var hermesSkillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}$`)

// validateSkills 校验签过名的技能引用列表：ID 非 0、name 目录名安全、version
// 安全 tag、sha256 64-hex、source_url 合法、无重复 ID。即使签名有效，不安全
// 的技能名也必须拒绝（会落到文件系统）。
func validateSkills(skills []DesiredStateSkill) error {
	seen := make(map[uint64]bool, len(skills))
	for _, skill := range skills {
		if skill.ID == 0 {
			return errors.New("desired-state skill references invalid id")
		}
		if !hermesSkillNamePattern.MatchString(skill.Name) {
			return errors.New("desired-state skill name is invalid")
		}
		if skill.Version == "" || len(skill.Version) > 40 {
			return errors.New("desired-state skill version is invalid")
		}
		if len(skill.SHA256) != 64 {
			return errors.New("desired-state skill digest is invalid")
		}
		if _, err := hex.DecodeString(skill.SHA256); err != nil {
			return errors.New("desired-state skill digest is invalid")
		}
		// source_url：绝对 http(s) 或本控制面相对下载路径
		if strings.HasPrefix(skill.SourceURL, "/") {
			if !strings.HasPrefix(skill.SourceURL, "/api/v1/ai/hermes/device/skills/") {
				return errors.New("desired-state skill source url is invalid")
			}
		} else {
			parsed, err := url.Parse(skill.SourceURL)
			if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
				return errors.New("desired-state skill source url is invalid")
			}
		}
		if seen[skill.ID] {
			return errors.New("desired-state skill list contains duplicates")
		}
		seen[skill.ID] = true
	}
	return nil
}

// 命令白名单（防御镜像，权威侧在 server service/hermes_mcp.go）。
var mcpAllowedCommands = map[string]bool{"npx": true, "uvx": true, "python": true, "python3": true}

var mcpAllowedPackages = map[string]bool{
	"@modelcontextprotocol/server-filesystem": true,
	"@modelcontextprotocol/server-memory":      true,
	"@modelcontextprotocol/server-everything":  true,
}

var mcpAllowedModules = map[string]bool{"mcp.server": true, "mcp.server.fastmcp": true}

// mcpArgUnsafeChars 拒绝 shell 元字符/插值/换行。
var mcpArgUnsafeChars = regexp.MustCompile("[\x00-\x1f\x7f$;`\"'&|<>()\\[\\]{}*?#!]")

// validateMcpServers 校验签过名的 MCP server 配置列表：ID 非 0、name 目录名安全、
// type 枚举、stdio 命令白名单 argv-only、HTTP URL 校验、env/headers 值无 CR/LF、
// 无重复 ID。即使签名有效，不安全命令/URL 也必须拒绝。
func validateMcpServers(servers []DesiredStateMcpServer) error {
	seen := make(map[uint64]bool, len(servers))
	for _, server := range servers {
		if server.ID == 0 {
			return errors.New("desired-state mcp server references invalid id")
		}
		if !hermesSkillNamePattern.MatchString(server.Name) {
			return errors.New("desired-state mcp server name is invalid")
		}
		switch server.Type {
		case "stdio":
			if err := validateMcpCommandArgsMirror(server.Command, server.Args); err != nil {
				return err
			}
		case "sse", "http":
			if err := validateMcpURLMirror(server.URL); err != nil {
				return err
			}
		default:
			return errors.New("desired-state mcp server type is invalid")
		}
		for _, pair := range []map[string]string{server.Env, server.Headers} {
			for key, value := range pair {
				if strings.ContainsAny(value, "\r\n") {
					return errors.New("desired-state mcp value contains newline")
				}
				_ = key
			}
		}
		if seen[server.ID] {
			return errors.New("desired-state mcp server list contains duplicates")
		}
		seen[server.ID] = true
	}
	return nil
}

// validateCron 校验签过名的 cron 任务引用列表（M5）：ID 非 0、表达式可解析、
// enabled 的任务 prompt 非空且不超长、无重复 ID。即使签名有效，坏表达式/空提示词
// 也必须拒绝——manager 会按这些引用在设备端执行。
func validateCron(jobs []DesiredStateCronJob) error {
	seen := make(map[uint64]bool, len(jobs))
	for _, job := range jobs {
		if job.ID == 0 {
			return errors.New("desired-state cron job references invalid id")
		}
		if job.Name == "" || len(job.Name) > 80 {
			return errors.New("desired-state cron job name is invalid")
		}
		if _, err := ParseCronExpression(job.CronExpression); err != nil {
			return errors.New("desired-state cron job expression is invalid")
		}
		if job.Enabled {
			if job.Prompt == "" {
				return errors.New("desired-state enabled cron job requires a prompt")
			}
			if len(job.Prompt) > 10000 {
				return errors.New("desired-state cron job prompt is too long")
			}
		}
		if seen[job.ID] {
			return errors.New("desired-state cron job list contains duplicates")
		}
		seen[job.ID] = true
	}
	return nil
}

func validateMcpCommandArgsMirror(command string, args []string) error {
	command = strings.ToLower(strings.TrimSpace(command))
	if !mcpAllowedCommands[command] {
		return errors.New("desired-state mcp command is not allowlisted")
	}
	if len(args) == 0 {
		return errors.New("desired-state mcp args are required")
	}
	switch command {
	case "npx", "uvx":
		if args[0] != "-y" && args[0] != "--yes" {
			return errors.New("desired-state npx/uvx must use -y")
		}
		if len(args) < 2 || !mcpAllowedPackages[args[1]] {
			return errors.New("desired-state npx/uvx package is not allowlisted")
		}
		return validateMcpArgsSafeMirror(args[2:])
	case "python", "python3":
		if args[0] != "-m" {
			return errors.New("desired-state python must use -m")
		}
		if len(args) < 2 || !mcpAllowedModules[args[1]] {
			return errors.New("desired-state python module is not allowlisted")
		}
		return validateMcpArgsSafeMirror(args[2:])
	}
	return nil
}

func validateMcpArgsSafeMirror(args []string) error {
	for _, arg := range args {
		if mcpArgUnsafeChars.MatchString(arg) {
			return errors.New("desired-state mcp arg contains unsafe characters")
		}
	}
	return nil
}

func validateMcpURLMirror(raw string) error {
	if raw == "" {
		return errors.New("desired-state mcp url is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return errors.New("desired-state mcp url is malformed")
	}
	if parsed.User != nil {
		return errors.New("desired-state mcp url must not embed credentials")
	}
	host := strings.ToLower(parsed.Hostname())
	if parsed.Scheme == "http" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return errors.New("desired-state mcp http url must target loopback")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("desired-state mcp url must use https")
	}
	return nil
}

func stringIn(target string, values []string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func decodeBase64(value string) ([]byte, error) {
	encodings := []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding}
	var lastErr error
	for _, encoding := range encodings {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
