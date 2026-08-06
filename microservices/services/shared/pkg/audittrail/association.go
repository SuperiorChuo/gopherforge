package audittrail

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// RecordAssociationRequest 描述一次关联表（M2M）变更的语义快照。关联表写入是
// 批替换（delete-all + insert-many），装不进插件单行/PK 边界，在这里显式落一行：
// Before/After 是该关系的 id 集合，TargetID 是关系所属的宿主行（user_id/role_id）。
type RecordAssociationRequest struct {
	TargetType    string
	TargetID      string
	Action        string
	Before        map[string]any
	After         map[string]any
	Summary       string
	FixedTenantID uint // 非 0：平台归属目标（如 menu_permissions），忽略 ctx 租户
}

// RecordAssociation 在调用方事务内为一次关联表变更记一行 audit_logs，与业务写
// 同事务提交/回滚。actor 从 ctx 解析；无 actor 的后台写静默跳过（与插件一致）。
// 未在事务内调用返回 ErrTransactionRequired——审计行必须与关联变更同生共死。
func RecordAssociation(ctx context.Context, tx *gorm.DB, req RecordAssociationRequest) error {
	actor, ok := ActorFromContext(ctx)
	if !ok {
		return nil
	}
	if tx == nil || tx.Statement == nil || tx.Statement.ConnPool == nil {
		return ErrTransactionRequired
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return fmt.Errorf("%w: association audit requires an active transaction", ErrTransactionRequired)
	}
	tenantID := req.FixedTenantID
	if tenantID == 0 {
		var found bool
		tenantID, found = TenantIDFromContext(ctx)
		if !found {
			return ErrTenantContextRequired
		}
	}
	auditDB := tx.Session(&gorm.Session{
		NewDB:     true,
		SkipHooks: true,
		Context:   internalContext(ctx, tenantID),
	})
	return auditDB.Create(&auditRecord{
		TenantID:   tenantID,
		ActorType:  actor.Type,
		ActorID:    actor.ID,
		Action:     req.Action,
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		BeforeJSON: req.Before,
		AfterJSON:  req.After,
		Summary:    req.Summary,
		CreatedAt:  time.Now(),
	}).Error
}
