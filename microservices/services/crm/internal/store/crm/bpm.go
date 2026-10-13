package store

// 合同审批接入（BPM）专用写路径：状态推进一律条件更新（乐观锁），
// 幂等承接 bpm 终态回调。仅在 BPM_ENABLE 开启的链路被调用；
// 开关关闭时这些方法不会被触达，CRM 行为与接入前完全一致。

import (
	"context"
	"errors"
	"time"

	"github.com/go-admin-kit/services/crm/internal/model"
	"gorm.io/gorm"
)

// 合同状态（原有 draft/active/void 不变，BPM 接入新增 approving）。
const (
	ContractDraft     = "draft"
	ContractApproving = "approving"
	ContractActive    = "active"
	ContractVoid      = "void"
)

var (
	ErrContractNotDraft     = errors.New("仅草拟状态的合同可提交审批")
	ErrContractNotApproving = errors.New("合同不在审批中")
)

// 回款状态（BPM M2：登记走审批；BPM 关闭时直接 confirmed，与接入前一致）。
const (
	PaymentPendingApproval = "pending_approval"
	PaymentConfirmed       = "confirmed"
	PaymentRejected        = "rejected"
)

var ErrPaymentNotPending = errors.New("回款记录不在审批中")

// MarkContractApproving 提交审批成功后 draft → approving（条件更新防并发）。
func (s *Store) MarkContractApproving(id, tenantID uint64) error {
	res := s.db.Model(&model.Contract{}).
		Where("id = ? AND tenant_id = ? AND status = ?", id, tenantID, ContractDraft).
		Update("status", ContractApproving)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrContractNotDraft
	}
	return nil
}

// ApplyContractApprovalResult 承接 bpm 终态回调：
//   - approved  → active（signed_at 为空时补当前时间）
//   - rejected / canceled → 回 draft
//
// 幂等：合同已在目标状态时返回 nil（回调可能重试多次）。
func (s *Store) ApplyContractApprovalResult(id, tenantID uint64, result string) error {
	var target string
	switch result {
	case "approved":
		target = ContractActive
	case "rejected", "canceled":
		target = ContractDraft
	default:
		return errors.New("未知审批结果: " + result)
	}
	ct, err := s.GetContract(id, tenantID)
	if err != nil {
		return err
	}
	if ct.Status == target {
		return nil // 幂等：重复回调
	}
	if ct.Status != ContractApproving {
		return ErrContractNotApproving
	}
	updates := map[string]any{"status": target}
	if target == ContractActive && ct.SignedAt == nil {
		now := time.Now()
		updates["signed_at"] = now
	}
	res := s.db.Model(&model.Contract{}).
		Where("id = ? AND tenant_id = ? AND status = ?", id, tenantID, ContractApproving).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrContractNotApproving
	}
	return nil
}

// ApplyPaymentApprovalResult 承接 bpm 回款审批终态回调（M2）：
//   - approved → confirmed（此后才计入回款统计）
//   - rejected / canceled → rejected（保留记录）
//
// 幂等：已在目标状态返回 nil；仅 pending_approval 态允许迁移（条件更新）。
func (s *Store) ApplyPaymentApprovalResult(id, tenantID uint64, result string) error {
	var target string
	switch result {
	case "approved":
		target = PaymentConfirmed
	case "rejected", "canceled":
		target = PaymentRejected
	default:
		return errors.New("未知审批结果: " + result)
	}
	var p model.Payment
	if err := s.db.Where("id = ? AND tenant_id = ?", id, tenantID).First(&p).Error; err != nil {
		return errors.New("回款记录不存在")
	}
	if p.Status == target {
		return nil // 幂等：重复回调
	}
	if p.Status != PaymentPendingApproval {
		return ErrPaymentNotPending
	}
	res := s.db.Model(&model.Payment{}).
		Where("id = ? AND tenant_id = ? AND status = ?", id, tenantID, PaymentPendingApproval).
		Update("status", target)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrPaymentNotPending
	}
	return nil
}

// DeletePendingPayment 发起审批失败时的补偿：删除刚登记、仍在
// pending_approval 态的回款记录（避免悬挂在审批中却无实例）。
// 单行删除，带审计 ctx（补偿路径与 AddPayment 的 create 同 actor，完整留痕）；
// 状态条件保证幂等，删除时若已非 pending_approval 则跳过。
func (s *Store) DeletePendingPayment(ctx context.Context, id, tenantID uint64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.
			Where("id = ? AND tenant_id = ? AND status = ?", id, tenantID, PaymentPendingApproval).
			Delete(&model.Payment{})
		if res.Error != nil {
			return res.Error
		}
		return nil
	})
}
