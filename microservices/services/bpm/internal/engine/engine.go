// Package engine 实现审批流推进：节点树上的单游标 + 任务展开器。
//
// 所有推进在单个 DB 事务内完成（更新任务 → 节点计数收敛 → 移动游标 →
// 展开下个节点任务 → 写日志）；实例行在 postgres 下加 SELECT ... FOR UPDATE
// 行锁防会签并发双推进（sqlite 测试库单连接天然串行），任务状态更新一律带
// WHERE status='pending' 条件更新做乐观兜底。
//
// 通知与回调等副作用不在事务内发出：引擎把它们收集进 Effects，由调用方
// （api 层）在事务提交后分发，保证"审批事实先落库"。
package engine

import (
	"errors"
	"github.com/go-admin-kit/services/bpm/internal/model"
	"github.com/go-admin-kit/services/shared/pkg/identityclient"
	"gorm.io/gorm"
)

var (
	ErrNoActiveDefinition  = errors.New("流程没有已发布版本，无法发起")
	ErrDuplicateRunning    = errors.New("该业务对象已有在途审批，不可重复发起")
	ErrIdempotencyKeyReuse = errors.New("幂等键已用于不同的审批请求")
	ErrIdempotencyRecord   = errors.New("幂等记录缺少审批实例")
	ErrTaskNotFound        = errors.New("任务不存在")
	ErrNotAssignee         = errors.New("仅当前处理人可操作该任务")
	ErrTaskHandled         = errors.New("任务已被处理")
	ErrInstanceNotFound    = errors.New("流程实例不存在")
	ErrInstanceNotRunning  = errors.New("流程实例不在审批中")
	ErrNotInitiator        = errors.New("仅发起人可操作")
	ErrCancelDenied        = errors.New("已有审批通过记录，不可撤销")
	ErrCommentRequired     = errors.New("拒绝必须填写意见")
	// M2 新增动作错误
	ErrTransferTarget     = errors.New("转办目标人不能为空")
	ErrTransferSelf       = errors.New("不能转办给自己")
	ErrReturnComment      = errors.New("退回必须填写意见")
	ErrReturnTarget       = errors.New("退回目标未知（仅支持 start / prev）")
	ErrBackPrevNotAllowed = errors.New("该节点未开启退回上一节点")
	ErrNotReturnedState   = errors.New("流程未处于退回待重提状态")
	ErrReturnStartTask    = errors.New("重新提交任务请使用重提或撤销，不支持该动作")
	// M3 新增动作错误
	ErrTerminateReason  = errors.New("终止必须填写原因")
	ErrInstanceFinished = errors.New("流程实例已结束")
	// M3+ 加签 / 委派动作错误
	ErrAddSignTarget    = errors.New("加签目标人不能为空")
	ErrAddSignSeq       = errors.New("依次审批节点不支持加签")
	ErrAddSignDuplicate = errors.New("目标人已在当前节点待审，不能重复加签")
	ErrDelegateTarget   = errors.New("委派目标人不能为空")
	ErrDelegateSelf     = errors.New("不能委派给自己")
	ErrTaskDelegated    = errors.New("任务委派办理中，仅支持办理完成")
	ErrNotDelegated     = errors.New("任务不在委派办理中")
	ErrDelegateComment  = errors.New("办理完成必须填写意见")
)

type Engine struct {
	db       *gorm.DB
	idClient *identityclient.Client
}

// SetIdentity 设置 identity owner API 客户端（Phase 3：gRPC 优先 + HTTP 回退）。
func (e *Engine) SetIdentity(apiBase, internalToken string) {
	if c, err := identityclient.New(apiBase, internalToken); err == nil {
		e.idClient = c
	}
}

func New(db *gorm.DB) *Engine { return &Engine{db: db} }

// Effects 事务内收集、提交后由调用方分发的副作用。
type Effects struct {
	Instance *model.ProcessInstance
	// NewTasks 本次推进新展开的待办任务（发 bpm.task_assigned 站内信）
	NewTasks []model.Task
	// CcRecords 本次推进落地的抄送记录（发 bpm.cc 站内信）
	CcRecords []model.CcRecord
	// FinalResult 非空表示实例到达终态（approved|rejected|canceled），
	// 需发终态回调 + 给发起人发 bpm.result 站内信
	FinalResult string
	// ResultText 终态文案覆盖（管理员终止时区分于发起人撤销；空=按状态取）
	ResultText string
	// DelegatedTasks 本次委派给受托人的任务（发 bpm.task_delegated 站内信）
	DelegatedTasks []model.Task
	// DelegateResolvedTasks 委派办结回到原处理人的任务（发 bpm.task_delegate_resolved 站内信）
	DelegateResolvedTasks []model.Task
}

// instVars 实例运行期变量（M1：发起人自选的选人结果）。
type instVars struct {
	SelectedAssignees map[string][]uint64 `json:"selected_assignees"`
}
