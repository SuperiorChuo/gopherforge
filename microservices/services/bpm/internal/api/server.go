// Package api 装配 bpm-service HTTP 处理器。
// 鉴权约定：网关 auth-verify 注入的 X-Auth-* 头优先，
// Bearer JWT 兜底；internal 端点走 X-Internal-Token 共享密钥（内网直连）。
package api

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-kit/services/bpm/internal/authjwt"
	"github.com/go-admin-kit/services/bpm/internal/callback"
	"github.com/go-admin-kit/services/bpm/internal/engine"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"github.com/go-admin-kit/services/bpm/internal/notifyclient"
	"github.com/go-admin-kit/services/bpm/internal/store"
)

type Server struct {
	Store  *store.Store
	Engine *engine.Engine
	Secret string
	// InternalToken 校验业务方内网调用；空=internal 端点 503。
	InternalToken string
	// Notify 站内信（bpm.task_assigned / bpm.cc / bpm.result）；nil 或未配
	// token 时静默跳过。
	Notify *notifyclient.Client
	// Callback 终态回调分发器；nil=不回调。
	Callback *callback.Dispatcher
}

// ---- envelope（统一 {code,message,data}，与核心服务一致）----

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "success", "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg})
}

// ---- health ----

func (s *Server) HealthLive(c *gin.Context) {
	ok(c, gin.H{"status": "alive", "service": "bpm-service", "time": time.Now().Format(time.RFC3339)})
}

func (s *Server) HealthReady(c *gin.Context) {
	sqlDB, err := s.Store.DB().DB()
	if err != nil || sqlDB.Ping() != nil {
		fail(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ok(c, gin.H{"status": "ready"})
}

// ---- auth ----

func (s *Server) requireUser(c *gin.Context) (*authjwt.AgentClaims, bool) {
	if uid := c.GetHeader("X-Auth-User-ID"); uid != "" {
		if id, err := strconv.ParseUint(uid, 10, 64); err == nil && id > 0 {
			return &authjwt.AgentClaims{
				UserID:        id,
				Username:      c.GetHeader("X-Auth-Username"),
				TenantID:      tenantFromHeader(c),
				PlatformAdmin: c.GetHeader("X-Auth-Platform-Admin") == "1",
			}, true
		}
	}
	tok := bearer(c)
	if tok == "" {
		fail(c, http.StatusUnauthorized, "missing token")
		return nil, false
	}
	claims, err := authjwt.ParseAgent(s.Secret, tok)
	if err != nil {
		fail(c, http.StatusUnauthorized, "invalid token")
		return nil, false
	}
	return claims, true
}

// requireInternal 校验 X-Internal-Token；未配置密钥直接 503（拒绝裸奔），

func (s *Server) requireInternal(c *gin.Context) bool {
	if s.InternalToken == "" {
		fail(c, http.StatusServiceUnavailable, "internal endpoint disabled")
		return false
	}
	got := c.GetHeader("X-Internal-Token")
	if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.InternalToken)) != 1 {
		fail(c, http.StatusUnauthorized, "invalid internal token")
		return false
	}
	return true
}

// internalTenant 内部调用从 X-Tenant-ID 头取租户（缺省 1）。
func internalTenant(c *gin.Context) uint64 {
	if v := c.GetHeader("X-Tenant-ID"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			return authjwt.NormalizeTenantID(id)
		}
	}
	return 1
}

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func tenantFromHeader(c *gin.Context) uint64 {
	if v := c.GetHeader("X-Auth-Tenant-ID"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil {
			return authjwt.NormalizeTenantID(id)
		}
	}
	return 1
}

func pathID(c *gin.Context, name string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		fail(c, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func pageOf(c *gin.Context) store.Page {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	ps, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	return store.Page{Page: p, PageSize: ps}
}

// ---- effects 分发（事务提交后）----

// ApplyEffects 供进程内后台任务（超时 ticker）分发副作用的导出入口。
func (s *Server) ApplyEffects(eff *engine.Effects) { s.applyEffects(eff) }

// applyEffects 分发引擎收集的副作用：新待办/抄送站内信 + 终态回调与结果
// 通知。全部尽力而为，失败仅日志，不影响审批事实。
func (s *Server) applyEffects(eff *engine.Effects) {
	if eff == nil || eff.Instance == nil {
		return
	}
	inst := eff.Instance
	for i := range eff.NewTasks {
		s.notifyTaskAssigned(inst, &eff.NewTasks[i])
	}
	for i := range eff.CcRecords {
		s.notifyCc(inst, &eff.CcRecords[i])
	}
	if eff.FinalResult != "" {
		s.notifyResult(inst, eff.ResultText)
		if s.Callback != nil {
			finishedAt := ""
			if inst.FinishedAt != nil {
				finishedAt = inst.FinishedAt.Format(time.RFC3339)
			}
			s.Callback.Dispatch(inst.TenantID, callback.Payload{
				InstanceID:    inst.ID,
				DefinitionKey: inst.DefinitionKey,
				BizType:       inst.BizType,
				BizID:         inst.BizID,
				Result:        inst.Status,
				FormSnapshot:  []byte(inst.FormSnapshot),
				FinishedAt:    finishedAt,
			})
		}
	}
}

// notifyTaskAssigned 新待办站内信（模板 bpm.task_assigned；未建模板时
// 用直发标题兜底）。
func (s *Server) notifyTaskAssigned(inst *model.ProcessInstance, t *model.Task) {
	if s.Notify == nil || !s.Notify.Enabled() {
		return
	}
	taskID := strconv.FormatUint(t.ID, 10)
	in := notifyclient.SendInput{
		TenantID:     inst.TenantID,
		UserID:       t.AssigneeID,
		TemplateCode: "bpm.task_assigned",
		Type:         "bpm",
		RefType:      "bpm_task",
		RefID:        taskID,
		Vars: map[string]string{
			"instance_title": inst.Title,
			"node_name":      t.NodeName,
			"initiator_id":   strconv.FormatUint(inst.InitiatorID, 10),
			"task_id":        taskID,
		},
		Title:   "新的审批待办：" + inst.Title,
		Content: "节点「" + t.NodeName + "」等待你审批",
		Link:    "/bpm/todo?taskId=" + taskID,
	}
	s.sendNotify(in, "task-assigned")
}

// notifyCc 抄送站内信（模板 bpm.cc）。
func (s *Server) notifyCc(inst *model.ProcessInstance, rec *model.CcRecord) {
	if s.Notify == nil || !s.Notify.Enabled() {
		return
	}
	instID := strconv.FormatUint(inst.ID, 10)
	in := notifyclient.SendInput{
		TenantID:     inst.TenantID,
		UserID:       rec.UserID,
		TemplateCode: "bpm.cc",
		Type:         "bpm",
		RefType:      "bpm_cc",
		RefID:        strconv.FormatUint(rec.ID, 10),
		Vars: map[string]string{
			"instance_title": inst.Title,
			"instance_id":    instID,
			"node_name":      rec.NodeName,
		},
		Title:   "审批抄送：" + inst.Title,
		Content: "流程「" + inst.Title + "」抄送给你",
		Link:    "/bpm/instance/" + instID,
	}
	s.sendNotify(in, "cc")
}

// notifyResult 终态结果通知发起人（模板 bpm.result）。overrideText 非空时
// 覆盖按状态取的文案（管理员终止与发起人撤销同为 canceled，文案需区分）。
func (s *Server) notifyResult(inst *model.ProcessInstance, overrideText string) {
	if s.Notify == nil || !s.Notify.Enabled() {
		return
	}
	resultText := map[string]string{
		model.InstApproved: "已通过",
		model.InstRejected: "已拒绝",
		model.InstCanceled: "已撤销",
	}[inst.Status]
	if overrideText != "" {
		resultText = overrideText
	}
	instID := strconv.FormatUint(inst.ID, 10)
	in := notifyclient.SendInput{
		TenantID:     inst.TenantID,
		UserID:       inst.InitiatorID,
		TemplateCode: "bpm.result",
		Type:         "bpm",
		RefType:      "bpm_instance",
		RefID:        instID,
		Vars: map[string]string{
			"instance_title": inst.Title,
			"result":         inst.Status,
			"result_text":    resultText,
			"instance_id":    instID,
		},
		Title:   "审批结果：" + inst.Title + " " + resultText,
		Content: "你发起的流程「" + inst.Title + "」" + resultText,
		Link:    "/bpm/instance/" + instID,
	}
	s.sendNotify(in, "result")
}

func (s *Server) sendNotify(in notifyclient.SendInput, kind string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if _, err := s.Notify.Send(ctx, in); err != nil {
			log.Printf("bpm notify %s user=%d: %v", kind, in.UserID, err)
		}
	}()
}
