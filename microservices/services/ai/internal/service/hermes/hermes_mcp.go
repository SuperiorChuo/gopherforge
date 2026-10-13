package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// Hermes M3：MCP 管理。控制面管 MCP server 白名单与配置，manager 受控注入
// config.yaml 的 mcp_servers 块 + .env 密钥。上游不是白名单，安全 gate 在控制面。

// 命令白名单（权威侧，协议 validateMcpServers 防御镜像）。其余命令即使匹配
// 模板也一律拒绝——拦截 bash/sh/cmd/curl/wget/nc 等 shell + 网络外带形状。
var hermesMcpAllowedCommands = map[string]bool{
	"npx": true, "uvx": true, "python": true, "python3": true,
}

// 包/模块白名单：新增 = 代码变更（可审计）。启动形状只允许固定模板。
var hermesMcpAllowedPackages = map[string]bool{
	"@modelcontextprotocol/server-filesystem": true,
	"@modelcontextprotocol/server-memory":     true,
	"@modelcontextprotocol/server-everything": true,
}

var hermesMcpAllowedModules = map[string]bool{
	"mcp.server":         true,
	"mcp.server.fastmcp": true,
}

// hermesMcpArgUnsafeChars 拒绝任何 shell 元字符/插值/换行。
var hermesMcpArgUnsafeChars = regexp.MustCompile("[\x00-\x1f\x7f$;`\"'&|<>()\\[\\]{}*?#!]")

// hermesMcpVarPattern 匹配 ${VAR} 模板（大写字母数字下划线）。
var hermesMcpVarPattern = regexp.MustCompile(`^\$\{[A-Z][A-Z0-9_]*\}$`)

// hermesMcpEnvKeyPattern 是 env/header 键白名单。
var hermesMcpEnvKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
var hermesMcpHeaderKeyPattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func validateMcpCommandArgs(command string, args []string) error {
	command = strings.ToLower(strings.TrimSpace(command))
	if !hermesMcpAllowedCommands[command] {
		return fmt.Errorf("%w: mcp command %q is not allowlisted", ErrHermesInvalidInput, command)
	}
	if len(args) == 0 {
		return fmt.Errorf("%w: mcp args are required", ErrHermesInvalidInput)
	}
	switch command {
	case "npx", "uvx":
		if args[0] != "-y" && args[0] != "--yes" {
			return fmt.Errorf("%w: npx/uvx must use -y", ErrHermesInvalidInput)
		}
		if len(args) < 2 || !hermesMcpAllowedPackages[args[1]] {
			return fmt.Errorf("%w: npx/uvx package is not allowlisted", ErrHermesInvalidInput)
		}
		return validateMcpArgsSafe(args[2:])
	case "python", "python3":
		if args[0] != "-m" {
			return fmt.Errorf("%w: python must use -m", ErrHermesInvalidInput)
		}
		if len(args) < 2 || !hermesMcpAllowedModules[args[1]] {
			return fmt.Errorf("%w: python module is not allowlisted", ErrHermesInvalidInput)
		}
		return validateMcpArgsSafe(args[2:])
	}
	return nil
}

func validateMcpArgsSafe(args []string) error {
	for _, arg := range args {
		if hermesMcpArgUnsafeChars.MatchString(arg) {
			return fmt.Errorf("%w: mcp arg contains unsafe characters", ErrHermesInvalidInput)
		}
	}
	return nil
}

func validateMcpHTTPURL(raw string) error {
	if raw == "" || len(raw) > 1024 {
		return fmt.Errorf("%w: mcp url is required", ErrHermesInvalidInput)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: mcp url is malformed", ErrHermesInvalidInput)
	}
	if parsed.User != nil {
		return fmt.Errorf("%w: mcp url must not embed credentials", ErrHermesInvalidInput)
	}
	host := strings.ToLower(parsed.Hostname())
	if parsed.Scheme == "http" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("%w: mcp http url must target loopback", ErrHermesInvalidInput)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("%w: mcp url must use https", ErrHermesInvalidInput)
	}
	return nil
}

// validateMcpSecretValues 校验 env/headers 的值：无 CR/LF、内嵌 ${VAR} 合法且命中
// secret_json。返回引用的 secret 键集合。
func validateMcpSecretValues(pairs map[string]string, keyPattern *regexp.Regexp, secrets map[string]string) ([]string, error) {
	var refs []string
	for key, value := range pairs {
		if !keyPattern.MatchString(key) {
			return nil, fmt.Errorf("%w: mcp key %q is invalid", ErrHermesInvalidInput, key)
		}
		if strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("%w: mcp value for %q contains newline", ErrHermesInvalidInput, key)
		}
		// 提取 ${VAR} 引用
		for _, token := range regexp.MustCompile(`\$\{[A-Z][A-Z0-9_]*\}`).FindAllString(value, -1) {
			varName := strings.TrimSuffix(strings.TrimPrefix(token, "${"), "}")
			if !hermesMcpVarPattern.MatchString(token) {
				return nil, fmt.Errorf("%w: malformed mcp variable %q", ErrHermesInvalidInput, token)
			}
			if _, ok := secrets[varName]; !ok {
				return nil, fmt.Errorf("%w: mcp variable %q is not declared in secrets", ErrHermesInvalidInput, varName)
			}
			refs = append(refs, varName)
		}
	}
	return refs, nil
}

// normalizeMcpJsonLists 规范化空 JSON 数组/map 为可序列化形态。
func normalizeMcpJsonLists(s *localmodel.HermesMcpServer) {
	if s.Args == nil {
		s.Args = []string{}
	}
	if s.ToolsInclude == nil {
		s.ToolsInclude = []string{}
	}
	if s.ToolsExclude == nil {
		s.ToolsExclude = []string{}
	}
	if s.Env == nil {
		s.Env = map[string]string{}
	}
	if s.Headers == nil {
		s.Headers = map[string]string{}
	}
	if s.SecretJSON == nil {
		s.SecretJSON = map[string]string{}
	}
}

type CreateHermesMcpServerRequest struct {
	Name         string            `json:"name"`
	DisplayName  string            `json:"display_name"`
	Description  string            `json:"description"`
	Type         string            `json:"type"`
	Command      string            `json:"command"`
	Args         []string          `json:"args"`
	URL          string            `json:"url"`
	Env          map[string]string `json:"env"`
	Headers      map[string]string `json:"headers"`
	ToolsInclude []string          `json:"tools_include"`
	ToolsExclude []string          `json:"tools_exclude"`
	Timeout      int               `json:"timeout"`
	SecretJSON   map[string]string `json:"secret_json"`
}

// validateMcpServer 校验 MCP server 定义（Create/Update 共用）。
func validateMcpServer(request *CreateHermesMcpServerRequest) error {
	request.Name = strings.ToLower(strings.TrimSpace(request.Name))
	request.Type = strings.ToLower(strings.TrimSpace(request.Type))
	request.Command = strings.TrimSpace(request.Command)
	request.URL = strings.TrimSpace(request.URL)
	if !hermesSkillNamePattern.MatchString(request.Name) {
		return fmt.Errorf("%w: invalid mcp server name", ErrHermesInvalidInput)
	}
	switch request.Type {
	case "stdio":
		if err := validateMcpCommandArgs(request.Command, request.Args); err != nil {
			return err
		}
	case "sse", "http":
		if err := validateMcpHTTPURL(request.URL); err != nil {
			return err
		}
		// sse 需要 transport: sse 标记，在 config 渲染时按 type 输出
	default:
		return fmt.Errorf("%w: mcp type must be stdio, sse or http", ErrHermesInvalidInput)
	}
	secrets := request.SecretJSON
	if secrets == nil {
		secrets = map[string]string{}
	}
	if _, err := validateMcpSecretValues(request.Env, hermesMcpEnvKeyPattern, secrets); err != nil {
		return err
	}
	if _, err := validateMcpSecretValues(request.Headers, hermesMcpHeaderKeyPattern, secrets); err != nil {
		return err
	}
	return nil
}

func (s *HermesService) CreateMcpServer(ctx context.Context, actorID uint, request CreateHermesMcpServerRequest) (*localmodel.HermesMcpServer, error) {
	if actorID == 0 {
		return nil, fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	if err := validateMcpServer(&request); err != nil {
		return nil, err
	}
	server := &localmodel.HermesMcpServer{
		TenantID:       tenant.FromContextOrDefault(ctx),
		Name:           request.Name,
		DisplayName:    request.DisplayName,
		Description:    request.Description,
		Type:           request.Type,
		Command:        request.Command,
		Args:           request.Args,
		URL:            request.URL,
		Env:            request.Env,
		Headers:        request.Headers,
		ToolsInclude:   request.ToolsInclude,
		ToolsExclude:   request.ToolsExclude,
		Timeout:        request.Timeout,
		Status:         localmodel.HermesMcpPending,
		SecretJSON:     request.SecretJSON,
		SecretRevision: 0,
		CreatedAt:      s.now().UTC(),
		UpdatedAt:      s.now().UTC(),
	}
	normalizeMcpJsonLists(server)
	if err := s.dao.CreateMcpServer(ctx, server); err != nil {
		return nil, err
	}
	server.SecretJSON = nil // 不把密钥回给创建响应
	return server, nil
}

// UpdateMcpServer 更新功能字段并重置 pending（需重审批）。secret 变更 bump
// secret_revision（digest 含它，manager 感知轮换）。
func (s *HermesService) UpdateMcpServer(ctx context.Context, actorID uint, id uint64, request CreateHermesMcpServerRequest) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	if err := validateMcpServer(&request); err != nil {
		return err
	}
	current, err := s.dao.GetMcpServer(ctx, id)
	if err != nil {
		return err
	}
	revision := current.SecretRevision
	if request.SecretJSON != nil && len(request.SecretJSON) > 0 {
		revision++
	}
	fields := map[string]any{
		"display_name":    request.DisplayName,
		"description":     request.Description,
		"type":            request.Type,
		"command":         request.Command,
		"args":            request.Args,
		"url":             request.URL,
		"env":             request.Env,
		"headers":         request.Headers,
		"tools_include":   request.ToolsInclude,
		"tools_exclude":   request.ToolsExclude,
		"timeout":         request.Timeout,
		"status":          localmodel.HermesMcpPending,
		"secret_revision": revision,
		"updated_at":      s.now().UTC(),
	}
	if request.SecretJSON != nil && len(request.SecretJSON) > 0 {
		fields["secret_json"] = request.SecretJSON
	}
	return s.dao.UpdateMcpServer(ctx, id, fields)
}

func (s *HermesService) ListMcpServers(ctx context.Context, page, pageSize int) ([]localmodel.HermesMcpServer, int64, error) {
	page, pageSize = normalizeHermesPage(page, pageSize)
	return s.dao.ListMcpServers(ctx, page, pageSize)
}

func (s *HermesService) ApproveMcpServer(ctx context.Context, actorID uint, id uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	return s.dao.SetMcpServerStatus(ctx, id, localmodel.HermesMcpPending, localmodel.HermesMcpApproved, actorID, s.now().UTC())
}

func (s *HermesService) RejectMcpServer(ctx context.Context, actorID uint, id uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	return s.dao.SetMcpServerStatus(ctx, id, localmodel.HermesMcpPending, localmodel.HermesMcpRejected, actorID, s.now().UTC())
}

func (s *HermesService) RevokeMcpServer(ctx context.Context, actorID uint, id uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	return s.dao.SetMcpServerStatus(ctx, id, localmodel.HermesMcpApproved, localmodel.HermesMcpRevoked, actorID, s.now().UTC())
}

// BindClusterMcpServer 把已审批 MCP server 绑定到启用集群。
func (s *HermesService) BindClusterMcpServer(ctx context.Context, actorID uint, clusterID, serverID uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	cluster, err := s.dao.GetCluster(ctx, clusterID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: cluster not found", ErrHermesInvalidInput)
		}
		return err
	}
	if !cluster.Enabled {
		return fmt.Errorf("%w: cluster is disabled", ErrHermesInvalidInput)
	}
	server, err := s.dao.GetMcpServer(ctx, serverID)
	if err != nil {
		return err
	}
	if server.Status != localmodel.HermesMcpApproved {
		return fmt.Errorf("%w: mcp server is not approved", ErrHermesInvalidInput)
	}
	return s.dao.BindMcpServerToCluster(ctx, clusterID, serverID, actorID, s.now().UTC())
}

func (s *HermesService) UnbindClusterMcpServer(ctx context.Context, actorID uint, clusterID, serverID uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	return s.dao.UnbindMcpServerFromCluster(ctx, clusterID, serverID)
}

func (s *HermesService) ListClusterMcpServers(ctx context.Context, clusterID uint64) ([]localmodel.HermesMcpServer, error) {
	return s.dao.ListClusterMcpServers(ctx, clusterID)
}

// buildDesiredStateMcpServers 载入集群绑定的已审批 MCP server（env/headers 只含
// ${VAR} 模板 + secret_revision，无明文密钥）。
func (s *HermesService) buildDesiredStateMcpServers(ctx context.Context, clusterID uint64) ([]HermesDesiredStateMcpServer, error) {
	servers, err := s.dao.ListApprovedMcpServersForCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, nil
	}
	out := make([]HermesDesiredStateMcpServer, 0, len(servers))
	for _, server := range servers {
		out = append(out, HermesDesiredStateMcpServer{
			ID:             server.ID,
			Name:           server.Name,
			Type:           server.Type,
			URL:            server.URL,
			Command:        server.Command,
			Args:           server.Args,
			Env:            server.Env,
			Headers:        server.Headers,
			ToolsInclude:   server.ToolsInclude,
			ToolsExclude:   server.ToolsExclude,
			Timeout:        server.Timeout,
			SecretRevision: server.SecretRevision,
		})
	}
	return out, nil
}

// IssueMcpSecrets 设备签名端点：解析 device→instance→cluster→已审批绑定 server，
// 交集返回请求的密钥键。只返回授权键，防越权读他人集群密钥。
func (s *HermesService) IssueMcpSecrets(ctx context.Context, deviceID string, envKeys []string) (map[string]string, error) {
	instance, err := s.dao.GetInstanceByDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if instance == nil || instance.ClusterID == 0 {
		return nil, fmt.Errorf("%w: device has no cluster", ErrHermesInvalidInput)
	}
	serverIDs, err := s.dao.ListClusterMcpServerIDs(ctx, instance.ClusterID)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	requested := map[string]bool{}
	for _, key := range envKeys {
		requested[key] = true
	}
	for _, serverID := range serverIDs {
		secrets, err := s.dao.GetMcpSecrets(ctx, serverID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, err
		}
		for key, value := range secrets {
			if requested[key] {
				result[key] = value
			}
		}
	}
	return result, nil
}

var _ = dao.ErrHermesMcpDuplicate
