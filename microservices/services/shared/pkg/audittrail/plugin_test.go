package audittrail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/mask"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type auditTestUser struct {
	ID                 uint   `gorm:"primaryKey"`
	TenantID           uint   `gorm:"not null"`
	Username           string `gorm:"not null"`
	Password           string `gorm:"not null"`
	Nickname           string
	Email              string
	Phone              string
	MustChangePassword bool
	PasswordChangedAt  *time.Time
	TOTPSecret         string
	TOTPEnabled        bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (auditTestUser) TableName() string { return "users" }

type storedAuditLog struct {
	ID         uint           `gorm:"primaryKey"`
	TenantID   uint           `gorm:"not null"`
	ActorType  string         `gorm:"not null"`
	ActorID    string         `gorm:"not null"`
	Action     string         `gorm:"not null"`
	TargetType string         `gorm:"not null"`
	TargetID   string         `gorm:"not null"`
	BeforeJSON map[string]any `gorm:"column:before_json;serializer:json"`
	AfterJSON  map[string]any `gorm:"column:after_json;serializer:json"`
	Summary    string
	CreatedAt  time.Time
}

func (storedAuditLog) TableName() string { return "audit_logs" }

type auditTestMenu struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"not null"`
}

func (auditTestMenu) TableName() string { return "menus" }

type auditHookedUser struct {
	ID             uint `gorm:"primaryKey"`
	TenantID       uint
	ExpectedTenant uint `gorm:"-"`
	Name           string
}

func (auditHookedUser) TableName() string { return "hooked_users" }

func (user *auditHookedUser) BeforeCreate(*gorm.DB) error {
	if user.TenantID != user.ExpectedTenant {
		return fmt.Errorf("BeforeCreate saw tenant %d, want %d", user.TenantID, user.ExpectedTenant)
	}
	return nil
}

type auditTestParent struct {
	ID       uint `gorm:"primaryKey"`
	TenantID uint
	Name     string
	Children []auditTestChild `gorm:"foreignKey:ParentID"`
}

func (auditTestParent) TableName() string { return "audit_test_parents" }

type auditTestChild struct {
	ID       uint `gorm:"primaryKey"`
	ParentID uint `gorm:"index"`
	Name     string
}

func (auditTestChild) TableName() string { return "audit_test_children" }

func TestContextCarriesNormalizedActorAndTenant(t *testing.T) {
	ctx := WithTenantID(WithActor(context.Background(), " operator ", " alice "), 42)

	actor, ok := ActorFromContext(ctx)
	if !ok {
		t.Fatal("ActorFromContext() did not find the explicitly attached actor")
	}
	if actor.Type != "operator" || actor.ID != "alice" {
		t.Fatalf("actor = %#v, want normalized operator/alice", actor)
	}
	if tenantID, ok := TenantIDFromContext(ctx); !ok || tenantID != 42 {
		t.Fatalf("tenant = (%d, %v), want (42, true)", tenantID, ok)
	}
}

func TestPluginCreateWritesRedactedAuditInActorTenant(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	ctx := actorTenantContext("alice", 42)
	changedAt := time.Date(2026, 8, 1, 8, 30, 0, 0, time.UTC)
	user := auditTestUser{
		TenantID:           999,
		Username:           "new-user",
		Password:           "password-hash-create",
		Nickname:           "before",
		Email:              "alice@example.com",
		Phone:              "13812345678",
		MustChangePassword: true,
		PasswordChangedAt:  &changedAt,
		TOTPSecret:         "totp-secret-create",
	}

	if err := db.WithContext(ctx).Create(&user).Error; err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if user.TenantID != 42 {
		t.Fatalf("created tenant = %d, want actor tenant 42", user.TenantID)
	}

	log := onlyAuditLog(t, db)
	if log.TenantID != 42 || log.ActorType != "operator" || log.ActorID != "alice" {
		t.Fatalf("audit attribution = tenant:%d actor:%s/%s", log.TenantID, log.ActorType, log.ActorID)
	}
	if log.Action != "create" || log.TargetType != "user" || log.TargetID != fmt.Sprint(user.ID) {
		t.Fatalf("audit target = %s %s/%s", log.Action, log.TargetType, log.TargetID)
	}
	if len(log.BeforeJSON) != 0 {
		t.Fatalf("create before = %#v, want empty", log.BeforeJSON)
	}
	assertSecretValueRedacted(t, log.AfterJSON, "password", "password-hash-create")
	assertSecretValueRedacted(t, log.AfterJSON, "totp_secret", "totp-secret-create")
	if got := log.AfterJSON["email"]; got != "a***e@example.com" {
		t.Fatalf("masked email = %#v", got)
	}
	if got := log.AfterJSON["phone"]; got != "138****5678" {
		t.Fatalf("masked phone = %#v", got)
	}
	if got, ok := log.AfterJSON["must_change_password"].(bool); !ok || !got {
		t.Fatalf("must_change_password = %#v, want visible boolean true", log.AfterJSON["must_change_password"])
	}
}

func TestPluginUpdateCapturesDatabaseBeforeAndAfterForMapAndSave(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	user := seedAuditTestUser(t, db, 42, "before")
	ctx := actorTenantContext("bob", 42)

	if err := db.WithContext(ctx).
		Model(&auditTestUser{}).
		Where("id = ?", user.ID).
		Updates(map[string]any{
			"nickname":             "after-map",
			"password":             "password-hash-map",
			"must_change_password": true,
		}).Error; err != nil {
		t.Fatalf("map Updates() error = %v", err)
	}

	mapLog := onlyAuditLog(t, db)
	if mapLog.Action != "update" || mapLog.BeforeJSON["nickname"] != "before" || mapLog.AfterJSON["nickname"] != "after-map" {
		t.Fatalf("map update snapshots = before:%#v after:%#v", mapLog.BeforeJSON, mapLog.AfterJSON)
	}
	assertSecretValueRedacted(t, mapLog.AfterJSON, "password", "password-hash-map")
	if got, ok := mapLog.AfterJSON["must_change_password"].(bool); !ok || !got {
		t.Fatalf("updated must_change_password = %#v, want true", mapLog.AfterJSON["must_change_password"])
	}

	if err := db.Where("1 = 1").Delete(&storedAuditLog{}).Error; err != nil {
		t.Fatalf("clear audit logs: %v", err)
	}
	user.Nickname = "after-save"
	user.Password = "password-hash-save"
	if err := db.WithContext(ctx).Save(&user).Error; err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	saveLog := onlyAuditLog(t, db)
	if saveLog.BeforeJSON["nickname"] != "after-map" || saveLog.AfterJSON["nickname"] != "after-save" {
		t.Fatalf("Save snapshots = before:%#v after:%#v", saveLog.BeforeJSON, saveLog.AfterJSON)
	}
	assertSecretValueRedacted(t, saveLog.AfterJSON, "password", "password-hash-save")
}

func TestPluginDeleteCapturesBeforeAndNoAfter(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	user := seedAuditTestUser(t, db, 7, "delete-me")

	if err := db.WithContext(actorTenantContext("deleter", 7)).Delete(&auditTestUser{}, user.ID).Error; err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	log := onlyAuditLog(t, db)
	if log.Action != "delete" || log.BeforeJSON["nickname"] != "delete-me" || len(log.AfterJSON) != 0 {
		t.Fatalf("delete snapshots = before:%#v after:%#v", log.BeforeJSON, log.AfterJSON)
	}
	assertSecretValueRedacted(t, log.BeforeJSON, "password", "password-hash-seed")
}

func TestPluginRequiresFinitePrimaryKeyAndRejectsOverLimitMutation(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	first := seedAuditTestUser(t, db, 9, "first")
	second := seedAuditTestUser(t, db, 9, "second")
	ctx := actorTenantContext("bulk-operator", 9)

	err := db.WithContext(ctx).
		Model(&auditTestUser{}).
		Where("tenant_id = ?", 9).
		Update("nickname", "bulk").Error
	if !errors.Is(err, ErrUnboundedMutation) {
		t.Fatalf("unbounded Update() error = %v, want ErrUnboundedMutation", err)
	}

	err = db.WithContext(ctx).
		Model(&auditTestUser{}).
		Where("id IN ?", []uint{first.ID, second.ID}).
		Update("nickname", "bulk").Error
	if !errors.Is(err, ErrMutationTooBroad) {
		t.Fatalf("over-limit Update() error = %v, want ErrMutationTooBroad", err)
	}

	var users []auditTestUser
	if err := db.Order("id ASC").Find(&users).Error; err != nil {
		t.Fatalf("load users: %v", err)
	}
	if len(users) != 2 || users[0].Nickname != "first" || users[1].Nickname != "second" {
		t.Fatalf("rejected mutations changed users: %#v", users)
	}
	assertAuditCount(t, db, 0)
}

func TestPluginAcceptsPrimaryKeyWithAdditionalPredicateAndRejectsRawOR(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	user := seedAuditTestUser(t, db, 9, "guarded-before")
	ctx := actorTenantContext("guarded-operator", 9)

	if err := db.WithContext(ctx).
		Model(&auditTestUser{}).
		Where("id = ? AND password = ?", user.ID, "password-hash-seed").
		UpdateColumn("nickname", "guarded-after").Error; err != nil {
		t.Fatalf("bounded guarded UpdateColumn() error = %v", err)
	}
	log := onlyAuditLog(t, db)
	if log.BeforeJSON["nickname"] != "guarded-before" || log.AfterJSON["nickname"] != "guarded-after" {
		t.Fatalf("guarded snapshots = before:%#v after:%#v", log.BeforeJSON, log.AfterJSON)
	}

	clearAuditLogs(t, db)
	result := db.WithContext(ctx).
		Model(&auditTestUser{}).
		Where("id = ? AND password = ?", user.ID, "wrong-password").
		Update("nickname", "must-not-change")
	if result.Error != nil || result.RowsAffected != 0 {
		t.Fatalf("non-matching guarded update = rows:%d error:%v, want no-op", result.RowsAffected, result.Error)
	}
	assertAuditCount(t, db, 0)

	unsafePredicates := []string{
		"id = ? OR id = ?",
		"id = ?\nOR 1 = ?",
		"id = ?\tOR\t1 = ?",
		"id = ? OR(1 = ?)",
		"NOT id = ? AND id = ?",
		"id = ? IS FALSE AND id = ?",
	}
	for _, predicate := range unsafePredicates {
		err := db.WithContext(ctx).
			Model(&auditTestUser{}).
			Where(predicate, user.ID, user.ID+1).
			Update("nickname", "unsafe-predicate").Error
		if !errors.Is(err, ErrUnboundedMutation) {
			t.Fatalf("unsafe predicate %q error = %v, want ErrUnboundedMutation", predicate, err)
		}
	}
	var got auditTestUser
	if err := db.First(&got, user.ID).Error; err != nil {
		t.Fatalf("reload guarded user: %v", err)
	}
	if got.Nickname != "guarded-after" {
		t.Fatalf("nickname = %q after rejected predicates, want guarded-after", got.Nickname)
	}
}

func TestPluginRejectsChainedORBeforeWriting(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	first := seedAuditTestUser(t, db, 9, "first-before")
	second := seedAuditTestUser(t, db, 9, "second-before")
	ctx := actorTenantContext("guarded-operator", 9)

	err := db.WithContext(ctx).
		Model(&auditTestUser{}).
		Where("id = ?", first.ID).
		Or("id = ?", second.ID).
		Update("nickname", "must-not-change").Error
	if !errors.Is(err, ErrUnboundedMutation) {
		t.Fatalf("chained OR update error = %v, want ErrUnboundedMutation", err)
	}
	err = db.WithContext(ctx).
		Model(&first).
		Or("1 = 1").
		Update("nickname", "must-not-change-from-model").Error
	if !errors.Is(err, ErrUnboundedMutation) {
		t.Fatalf("model primary key with chained OR error = %v, want ErrUnboundedMutation", err)
	}

	assertUserNickname(t, db, first.ID, "first-before")
	assertUserNickname(t, db, second.ID, "second-before")
	assertAuditCount(t, db, 0)
}

func TestPluginAllowsParenthesizedORWhenModelPrimaryKeyRemainsTheBound(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	first := seedAuditTestUser(t, db, 9, "first-before")
	second := seedAuditTestUser(t, db, 9, "second-before")
	ctx := actorTenantContext("guarded-operator", 9)

	result := db.WithContext(ctx).
		Model(&first).
		Where(clause.Or(
			clause.Eq{Column: "nickname", Value: "first-before"},
			clause.Eq{Column: "nickname", Value: "second-before"},
		)).
		Update("nickname", "first-after")
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("model update with parenthesized OR = rows:%d error:%v", result.RowsAffected, result.Error)
	}

	assertUserNickname(t, db, first.ID, "first-after")
	assertUserNickname(t, db, second.ID, "second-before")
	assertAuditCount(t, db, 1)
}

func TestPluginSupportsBoundedMultiRowMutationWhenConfigured(t *testing.T) {
	db := openAuditTrailTestDB(t, 2)
	first := seedAuditTestUser(t, db, 14, "first")
	second := seedAuditTestUser(t, db, 14, "second")

	result := db.WithContext(actorTenantContext("bulk-operator", 14)).
		Model(&auditTestUser{}).
		Where("id IN ?", []uint{first.ID, second.ID}).
		Update("nickname", "bulk-after")
	if result.Error != nil || result.RowsAffected != 2 {
		t.Fatalf("bounded multi-row update = rows:%d error:%v", result.RowsAffected, result.Error)
	}
	assertAuditCount(t, db, 2)
}

func TestPluginRejectsOverLimitCreateBeforeWriting(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	users := []auditTestUser{
		{Username: "batch-first", Password: "first-secret"},
		{Username: "batch-second", Password: "second-secret"},
	}

	err := db.WithContext(actorTenantContext("batch-operator", 15)).Create(&users).Error
	if !errors.Is(err, ErrMutationTooBroad) {
		t.Fatalf("over-limit Create() error = %v, want ErrMutationTooBroad", err)
	}
	var count int64
	if err := db.Model(&auditTestUser{}).Count(&count).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("user count = %d after rejected batch create, want 0", count)
	}
	assertAuditCount(t, db, 0)
}

func TestPluginAuditInsertFailureRollsBackBusinessWrite(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	user := seedAuditTestUser(t, db, 11, "stable")
	if err := db.Migrator().DropTable(&storedAuditLog{}); err != nil {
		t.Fatalf("drop audit_logs: %v", err)
	}

	err := db.WithContext(actorTenantContext("alice", 11)).
		Model(&auditTestUser{}).
		Where("id = ?", user.ID).
		Update("nickname", "must-rollback").Error
	if err == nil {
		t.Fatal("Update() error = nil after audit_logs was removed")
	}

	var got auditTestUser
	if err := db.First(&got, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if got.Nickname != "stable" {
		t.Fatalf("nickname = %q, want rolled back value stable", got.Nickname)
	}
}

func TestPluginOuterTransactionRollbackRemovesBusinessAndAuditRows(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	rollbackErr := errors.New("rollback requested by caller")
	ctx := actorTenantContext("transaction-operator", 16)

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user := auditTestUser{Username: "rolled-back", Password: "transaction-secret"}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		var auditCount int64
		if err := tx.Model(&storedAuditLog{}).Count(&auditCount).Error; err != nil {
			return err
		}
		if auditCount != 1 {
			return fmt.Errorf("audit count inside transaction = %d, want 1", auditCount)
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("Transaction() error = %v, want caller rollback", err)
	}
	var userCount int64
	if err := db.Model(&auditTestUser{}).Count(&userCount).Error; err != nil {
		t.Fatalf("count users after rollback: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("user count after rollback = %d, want 0", userCount)
	}
	assertAuditCount(t, db, 0)
}

func TestPluginRequiresTransactionButAllowsExplicitTransaction(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	user := seedAuditTestUser(t, db, 17, "transaction-required")
	ctx := actorTenantContext("transaction-operator", 17)

	err := db.Session(&gorm.Session{SkipDefaultTransaction: true}).WithContext(ctx).
		Model(&auditTestUser{}).
		Where("id = ?", user.ID).
		Update("nickname", "must-not-commit").Error
	if !errors.Is(err, ErrTransactionRequired) {
		t.Fatalf("transactionless Update() error = %v, want ErrTransactionRequired", err)
	}
	assertUserNickname(t, db, user.ID, "transaction-required")
	assertAuditCount(t, db, 0)

	err = db.Session(&gorm.Session{SkipDefaultTransaction: true}).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Model(&auditTestUser{}).
			Where("id = ?", user.ID).
			Update("nickname", "explicit-transaction").Error
	})
	if err != nil {
		t.Fatalf("explicit transaction Update() error = %v", err)
	}
	assertUserNickname(t, db, user.ID, "explicit-transaction")
	assertAuditCount(t, db, 1)
}

func TestPluginRequiresTenantForExplicitActor(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	ctx := WithActor(context.Background(), "operator", "missing-tenant")
	user := auditTestUser{Username: "must-not-exist", Password: "secret"}

	err := db.WithContext(ctx).Create(&user).Error
	if !errors.Is(err, ErrTenantContextRequired) {
		t.Fatalf("actor-only Create() error = %v, want ErrTenantContextRequired", err)
	}
	var count int64
	if err := db.Model(&auditTestUser{}).Count(&count).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("user count = %d after missing-tenant create, want 0", count)
	}
}

func TestPluginFixedTenantSurvivesExistingTenantCreatePlugin(t *testing.T) {
	db := openBaseAuditTrailTestDB(t, &auditTestMenu{})
	if err := db.Callback().Create().Before("gorm:create").Register("test:tenant:before_create", func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.LookUpField("tenant_id") == nil {
			return
		}
		tenantID, _ := tx.Statement.Context.Value(tenantCompatibilityContextKey).(uint)
		if tenantID > 0 {
			setTenantField(tx, "tenant_id", tenantID)
		}
	}); err != nil {
		t.Fatalf("register tenant simulation: %v", err)
	}
	if err := Register(db, Config{Targets: []Target{{
		Model:          &auditTestMenu{},
		Table:          "menus",
		TargetType:     "menu",
		FixedTenantID:  1,
		SnapshotFields: []string{"id", "name"},
	}}}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	menu := auditTestMenu{Name: "global-menu"}
	if err := db.WithContext(actorTenantContext("tenant-42-operator", 42)).Create(&menu).Error; err != nil {
		t.Fatalf("Create(menu) error = %v", err)
	}
	log := onlyAuditLog(t, db)
	if log.TenantID != 1 {
		t.Fatalf("fixed-tenant audit attribution = %d, want platform tenant 1", log.TenantID)
	}
}

func TestPluginSetsTenantBeforeModelBeforeCreateHook(t *testing.T) {
	db := openBaseAuditTrailTestDB(t, &auditHookedUser{})
	if err := Register(db, Config{Targets: []Target{{
		Model:          &auditHookedUser{},
		Table:          "hooked_users",
		TargetType:     "hooked_user",
		TenantField:    "tenant_id",
		SnapshotFields: []string{"id", "tenant_id", "name"},
	}}}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	user := auditHookedUser{TenantID: 999, ExpectedTenant: 23, Name: "hook-order"}
	if err := db.WithContext(actorTenantContext("hook-operator", 23)).Create(&user).Error; err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if user.TenantID != 23 {
		t.Fatalf("created tenant = %d, want 23", user.TenantID)
	}
	if log := onlyAuditLog(t, db); log.TenantID != 23 {
		t.Fatalf("audit tenant = %d, want 23", log.TenantID)
	}
}

func TestPluginRejectsCrossTenantDeleteBeforeAssociations(t *testing.T) {
	db := openBaseAuditTrailTestDB(t, &auditTestParent{}, &auditTestChild{})
	if err := Register(db, Config{Targets: []Target{{
		Model:          &auditTestParent{},
		Table:          "audit_test_parents",
		TargetType:     "parent",
		TenantField:    "tenant_id",
		SnapshotFields: []string{"id", "tenant_id", "name"},
	}}}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	parent := auditTestParent{
		TenantID: 1,
		Name:     "tenant-one",
		Children: []auditTestChild{{Name: "must-survive"}},
	}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("seed parent: %v", err)
	}

	err := db.WithContext(actorTenantContext("tenant-two-operator", 2)).
		Select(clause.Associations).
		Delete(&parent).Error
	if !errors.Is(err, ErrAuditConsistency) {
		t.Fatalf("cross-tenant Delete() error = %v, want ErrAuditConsistency", err)
	}
	var parentCount, childCount int64
	if err := db.Model(&auditTestParent{}).Count(&parentCount).Error; err != nil {
		t.Fatalf("count parents: %v", err)
	}
	if err := db.Model(&auditTestChild{}).Count(&childCount).Error; err != nil {
		t.Fatalf("count children: %v", err)
	}
	if parentCount != 1 || childCount != 1 {
		t.Fatalf("cross-tenant delete changed associations: parents=%d children=%d", parentCount, childCount)
	}
	assertAuditCount(t, db, 0)
}

func TestPluginAssociationCallbacksDoNotReuseParentState(t *testing.T) {
	db := openBaseAuditTrailTestDB(t, &auditTestParent{}, &auditTestChild{})
	if err := Register(db, Config{Targets: []Target{{
		Model:          &auditTestParent{},
		Table:          "audit_test_parents",
		TargetType:     "parent",
		TenantField:    "tenant_id",
		SnapshotFields: []string{"id", "tenant_id", "name"},
	}}}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	ctx := actorTenantContext("association-operator", 25)
	parent := auditTestParent{
		TenantID: 999,
		Name:     "parent-before",
		Children: []auditTestChild{{Name: "child-before"}},
	}

	if err := db.WithContext(ctx).Create(&parent).Error; err != nil {
		t.Fatalf("Create(parent with association) error = %v", err)
	}
	assertAuditCount(t, db, 1)
	clearAuditLogs(t, db)

	parent.Name = "parent-after"
	parent.Children[0].Name = "child-after"
	if err := db.WithContext(ctx).
		Session(&gorm.Session{FullSaveAssociations: true}).
		Save(&parent).Error; err != nil {
		t.Fatalf("Save(parent with association) error = %v", err)
	}
	log := onlyAuditLog(t, db)
	if log.BeforeJSON["name"] != "parent-before" || log.AfterJSON["name"] != "parent-after" {
		t.Fatalf("parent association-save snapshots = before:%#v after:%#v", log.BeforeJSON, log.AfterJSON)
	}
}

func TestPluginRejectsMapCreateSaveFallbackAndTableOverrides(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	ctx := actorTenantContext("shape-operator", 24)

	err := db.WithContext(ctx).Model(&auditTestUser{}).Create(map[string]any{
		"tenant_id": 24,
		"username":  "map-create",
		"password":  "map-secret",
	}).Error
	if !errors.Is(err, ErrUnsupportedMutation) {
		t.Fatalf("map Create() error = %v, want ErrUnsupportedMutation", err)
	}

	missing := auditTestUser{ID: 5000, TenantID: 24, Username: "save-missing", Password: "save-secret"}
	err = db.WithContext(ctx).Save(&missing).Error
	if !errors.Is(err, ErrAuditConsistency) {
		t.Fatalf("missing-row Save() error = %v, want ErrAuditConsistency", err)
	}

	err = db.WithContext(ctx).
		Model(&auditTestUser{}).
		Table("users AS u").
		Where("id = ?", 1).
		Update("nickname", "alias-bypass").Error
	if !errors.Is(err, ErrUnsupportedMutation) {
		t.Fatalf("aliased table Update() error = %v, want ErrUnsupportedMutation", err)
	}

	err = db.WithContext(ctx).
		Table("users").
		Where("id = ?", 1).
		Updates(map[string]any{"nickname": "schema-bypass"}).Error
	if !errors.Is(err, ErrUnsupportedMutation) {
		t.Fatalf("schema-less table Update() error = %v, want ErrUnsupportedMutation", err)
	}

	var count int64
	if err := db.Model(&auditTestUser{}).Count(&count).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("unsupported mutations created %d users, want 0", count)
	}
	assertAuditCount(t, db, 0)
}

func TestPluginRejectsRawExecAgainstAuditedTables(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	user := seedAuditTestUser(t, db, 24, "raw-before")
	ctx := actorTenantContext("raw-operator", 24)

	err := db.WithContext(ctx).
		Exec(`UPDATE "users" SET nickname = ? WHERE id = ?`, "raw-after", user.ID).
		Error
	if !errors.Is(err, ErrUnsupportedMutation) {
		t.Fatalf("raw Exec() error = %v, want ErrUnsupportedMutation", err)
	}

	assertUserNickname(t, db, user.ID, "raw-before")
	assertAuditCount(t, db, 0)
}

func TestNewPluginRejectsUnprotectedMaskConfiguration(t *testing.T) {
	_, err := NewPlugin(Config{Targets: []Target{{
		Model:          &auditTestUser{},
		Table:          "users",
		TargetType:     "user",
		SnapshotFields: []string{"id", "email"},
		FieldMasks:     map[string]string{"email": "typo-mask"},
	}}})
	if err == nil {
		t.Fatal("NewPlugin() accepted an unknown mask strategy")
	}
	_, err = NewPlugin(Config{Targets: []Target{{
		Model:          &auditTestUser{},
		Table:          "users",
		TargetType:     "user",
		SnapshotFields: []string{"id"},
		FieldMasks:     map[string]string{"email": "email"},
	}}})
	if err == nil {
		t.Fatal("NewPlugin() accepted a mask field outside the snapshot whitelist")
	}
}

func TestPluginSkipsWritesWithoutExplicitActorAndNeverAuditsAuditLogs(t *testing.T) {
	db := openAuditTrailTestDB(t, 1)
	user := auditTestUser{TenantID: 1, Username: "bootstrap", Password: "bootstrap-secret"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("actorless Create() error = %v", err)
	}
	assertAuditCount(t, db, 0)

	manual := storedAuditLog{
		TenantID: 1, ActorType: "system", ActorID: "manual", Action: "create",
		TargetType: "manual", TargetID: "1",
	}
	if err := db.WithContext(actorTenantContext("alice", 1)).Create(&manual).Error; err != nil {
		t.Fatalf("manual audit Create() error = %v", err)
	}
	assertAuditCount(t, db, 1)
}

func openAuditTrailTestDB(t *testing.T, maxRows int) *gorm.DB {
	t.Helper()
	db := openBaseAuditTrailTestDB(t, &auditTestUser{})

	if err := Register(db, Config{
		MaxRows: maxRows,
		Targets: []Target{{
			Model:       &auditTestUser{},
			Table:       "users",
			TargetType:  "user",
			TenantField: "tenant_id",
			SnapshotFields: []string{
				"id", "tenant_id", "username", "password", "nickname", "email", "phone",
				"must_change_password", "password_changed_at", "totp_secret", "totp_enabled",
				"created_at", "updated_at",
			},
			FieldMasks: map[string]string{"email": "email", "phone": "phone"},
		}},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	return db
}

func openBaseAuditTrailTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+dsnName+"?mode=memory&cache=shared"), &gorm.Config{PrepareStmt: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	models = append(models, &storedAuditLog{})
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	return db
}

func seedAuditTestUser(t *testing.T, db *gorm.DB, tenantID uint, nickname string) auditTestUser {
	t.Helper()
	user := auditTestUser{
		TenantID: tenantID,
		Username: fmt.Sprintf("user-%d-%s", tenantID, nickname),
		Password: "password-hash-seed",
		Nickname: nickname,
		Email:    "seed@example.com",
		Phone:    "13912345678",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return user
}

func actorTenantContext(actorID string, tenantID uint) context.Context {
	return WithTenantID(WithActor(context.Background(), "operator", actorID), tenantID)
}

func onlyAuditLog(t *testing.T, db *gorm.DB) storedAuditLog {
	t.Helper()
	var logs []storedAuditLog
	if err := db.Order("id ASC").Find(&logs).Error; err != nil {
		t.Fatalf("load audit logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("audit log count = %d, want 1: %#v", len(logs), logs)
	}
	return logs[0]
}

func clearAuditLogs(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Where("1 = 1").Delete(&storedAuditLog{}).Error; err != nil {
		t.Fatalf("clear audit logs: %v", err)
	}
}

func assertUserNickname(t *testing.T, db *gorm.DB, id uint, want string) {
	t.Helper()
	var user auditTestUser
	if err := db.First(&user, id).Error; err != nil {
		t.Fatalf("load user %d: %v", id, err)
	}
	if user.Nickname != want {
		t.Fatalf("user %d nickname = %q, want %q", id, user.Nickname, want)
	}
}

func assertAuditCount(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()
	var got int64
	if err := db.Model(&storedAuditLog{}).Count(&got).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if got != want {
		t.Fatalf("audit log count = %d, want %d", got, want)
	}
}

func assertSecretValueRedacted(t *testing.T, snapshot map[string]any, field, secret string) {
	t.Helper()
	if got := snapshot[field]; got != mask.RedactedValue {
		t.Fatalf("%s = %#v, want redacted marker", field, got)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("snapshot leaked %s: %s", field, encoded)
	}
}
