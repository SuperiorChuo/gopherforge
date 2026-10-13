package localmodel

import "time"

// Hermes M2：技能市场。技能 = 目录 + 必带 SKILL.md 的知识文档包（tarball），
// 注入员工 Hermes 上下文供办公使用。控制面存 tarball 本体（content bytea），
// manager 下载校验后直解压进 <profile>/skills/<name>/（上游 filesystem scan
// 自动发现，无需 config 引用）。

const (
	HermesSkillPending  = "pending"
	HermesSkillApproved = "approved"
	HermesSkillRejected = "rejected"
	HermesSkillRevoked  = "revoked"
)

// 供应链验签状态（M7 安全）：verified=信任根验签通过 / unverified=有信任根但未签名 /
// not_applicable=官方市场导入（无企业签名，靠 repo@commit 来源锚定）。
const (
	HermesSkillSignatureVerified      = "verified"
	HermesSkillSignatureUnverified    = "unverified"
	HermesSkillSignatureNotApplicable = "not_applicable"
)

// HermesSkill 是不可变技能发布（同 hermes_artifacts 语义）。version 不可变，
// 升级 = 新建 (name, version) 行并重新审批绑定。Content 是 tarball 本体，不进
// list 查询（仅下载端点 Select 读取）。
type HermesSkill struct {
	ID          uint64 `gorm:"primaryKey" json:"id"`
	TenantID    uint   `gorm:"not null;index;uniqueIndex:uk_hermes_skills_tenant_name_version,priority:1" json:"tenant_id"`
	Name        string `gorm:"size:80;not null;uniqueIndex:uk_hermes_skills_tenant_name_version,priority:2" json:"name"`
	DisplayName string `gorm:"size:120;not null" json:"display_name"`
	Description string `gorm:"type:text;not null" json:"description"`
	Version     string `gorm:"size:40;not null;uniqueIndex:uk_hermes_skills_tenant_name_version,priority:3" json:"version"`
	SHA256      string `gorm:"size:64;not null" json:"sha256"`
	Signature   string `gorm:"type:text;not null" json:"signature,omitempty"`
	// SignatureStatus 是供应链验签状态：verified（信任根验签通过）/
	// unverified（有信任根但未签名）/ not_applicable（官方市场导入，无企业签名）。
	SignatureStatus string `gorm:"size:16;not null;default:unverified" json:"signature_status"`
	Content         []byte `gorm:"type:bytea;not null" json:"-"`
	Status          string `gorm:"size:16;not null" json:"status"`
	// SourceRepo/SourcePath/SourceCommit 是官方市场导入的 provenance（钉住
	// repo@commit 的目录）；手工上传为空串。
	SourceRepo   string     `gorm:"size:200;not null;default:''" json:"source_repo"`
	SourcePath   string     `gorm:"size:300;not null;default:''" json:"source_path"`
	SourceCommit string     `gorm:"size:64;not null;default:''" json:"source_commit"`
	ApprovedBy   uint       `gorm:"not null" json:"approved_by"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (HermesSkill) TableName() string { return "hermes_skills" }

// HermesClusterSkill 是集群绑定：集群绑定某个技能版本（skill_id），随
// desired-state 流到该集群全部 profile（orchestrator + workers）。
type HermesClusterSkill struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint      `gorm:"not null;index;uniqueIndex:uk_hermes_cluster_skills_cluster_skill,priority:1" json:"tenant_id"`
	ClusterID uint64    `gorm:"not null;uniqueIndex:uk_hermes_cluster_skills_cluster_skill,priority:2" json:"cluster_id"`
	SkillID   uint64    `gorm:"not null;uniqueIndex:uk_hermes_cluster_skills_cluster_skill,priority:3" json:"skill_id"`
	CreatedBy uint      `gorm:"not null" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (HermesClusterSkill) TableName() string { return "hermes_cluster_skills" }

// HermesUserSkill 是员工自助绑定（M6）：员工把已审批技能装到自己名下，随
// desired-state 下发到该员工全部设备，与集群技能合并（同名集群优先）。
type HermesUserSkill struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	TenantID  uint      `gorm:"not null;index;uniqueIndex:uk_hermes_user_skills_tenant_user_skill,priority:1" json:"tenant_id"`
	UserID    uint      `gorm:"not null;uniqueIndex:uk_hermes_user_skills_tenant_user_skill,priority:2" json:"user_id"`
	SkillID   uint64    `gorm:"not null;uniqueIndex:uk_hermes_user_skills_tenant_user_skill,priority:3" json:"skill_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (HermesUserSkill) TableName() string { return "hermes_user_skills" }
