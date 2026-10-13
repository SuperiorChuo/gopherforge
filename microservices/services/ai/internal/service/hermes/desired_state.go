package service

import (
	"context"
	"errors"
	"fmt"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"gorm.io/gorm"
	"strings"
	"time"
)

type HermesDesiredStateAssistant struct {
	TemplateID string `json:"template_id"`
	Revision   uint64 `json:"template_revision"`
}

type HermesDesiredStateArtifact struct {
	ID        uint64 `json:"artifact_id"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	SHA256    string `json:"sha256"`
	SourceURL string `json:"source_url"`
}

type HermesDesiredStateModel struct {
	Mode    string `json:"mode"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

// HermesDesiredStateTask 是下发给单个 Agent 的同步任务意图（三期 manager 下发
// 通道）。manager 验签后把 Description 推给本地 Hermes（Gateway API server），
// 执行完成后经 POST /device/task-runs 回传结果。与 manager 侧
// protocol.DesiredStateTask 逐字一致（含 omitempty）。
type HermesDesiredStateTask struct {
	TaskID      uint64 `json:"task_id"`
	RunID       uint64 `json:"run_id"`
	Description string `json:"description"`
	Model       string `json:"model,omitempty"`
}

// HermesDesiredStateKanbanWorker / HermesDesiredStateKanban 是 kanban 多 Agent
// fan-out 指令（本里程碑），与 manager 侧 protocol.DesiredStateKanban 逐字一致。
// 与 HermesDesiredStateTask（push 通道）互斥：Kanban != nil 时走 kanban 通道。
type HermesDesiredStateKanbanWorker struct {
	Name string `json:"name"`
}

type HermesDesiredStateKanban struct {
	TaskID    uint64                           `json:"task_id"`
	RunID     uint64                           `json:"run_id"`
	BoardSlug string                           `json:"board_slug"`
	Title     string                           `json:"title"`
	Body      string                           `json:"body"`
	Assignee  string                           `json:"assignee"`
	Model     string                           `json:"model,omitempty"`
	Workers   []HermesDesiredStateKanbanWorker `json:"workers,omitempty"`
}

// HermesDesiredStateSkill 是下发给设备的一个技能引用（M2 技能市场）。manager
// 按 SourceURL 下载 tarball、校验 sha256、解压进 profile skills 树。与 manager
// 侧 protocol.DesiredStateSkill 逐字一致。
type HermesDesiredStateSkill struct {
	ID        uint64 `json:"skill_id"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
	SourceURL string `json:"source_url"`
}

// HermesDesiredStateMcpServer 是下发给设备的 MCP server 配置（M3 MCP 管理）。
// Env/Headers 值只含 ${VAR} 模板，明文密钥经设备签名端点另取；SecretRevision
// 让 manager 感知密钥轮换。与 manager 侧 protocol.DesiredStateMcpServer 逐字一致。
type HermesDesiredStateMcpServer struct {
	ID             uint64            `json:"server_id"`
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	URL            string            `json:"url,omitempty"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	ToolsInclude   []string          `json:"tools_include,omitempty"`
	ToolsExclude   []string          `json:"tools_exclude,omitempty"`
	Timeout        int               `json:"timeout,omitempty"`
	SecretRevision uint64            `json:"secret_revision,omitempty"`
}

// HermesDesiredStateCronJob 是下发给设备的定时任务引用（M5）。manager 在设备端
// 评估 cron 表达式，到点通过 loopback API server 把 Prompt 推给本地 Hermes。
// 与 manager 侧 protocol.DesiredStateCronJob 逐字一致。
type HermesDesiredStateCronJob struct {
	ID             uint64 `json:"job_id"`
	Name           string `json:"name"`
	CronExpression string `json:"cron_expression"`
	Prompt         string `json:"prompt"`
	Enabled        bool   `json:"enabled"`
}

type hermesUnsignedDesiredState struct {
	SchemaVersion  int                         `json:"schema_version"`
	Revision       uint64                      `json:"revision"`
	DesiredState   string                      `json:"desired_state"`
	LeaseExpiresAt time.Time                   `json:"lease_expires_at"`
	Profile        string                      `json:"profile"`
	Assistant      HermesDesiredStateAssistant `json:"assistant"`
	Hermes         HermesDesiredStateArtifact  `json:"hermes"`
	Model          HermesDesiredStateModel     `json:"model"`
	Toolsets       localmodel.HermesToolsets   `json:"toolsets"`
	Privacy        localmodel.HermesPrivacy    `json:"privacy"`
	// 管理员排的一次性运维动作。空串表示无动作。随本结构一起签名，
	// 因此 manager 可以确认动作确实来自控制面而非中间人。
	PendingAction string `json:"pending_action,omitempty"`
	// Task 是非空时的多 Agent 任务意图。指针 + omitempty：零值 struct 不会被
	// encoding/json 省略，指针才能在无任务时让 P1 payload 字节不变。必须是最
	// 后一个字段，与 manager 侧 UnsignedDesiredState mirror 一致。
	Task *HermesDesiredStateTask `json:"task,omitempty"`
	// Kanban 是非空时的 kanban fan-out 指令。与 Task 互斥。指针 + omitempty，
	// 必须是最后一个字段，与 manager 侧 mirror 一致。
	Kanban *HermesDesiredStateKanban `json:"kanban,omitempty"`
	// Skills 是集群绑定并已审批的技能引用（M2）。空时省略，P1 payload 字节不变。
	// 必须是最后一个字段，与 manager 侧 mirror 一致。
	Skills []HermesDesiredStateSkill `json:"skills,omitempty"`
	// McpServers 是集群绑定并已审批的 MCP server 配置（M3）。空时省略，P1 payload
	// 字节不变。必须是最后一个字段，与 manager 侧 mirror 一致。
	McpServers []HermesDesiredStateMcpServer `json:"mcp_servers,omitempty"`
	// Cron 是租户启用的定时任务（M5）。空时省略，P1 payload 字节不变。必须是最
	// 后一个字段，与 manager 侧 mirror 一致。
	Cron []HermesDesiredStateCronJob `json:"cron,omitempty"`
}

type HermesDesiredState struct {
	hermesUnsignedDesiredState
	Signature string `json:"signature"`
}

func (s *HermesService) DesiredState(ctx context.Context, device *localmodel.HermesDevice) (*HermesDesiredState, error) {
	if s.signer == nil {
		return nil, ErrHermesControlPlaneUnavailable
	}
	state, err := s.buildDesiredState(ctx, device)
	if err != nil {
		return nil, err
	}
	return s.signDesiredState(*state)
}

// resolveDeviceContext 是期望状态生成与模型代理授权的单一策略优先级路径。
// 三期起支持 company_cluster：实例归属启用集群且集群绑了策略时，worker 设备
// 用集群策略（dispatcher 开放工具集）；否则走 instance -> entitlement ->
// latest（全 company）。返回的 instance 供 buildDesiredState 填充 Task。
// enabled=false 表示权益或实例被停用，设备必须停跑，无论回落到哪个策略。
func (s *HermesService) resolveDeviceContext(ctx context.Context, device *localmodel.HermesDevice) (*localmodel.HermesPolicy, *localmodel.HermesInstance, bool, error) {
	entitlement, err := s.dao.GetEntitlement(ctx, device.UserID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, false, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || !entitlement.Enabled {
		return nil, nil, false, nil
	}

	instance, instanceErr := s.dao.GetInstanceByDevice(ctx, device.DeviceID)
	if instanceErr != nil && !errors.Is(instanceErr, gorm.ErrRecordNotFound) {
		return nil, nil, false, instanceErr
	}
	if instance != nil && !instance.Enabled {
		return nil, nil, false, nil
	}

	var policy *localmodel.HermesPolicy
	// 集群归属优先：实例入启用集群且集群绑了策略 → worker 设备用集群策略
	clusterResolved := false
	if instance != nil && instance.ClusterID > 0 {
		cluster, clusterErr := s.dao.GetCluster(ctx, instance.ClusterID)
		if clusterErr != nil && !errors.Is(clusterErr, gorm.ErrRecordNotFound) {
			return nil, nil, false, clusterErr
		}
		if clusterErr == nil && cluster.Enabled && cluster.PolicyID > 0 {
			policy, err = s.dao.GetPolicy(ctx, cluster.PolicyID)
			clusterResolved = true
		}
	}
	if policy == nil {
		switch {
		case instance != nil && instance.PolicyID > 0:
			policy, err = s.dao.GetPolicy(ctx, instance.PolicyID)
		case entitlement.PolicyID > 0:
			policy, err = s.dao.GetPolicy(ctx, entitlement.PolicyID)
		default:
			policy, err = s.dao.GetLatestPolicy(ctx)
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, false, ErrHermesPolicyNotConfigured
	}
	if err != nil {
		return nil, nil, false, err
	}
	// 防御不变式：company_cluster 策略只能经「启用集群归属」到达设备。DB 绕过
	// 把 cluster 策略放到实例/权益上时拒绝，防止非受控 dispatcher 工具集下发。
	if policy.Profile == localmodel.HermesProfileCompanyCluster && !clusterResolved {
		return nil, nil, false, fmt.Errorf("%w: %v", ErrHermesInvalidInput, errHermesClusterPolicyNotDeployable)
	}
	return policy, instance, true, nil
}

// resolveDevicePolicy 是 resolveDeviceContext 的薄包装，供模型代理授权等只需
// 策略语义的调用方复用（cluster 设备同样按集群策略路由模型，语义一致）。
func (s *HermesService) resolveDevicePolicy(ctx context.Context, device *localmodel.HermesDevice) (*localmodel.HermesPolicy, bool, error) {
	policy, _, enabled, err := s.resolveDeviceContext(ctx, device)
	return policy, enabled, err
}

func (s *HermesService) buildDesiredState(ctx context.Context, device *localmodel.HermesDevice) (*hermesUnsignedDesiredState, error) {
	if device.RevokedAt != nil {
		return &hermesUnsignedDesiredState{
			SchemaVersion:  1,
			DesiredState:   localmodel.HermesDesiredStateRevoked,
			LeaseExpiresAt: s.now().UTC().Add(5 * time.Minute),
			Profile:        "company",
			Toolsets:       localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
			Privacy:        localmodel.HermesPrivacy{},
		}, nil
	}
	policy, instance, enabled, err := s.resolveDeviceContext(ctx, device)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return &hermesUnsignedDesiredState{
			SchemaVersion:  1,
			DesiredState:   localmodel.HermesDesiredStateStopped,
			LeaseExpiresAt: s.now().UTC().Add(5 * time.Minute),
			Profile:        "company",
			Toolsets:       localmodel.HermesToolsets{"clarify", "memory", "no_mcp", "todo"},
			Privacy:        localmodel.HermesPrivacy{},
		}, nil
	}

	artifact := HermesDesiredStateArtifact{ID: policy.ArtifactID}
	if policy.ArtifactID > 0 {
		storedArtifact, err := s.dao.GetArtifact(ctx, policy.ArtifactID)
		if err != nil {
			return nil, err
		}
		if storedArtifact.Status != localmodel.HermesArtifactApproved {
			return nil, fmt.Errorf("Hermes artifact is no longer approved")
		}
		artifact.Version = storedArtifact.Version
		artifact.Commit = storedArtifact.UpstreamCommit
		artifact.SHA256 = storedArtifact.SHA256
		artifact.SourceURL = storedArtifact.SourceURL
	}

	state := &hermesUnsignedDesiredState{
		SchemaVersion:  1,
		Revision:       policy.Revision,
		DesiredState:   policy.DesiredState,
		LeaseExpiresAt: s.now().UTC().Add(time.Duration(policy.LeaseSeconds) * time.Second),
		Profile:        policy.Profile,
		Assistant: HermesDesiredStateAssistant{
			TemplateID: policy.AssistantTemplateID,
			Revision:   policy.AssistantTemplateRevision,
		},
		Hermes: artifact,
		Model: HermesDesiredStateModel{
			Mode:    "company_proxy",
			BaseURL: policy.ModelBaseURL,
			Model:   policy.ModelName,
		},
		Toolsets:      policy.Toolsets,
		Privacy:       policy.Privacy,
		PendingAction: device.PendingAction,
	}
	// 实例在集群时的任务下发：kanban 模式（集群 slug 非空）只给绑定设备的 lead
	// 实例发 Kanban 指令（worker 无设备无 Task/Kanban，由 dispatcher 按 assignee
	// 派生）；push 模式（空 slug）沿用三期把最老 assigned run 作为 Task。
	var clusterSkills []HermesDesiredStateSkill
	if instance != nil && instance.ClusterID > 0 {
		cluster, clusterErr := s.dao.GetCluster(ctx, instance.ClusterID)
		if clusterErr != nil {
			return nil, clusterErr
		}
		if cluster.Slug != "" {
			if instance.DeviceID != "" {
				directive, err := s.kanbanDirectiveForInstance(ctx, instance, cluster, policy)
				if err != nil {
					return nil, err
				}
				state.Kanban = directive
			}
		} else {
			task, err := s.pendingTaskForInstance(ctx, instance.ID, policy)
			if err != nil {
				return nil, err
			}
			state.Task = task
		}
		// M2 技能市场：实例在集群 → 载入集群绑定的已审批技能（无论 kanban 或 push）
		clusterSkills, err = s.buildDesiredStateSkills(ctx, instance.ClusterID)
		if err != nil {
			return nil, err
		}
		// M3 MCP 管理：同样载入集群绑定的已审批 MCP server（env/headers 只含模板）。
		// MCP 保持集群-only（自助绑定涉及企业密钥注入员工机，留 M6 后续安全评审）。
		servers, err := s.buildDesiredStateMcpServers(ctx, instance.ClusterID)
		if err != nil {
			return nil, err
		}
		state.McpServers = servers
	}
	// M6 员工自助：员工自装的已审批技能下发到其全部设备，与集群技能合并
	// （同名集群优先）。非集群设备由此从「零技能」变为可有技能。
	userSkills, err := s.buildDesiredStateUserSkills(ctx, device.UserID)
	if err != nil {
		return nil, err
	}
	state.Skills = mergeDesiredStateSkills(clusterSkills, userSkills)
	// M5 Cron：租户启用的定时任务，注入到每台设备（不限于集群门控块内）
	state.Cron, err = s.buildDesiredStateCron(ctx)
	if err != nil {
		return nil, err
	}
	return state, nil
}

// kanbanDirectiveForInstance 为 lead 设备构造 Kanban fan-out 指令：取实例最老
// 一条 assigned run，worker 集 = 集群内全部实例名 slugify，assignee = 本实例
// slugify。无 pending run 或 worker 集为空时返回 nil（Kanban 省略）。
func (s *HermesService) kanbanDirectiveForInstance(ctx context.Context, instance *localmodel.HermesInstance, cluster *localmodel.HermesCluster, policy *localmodel.HermesPolicy) (*HermesDesiredStateKanban, error) {
	runs, err := s.dao.ListPendingRunsByInstance(ctx, instance.ID, 1)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	run := runs[0]
	task, err := s.dao.GetTask(ctx, run.TaskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 任务被删，run 成为孤儿，不派发
		}
		return nil, err
	}
	instances, err := s.dao.ListInstancesByCluster(ctx, cluster.ID)
	if err != nil {
		return nil, err
	}
	workers := make([]HermesDesiredStateKanbanWorker, 0, len(instances))
	for _, inst := range instances {
		if name := slugifyHermesWorkerName(inst.Name); name != "" {
			workers = append(workers, HermesDesiredStateKanbanWorker{Name: name})
		}
	}
	if len(workers) == 0 {
		return nil, nil
	}
	assignee := slugifyHermesWorkerName(instance.Name)
	if assignee == "" {
		assignee = workers[0].Name
	}
	return &HermesDesiredStateKanban{
		TaskID:    task.ID,
		RunID:     run.ID,
		BoardSlug: cluster.Slug,
		Title:     task.Name,
		Body:      task.Description,
		Assignee:  assignee,
		Model:     policy.ModelName,
		Workers:   workers,
	}, nil
}

// pendingTaskForInstance 取实例最老一条 assigned run 构造成任务意图。没有
// pending run 时返回 nil（Task 字段省略，P1 payload 字节不变）。
func (s *HermesService) pendingTaskForInstance(ctx context.Context, instanceID uint64, policy *localmodel.HermesPolicy) (*HermesDesiredStateTask, error) {
	runs, err := s.dao.ListPendingRunsByInstance(ctx, instanceID, 1)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	run := runs[0]
	task, err := s.dao.GetTask(ctx, run.TaskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 任务被删，run 成为孤儿，不派发
		}
		return nil, err
	}
	return &HermesDesiredStateTask{
		TaskID:      task.ID,
		RunID:       run.ID,
		Description: task.Description,
		Model:       policy.ModelName,
	}, nil
}

// HermesDeviceActions 是控制面允许下发的全部动作。文档 §3.3：控制面不能发送
// shell 字符串、脚本正文或任意可执行参数，manager 只把状态收敛到预定义动作。
// 这里与迁移 000035 的 CHECK 约束必须保持一致。
var HermesDeviceActions = []string{"retry_update", "apply_policy", "restart"}

// 动作与 manager 回执事件类型的对应关系，用于收到事件后清除待办动作。
var hermesActionEventTypes = map[string][]string{
	"retry_update": {"update", "rollback"},
	"apply_policy": {"apply_policy"},
	"restart":      {"start", "stop"},
}

// RequestDeviceAction 给设备排一个运维动作，随下一次期望状态拉取下发。
func (s *HermesService) RequestDeviceAction(ctx context.Context, deviceID, action string) error {
	deviceID = strings.TrimSpace(deviceID)
	action = strings.ToLower(strings.TrimSpace(action))
	if deviceID == "" {
		return fmt.Errorf("%w: device id is required", ErrHermesInvalidInput)
	}
	if !containsHermesValue(HermesDeviceActions, action) {
		return fmt.Errorf("%w: unsupported device action", ErrHermesInvalidInput)
	}
	return s.dao.SetPendingAction(ctx, deviceID, action, s.now().UTC())
}

// CancelDeviceAction 撤销尚未执行的动作。
func (s *HermesService) CancelDeviceAction(ctx context.Context, deviceID string) error {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return fmt.Errorf("%w: device id is required", ErrHermesInvalidInput)
	}
	return s.dao.SetPendingAction(ctx, deviceID, "", s.now().UTC())
}

func (s *HermesService) signDesiredState(unsigned hermesUnsignedDesiredState) (*HermesDesiredState, error) {
	signature, err := s.signer.SignJSON(unsigned)
	if err != nil {
		return nil, err
	}
	return &HermesDesiredState{hermesUnsignedDesiredState: unsigned, Signature: signature}, nil
}
