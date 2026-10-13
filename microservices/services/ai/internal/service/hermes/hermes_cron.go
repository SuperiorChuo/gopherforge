package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-admin-kit/services/ai/internal/dao"
	localmodel "github.com/go-admin-kit/services/ai/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/tenant"
	"gorm.io/gorm"
)

// Hermes M5：Cron 定时任务。控制面定义 cron 任务（表达式 + 提示词），manager 在
// 设备端评估调度、到点推提示词给本地 Hermes，结果回传控制面留痕。
//
// 校验器只保证格式合法（范围/语法），实际调度匹配由 manager 侧解析器执行；
// 两者对 5 段 cron 的语义约定一致（分 时 日 月 周，? 仅日/周，0 与 7 均为周日）。

const (
	maxHermesCronPromptLen    = 10000
	maxHermesCronExprLen      = 40
	maxHermesCronRunResultLen = 4000
)

// HermesCronJobRequest 是创建/更新定时任务的请求体。Description 用指针区分
// 「未传=不改」与「传空串=清空」——其余 string 字段沿用空串=不改的部分更新语义
// （name/expression/prompt 业务上不允许为空，无清空需求）。
type HermesCronJobRequest struct {
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	CronExpression string  `json:"cron_expression"`
	ActionType     string  `json:"action_type"`
	Prompt         string  `json:"prompt"`
	Enabled        bool    `json:"enabled"`
}

// validateHermesCronExpression 校验 5 段 cron 表达式的格式与取值范围。语义与
// manager 侧 protocol.ParseCronExpression 一致（那里做真正的匹配）。
func validateHermesCronExpression(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" || len(expr) > maxHermesCronExprLen {
		return fmt.Errorf("%w: invalid cron expression", ErrHermesInvalidInput)
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("%w: cron expression must have 5 fields", ErrHermesInvalidInput)
	}
	type spec struct {
		value    string
		min, max int
		allowQ   bool
	}
	specs := []spec{
		{fields[0], 0, 59, false}, // 分
		{fields[1], 0, 23, false}, // 时
		{fields[2], 1, 31, true},  // 日
		{fields[3], 1, 12, false}, // 月
		{fields[4], 0, 7, true},   // 周（0 与 7 均为周日）
	}
	for _, s := range specs {
		if err := validateCronField(s.value, s.min, s.max, s.allowQ); err != nil {
			return err
		}
	}
	return nil
}

func validateCronField(field string, min, max int, allowQ bool) error {
	field = strings.TrimSpace(field)
	if field == "" {
		return fmt.Errorf("%w: empty cron field", ErrHermesInvalidInput)
	}
	if field == "*" {
		return nil
	}
	if field == "?" {
		if allowQ {
			return nil
		}
		return fmt.Errorf("%w: '?' only allowed in day-of-month/day-of-week", ErrHermesInvalidInput)
	}
	// 列表：逗号分隔
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("%w: empty cron field part", ErrHermesInvalidInput)
		}
		if err := validateCronRange(part, min, max); err != nil {
			return err
		}
	}
	return nil
}

func validateCronRange(part string, min, max int) error {
	// 步进：*/n 或 a-b/n
	if slash := strings.IndexByte(part, '/'); slash >= 0 {
		base := strings.TrimSpace(part[:slash])
		stepStr := strings.TrimSpace(part[slash+1:])
		step, err := strconv.Atoi(stepStr)
		if err != nil || step <= 0 {
			return fmt.Errorf("%w: invalid cron step", ErrHermesInvalidInput)
		}
		if base == "*" {
			return nil
		}
		part = base
	}
	// 范围：a-b
	var lo, hi int
	var err error
	if dash := strings.IndexByte(part, '-'); dash >= 0 {
		lo, err = strconv.Atoi(strings.TrimSpace(part[:dash]))
		if err != nil {
			return fmt.Errorf("%w: invalid cron value", ErrHermesInvalidInput)
		}
		hi, err = strconv.Atoi(strings.TrimSpace(part[dash+1:]))
		if err != nil {
			return fmt.Errorf("%w: invalid cron value", ErrHermesInvalidInput)
		}
	} else {
		lo, err = strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return fmt.Errorf("%w: invalid cron value", ErrHermesInvalidInput)
		}
		hi = lo
	}
	if lo < min || hi > max || lo > hi {
		return fmt.Errorf("%w: cron value out of range", ErrHermesInvalidInput)
	}
	return nil
}

func (s *HermesService) CreateCronJob(ctx context.Context, actorID uint, request HermesCronJobRequest) (*localmodel.HermesCronJob, error) {
	if actorID == 0 {
		return nil, fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	request.Name = strings.TrimSpace(request.Name)
	request.CronExpression = strings.TrimSpace(request.CronExpression)
	request.Prompt = strings.TrimSpace(request.Prompt)
	if len(request.Name) == 0 || len(request.Name) > 80 {
		return nil, fmt.Errorf("%w: invalid cron job name", ErrHermesInvalidInput)
	}
	if err := validateHermesCronExpression(request.CronExpression); err != nil {
		return nil, err
	}
	if request.ActionType == "" {
		request.ActionType = localmodel.HermesCronActionPrompt
	}
	if request.ActionType != localmodel.HermesCronActionPrompt {
		return nil, fmt.Errorf("%w: unsupported action type", ErrHermesInvalidInput)
	}
	if request.Prompt == "" {
		return nil, fmt.Errorf("%w: prompt is required", ErrHermesInvalidInput)
	}
	if len(request.Prompt) > maxHermesCronPromptLen {
		return nil, fmt.Errorf("%w: prompt too long", ErrHermesInvalidInput)
	}
	description := ""
	if request.Description != nil {
		description = strings.TrimSpace(*request.Description)
	}
	if len(description) > maxHermesCronPromptLen {
		return nil, fmt.Errorf("%w: description too long", ErrHermesInvalidInput)
	}
	now := s.now().UTC()
	job := &localmodel.HermesCronJob{
		TenantID:       tenant.FromContextOrDefault(ctx),
		Name:           request.Name,
		Description:    description,
		CronExpression: request.CronExpression,
		ActionType:     request.ActionType,
		Prompt:         request.Prompt,
		Enabled:        request.Enabled,
		CreatedBy:      actorID,
		UpdatedBy:      actorID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.dao.CreateCronJob(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *HermesService) UpdateCronJob(ctx context.Context, actorID uint, id uint64, request HermesCronJobRequest) (*localmodel.HermesCronJob, error) {
	if actorID == 0 {
		return nil, fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	existing, err := s.dao.GetCronJob(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: cron job not found", ErrHermesInvalidInput)
		}
		return nil, err
	}
	updates := map[string]any{}
	if request.Name != "" {
		name := strings.TrimSpace(request.Name)
		if len(name) == 0 || len(name) > 80 {
			return nil, fmt.Errorf("%w: invalid cron job name", ErrHermesInvalidInput)
		}
		updates["name"] = name
	}
	if request.CronExpression != "" {
		if err := validateHermesCronExpression(request.CronExpression); err != nil {
			return nil, err
		}
		updates["cron_expression"] = strings.TrimSpace(request.CronExpression)
	}
	if request.Description != nil {
		description := strings.TrimSpace(*request.Description)
		if len(description) > maxHermesCronPromptLen {
			return nil, fmt.Errorf("%w: description too long", ErrHermesInvalidInput)
		}
		if description != existing.Description {
			updates["description"] = description
		}
	}
	if request.ActionType != "" && request.ActionType != localmodel.HermesCronActionPrompt {
		return nil, fmt.Errorf("%w: unsupported action type", ErrHermesInvalidInput)
	}
	if request.Prompt != "" {
		prompt := strings.TrimSpace(request.Prompt)
		if prompt == "" || len(prompt) > maxHermesCronPromptLen {
			return nil, fmt.Errorf("%w: invalid prompt", ErrHermesInvalidInput)
		}
		updates["prompt"] = prompt
	}
	if request.Enabled != existing.Enabled {
		updates["enabled"] = request.Enabled
	}
	if len(updates) == 0 {
		return existing, nil
	}
	updates["updated_by"] = actorID
	updates["updated_at"] = s.now().UTC()
	if err := s.dao.UpdateCronJob(ctx, id, updates); err != nil {
		return nil, err
	}
	return s.dao.GetCronJob(ctx, id)
}

func (s *HermesService) ToggleCronJob(ctx context.Context, actorID uint, id uint64) (*localmodel.HermesCronJob, error) {
	if actorID == 0 {
		return nil, fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	job, err := s.dao.GetCronJob(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: cron job not found", ErrHermesInvalidInput)
		}
		return nil, err
	}
	if err := s.dao.UpdateCronJob(ctx, id, map[string]any{
		"enabled":    !job.Enabled,
		"updated_by": actorID,
		"updated_at": s.now().UTC(),
	}); err != nil {
		return nil, err
	}
	return s.dao.GetCronJob(ctx, id)
}

func (s *HermesService) DeleteCronJob(ctx context.Context, actorID uint, id uint64) error {
	if actorID == 0 {
		return fmt.Errorf("%w: missing actor", ErrHermesInvalidInput)
	}
	if _, err := s.dao.GetCronJob(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: cron job not found", ErrHermesInvalidInput)
		}
		return err
	}
	return s.dao.DeleteCronJob(ctx, id)
}

func (s *HermesService) ListCronJobs(ctx context.Context, page, pageSize int) ([]dao.HermesCronJobView, int64, error) {
	return s.dao.ListCronJobs(ctx, page, pageSize)
}

func (s *HermesService) ListCronRuns(ctx context.Context, jobID uint64, page, pageSize int) ([]localmodel.HermesCronRun, int64, error) {
	return s.dao.ListCronRuns(ctx, jobID, page, pageSize)
}

// buildDesiredStateCron 返回租户启用的定时任务，供 buildDesiredState 注入签名
// desired-state。无启用任务时返回 nil（omitempty 省略，P1 payload 字节不变）。
func (s *HermesService) buildDesiredStateCron(ctx context.Context) ([]HermesDesiredStateCronJob, error) {
	jobs, err := s.dao.ListEnabledCronJobs(ctx)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, nil
	}
	out := make([]HermesDesiredStateCronJob, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, HermesDesiredStateCronJob{
			ID:             job.ID,
			Name:           job.Name,
			CronExpression: job.CronExpression,
			Prompt:         job.Prompt,
			Enabled:        true,
		})
	}
	return out, nil
}

// ReportDeviceCronRun 是设备端执行结果的回传（签名通道）。job 必须存在且属同租户，
// status 只收 completed/failed（v1 设备路径直接写终态；triggered 留给未来异步流）。
func (s *HermesService) ReportDeviceCronRun(ctx context.Context, deviceID string, jobID uint64, status, result string) error {
	status = strings.TrimSpace(strings.ToLower(status))
	result = strings.TrimSpace(result)
	switch status {
	case localmodel.HermesCronRunStatusCompleted, localmodel.HermesCronRunStatusFailed:
	default:
		return fmt.Errorf("%w: unsupported cron run status", ErrHermesInvalidInput)
	}
	if result == "" {
		return fmt.Errorf("%w: result is required", ErrHermesInvalidInput)
	}
	if len(result) > maxHermesCronRunResultLen {
		return fmt.Errorf("%w: result too long", ErrHermesInvalidInput)
	}
	if jobID == 0 {
		return fmt.Errorf("%w: invalid cron job id", ErrHermesInvalidInput)
	}
	if _, err := s.dao.GetCronJob(ctx, jobID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: cron job not found", ErrHermesInvalidInput)
		}
		return err
	}
	now := s.now().UTC()
	run := &localmodel.HermesCronRun{
		TenantID:    tenant.FromContextOrDefault(ctx),
		JobID:       jobID,
		DeviceID:    deviceID,
		Status:      status,
		Result:      result,
		TriggeredAt: &now,
		FinishedAt:  &now,
		CreatedAt:   now,
	}
	return s.dao.CreateCronRun(ctx, run)
}
